package z3950

import (
	"errors"
	"fmt"
)

// This file is a minimal BER (Basic Encoding Rules) reader and writer covering
// exactly what the Z39.50 APDUs need: context/universal classes, multi-byte tag
// numbers (Z39.50 uses tags like 105 and 211), definite and indefinite lengths on
// read (servers may emit either), and definite lengths on write. It is written
// from X.690, not ported from any existing implementation.

// BER identifier classes (bits 8-7 of the identifier octet).
const (
	classUniversal   = 0x00
	classContext     = 0x80
	berConstructed   = 0x20
	tagMaskLow       = 0x1f
	indefiniteLength = -1
)

// Universal tags the APDUs use.
const (
	tagInteger       = 2
	tagOID           = 6
	tagExternal      = 8
	tagSequence      = 16
	tagVisibleString = 26
	tagGeneralString = 27
)

var errIncomplete = errors.New("z3950: incomplete BER element")

// ---- writer ----

// appendTag appends a BER identifier octet (or octets, for tag numbers above 30).
func appendTag(dst []byte, class byte, constructed bool, tag uint32) []byte {
	head := class
	if constructed {
		head |= berConstructed
	}
	if tag <= 30 {
		return append(dst, head|byte(tag))
	}
	dst = append(dst, head|tagMaskLow)
	// Base-128, big-endian, high bit set on all but the last octet.
	var tmp [5]byte
	n := 0
	for v := tag; ; {
		tmp[n] = byte(v & 0x7f)
		n++
		v >>= 7
		if v == 0 {
			break
		}
	}
	for i := n - 1; i > 0; i-- {
		dst = append(dst, tmp[i]|0x80)
	}
	return append(dst, tmp[0])
}

// appendLength appends a definite BER length.
func appendLength(dst []byte, n int) []byte {
	if n < 0x80 {
		return append(dst, byte(n))
	}
	var tmp [8]byte
	i := len(tmp)
	for v := n; v > 0; v >>= 8 {
		i--
		tmp[i] = byte(v)
	}
	dst = append(dst, 0x80|byte(len(tmp)-i))
	return append(dst, tmp[i:]...)
}

// appendElem appends a complete element with the given content.
func appendElem(dst []byte, class byte, constructed bool, tag uint32, content []byte) []byte {
	dst = appendTag(dst, class, constructed, tag)
	dst = appendLength(dst, len(content))
	return append(dst, content...)
}

// appendInt appends a primitive INTEGER with a context (or universal) tag,
// minimally encoded two's complement.
func appendInt(dst []byte, class byte, tag uint32, v int64) []byte {
	var tmp [9]byte
	n := len(tmp)
	for {
		n--
		tmp[n] = byte(v)
		v >>= 8
		if (v == 0 && tmp[n]&0x80 == 0) || (v == -1 && tmp[n]&0x80 != 0) {
			break
		}
	}
	return appendElem(dst, class, false, tag, tmp[n:])
}

// appendBool appends a primitive BOOLEAN.
func appendBool(dst []byte, class byte, tag uint32, v bool) []byte {
	b := byte(0x00)
	if v {
		b = 0xff
	}
	return appendElem(dst, class, false, tag, []byte{b})
}

// appendString appends a primitive string-valued element.
func appendString(dst []byte, class byte, tag uint32, s string) []byte {
	return appendElem(dst, class, false, tag, []byte(s))
}

// appendBits appends a primitive BIT STRING whose set bits are listed (bit 0 is
// the most significant bit of the first content octet, per X.690).
func appendBits(dst []byte, class byte, tag uint32, bits ...int) []byte {
	maxBit := 0
	for _, b := range bits {
		if b > maxBit {
			maxBit = b
		}
	}
	content := make([]byte, 1+maxBit/8+1)
	for _, b := range bits {
		content[1+b/8] |= 0x80 >> (b % 8)
	}
	content[0] = byte(7 - maxBit%8) // unused bits in the final octet
	return appendElem(dst, class, false, tag, content)
}

// appendOID appends an OBJECT IDENTIFIER (universal class unless overridden).
func appendOID(dst []byte, class byte, tag uint32, oid []uint32) []byte {
	var content []byte
	content = appendBase128(content, oid[0]*40+oid[1])
	for _, arc := range oid[2:] {
		content = appendBase128(content, arc)
	}
	return appendElem(dst, class, false, tag, content)
}

func appendBase128(dst []byte, v uint32) []byte {
	var tmp [5]byte
	n := 0
	for {
		tmp[n] = byte(v & 0x7f)
		n++
		v >>= 7
		if v == 0 {
			break
		}
	}
	for i := n - 1; i > 0; i-- {
		dst = append(dst, tmp[i]|0x80)
	}
	return append(dst, tmp[0])
}

// ---- reader ----

// berValue is one decoded element: its identifier and raw content octets. For an
// indefinite-length element the content excludes the end-of-contents marker.
type berValue struct {
	class       byte
	tag         uint32
	constructed bool
	content     []byte
}

// berHeader decodes an identifier + length header, returning the header size and
// the content length (indefiniteLength for an indefinite length).
func berHeader(b []byte) (class byte, tag uint32, constructed bool, hdr, length int, err error) {
	if len(b) < 2 {
		return 0, 0, false, 0, 0, errIncomplete
	}
	class = b[0] & 0xc0
	constructed = b[0]&berConstructed != 0
	i := 1
	if b[0]&tagMaskLow != tagMaskLow {
		tag = uint32(b[0] & tagMaskLow)
	} else {
		for {
			if i >= len(b) {
				return 0, 0, false, 0, 0, errIncomplete
			}
			if tag > 1<<24 {
				return 0, 0, false, 0, 0, fmt.Errorf("z3950: BER tag overflow")
			}
			tag = tag<<7 | uint32(b[i]&0x7f)
			done := b[i]&0x80 == 0
			i++
			if done {
				break
			}
		}
	}
	if i >= len(b) {
		return 0, 0, false, 0, 0, errIncomplete
	}
	switch l := b[i]; {
	case l < 0x80:
		return class, tag, constructed, i + 1, int(l), nil
	case l == 0x80:
		return class, tag, constructed, i + 1, indefiniteLength, nil
	default:
		n := int(l & 0x7f)
		if n > 4 {
			return 0, 0, false, 0, 0, fmt.Errorf("z3950: BER length too large")
		}
		if i+1+n > len(b) {
			return 0, 0, false, 0, 0, errIncomplete
		}
		length = 0
		for _, c := range b[i+1 : i+1+n] {
			length = length<<8 | int(c)
		}
		return class, tag, constructed, i + 1 + n, length, nil
	}
}

// berSize returns the total encoded size of the first element in b, resolving
// indefinite lengths by scanning nested elements to their end-of-contents. It
// returns errIncomplete when b does not yet hold a whole element, letting the
// transport read more bytes.
func berSize(b []byte) (int, error) {
	_, _, constructed, hdr, length, err := berHeader(b)
	if err != nil {
		return 0, err
	}
	if length != indefiniteLength {
		if hdr+length > len(b) {
			return 0, errIncomplete
		}
		return hdr + length, nil
	}
	if !constructed {
		return 0, fmt.Errorf("z3950: indefinite length on primitive element")
	}
	i := hdr
	for {
		if i+2 <= len(b) && b[i] == 0 && b[i+1] == 0 {
			return i + 2, nil
		}
		n, err := berSize(b[i:])
		if err != nil {
			return 0, err
		}
		i += n
	}
}

// berParse decodes the first element in b, returning it and the remaining bytes.
func berParse(b []byte) (berValue, []byte, error) {
	total, err := berSize(b)
	if err != nil {
		return berValue{}, nil, err
	}
	class, tag, constructed, hdr, length, err := berHeader(b)
	if err != nil {
		return berValue{}, nil, err
	}
	v := berValue{class: class, tag: tag, constructed: constructed}
	if length == indefiniteLength {
		v.content = b[hdr : total-2] // strip the end-of-contents octets
	} else {
		v.content = b[hdr:total]
	}
	return v, b[total:], nil
}

// children decodes a constructed element's content as a list of elements.
func (v berValue) children() ([]berValue, error) {
	if !v.constructed {
		return nil, fmt.Errorf("z3950: primitive element has no children (tag %d)", v.tag)
	}
	var out []berValue
	rest := v.content
	for len(rest) > 0 {
		child, r, err := berParse(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
		rest = r
	}
	return out, nil
}

// intVal decodes a primitive INTEGER content.
func (v berValue) intVal() (int64, error) {
	if len(v.content) == 0 || len(v.content) > 8 {
		return 0, fmt.Errorf("z3950: bad INTEGER length %d", len(v.content))
	}
	n := int64(0)
	if v.content[0]&0x80 != 0 {
		n = -1
	}
	for _, c := range v.content {
		n = n<<8 | int64(c)
	}
	return n, nil
}

// boolVal decodes a primitive BOOLEAN content.
func (v berValue) boolVal() bool { return len(v.content) > 0 && v.content[0] != 0 }

// stringVal returns the content as a string (Z39.50 InternationalString).
func (v berValue) stringVal() string { return string(v.content) }

// oidVal decodes an OBJECT IDENTIFIER content.
func (v berValue) oidVal() ([]uint32, error) {
	if len(v.content) == 0 {
		return nil, fmt.Errorf("z3950: empty OID")
	}
	var arcs []uint32
	var cur uint32
	for i, c := range v.content {
		cur = cur<<7 | uint32(c&0x7f)
		if c&0x80 == 0 {
			if len(arcs) == 0 {
				first := min(cur/40, 2)
				arcs = append(arcs, first, cur-first*40)
			} else {
				arcs = append(arcs, cur)
			}
			cur = 0
		} else if i == len(v.content)-1 {
			return nil, fmt.Errorf("z3950: truncated OID")
		}
	}
	return arcs, nil
}

// oidEqual reports whether an OID equals the given arcs.
func oidEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
