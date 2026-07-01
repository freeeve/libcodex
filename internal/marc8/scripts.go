package marc8

//go:generate go run ./gen

import (
	"encoding/base64"
	"sort"
	"sync"
)

// charKind classifies how a designated working set turns bytes into runes.
type charKind int

const (
	kindASCII       charKind = iota // Basic Latin: a GL byte is its own code point
	kindANSEL                       // Extended Latin: spacing graphics + combining marks
	kindSingle                      // a single-byte non-Latin set, via a lookup map
	kindEACC                        // the multibyte CJK set (EACC)
	kindUnsupported                 // an unknown designation; bytes pass through best-effort
)

// charSet describes one MARC-8 graphic character set and how to translate it.
type charSet struct {
	name   string
	final  byte // ISO 2022 final byte identifying the set in an escape sequence
	kind   charKind
	homeG1 bool          // working set the LoC table codes belong to (false = G0/GL)
	dec    map[byte]rune // kindSingle: MARC byte (in home range) -> rune

	encOnce sync.Once
	enc     map[rune]byte // kindSingle: rune -> MARC byte (home range), built lazily
}

// The supported character sets. ASCII and ANSEL keep their hand-maintained
// tables (in marc8.go); the rest use the generated decode maps.
var (
	csASCII = &charSet{name: "Basic Latin (ASCII)", final: 'B', kind: kindASCII}
	csANSEL = &charSet{name: "Extended Latin (ANSEL)", final: 'E', kind: kindANSEL, homeG1: true}
	csEACC  = &charSet{name: "Chinese, Japanese, Korean (EACC)", final: '1', kind: kindEACC}

	csBasicHebrew   = single("Basic Hebrew", '2', false, basicHebrewDec)
	csBasicCyrillic = single("Basic Cyrillic", 'N', false, basicCyrillicDec)
	csExtCyrillic   = single("Extended Cyrillic", 'Q', true, extendedCyrillicDec)
	csBasicArabic   = single("Basic Arabic", '3', false, basicArabicDec)
	csExtArabic     = single("Extended Arabic", '4', true, extendedArabicDec)
	csBasicGreek    = single("Basic Greek", 'S', false, basicGreekDec)
	csGreekSymbols  = single("Greek Symbols", 'g', false, greekSymbolsDec)
	csSubscripts    = single("Subscripts", 'b', false, subscriptsDec)
	csSuperscripts  = single("Superscripts", 'p', false, superscriptsDec)

	// csUnsupported is the sentinel for an unrecognized designation.
	csUnsupported = &charSet{name: "unsupported", kind: kindUnsupported}
)

func single(name string, final byte, homeG1 bool, dec map[byte]rune) *charSet {
	return &charSet{name: name, final: final, kind: kindSingle, homeG1: homeG1, dec: dec}
}

// setByFinal returns the character set an escape sequence designates, given its
// final byte and whether the sequence marked a multibyte set, or nil if unknown.
func setByFinal(final byte, multibyte bool) *charSet {
	if multibyte {
		if final == csEACC.final {
			return csEACC
		}
		return nil
	}
	switch final {
	case 'B':
		return csASCII
	case 'E':
		return csANSEL
	case '2':
		return csBasicHebrew
	case 'N':
		return csBasicCyrillic
	case 'Q':
		return csExtCyrillic
	case '3':
		return csBasicArabic
	case '4':
		return csExtArabic
	case 'S':
		return csBasicGreek
	case 'g':
		return csGreekSymbols
	case 'b':
		return csSubscripts
	case 'p':
		return csSuperscripts
	case 's':
		// ESC s reinstates Basic Latin (ASCII) in G0 -- the technique-1 escape a
		// record uses to return to ASCII after a single-character designation.
		return csASCII
	}
	return nil
}

// decodeChar decodes the next character from data, whose first byte belongs to
// this set's invocation range (gl reports the GL range). It returns the rune, the
// number of bytes consumed, and whether the byte(s) were defined in the set. An
// undefined byte yields a best-effort Latin-1 rune (or U+FFFD for an unmapped
// EACC triple) with ok=false, so the caller can flag the decode lossy.
func (cs *charSet) decodeChar(data []byte, gl bool) (rune, int, bool) {
	c := data[0]
	switch cs.kind {
	case kindANSEL:
		cc := c
		if gl {
			cc = c | 0x80 // ANSEL designated to G0 (non-standard): normalize to home range
		}
		if r, ok := anselRune(cc); ok {
			return r, 1, true
		}
		return rune(c), 1, false
	case kindSingle:
		if r, ok := cs.lookup(c); ok {
			return r, 1, true
		}
		return rune(c), 1, false
	case kindEACC:
		r, n := decodeEACC(data)
		return r, n, r != 0xFFFD
	default: // kindASCII and kindUnsupported: the byte is its own code point
		return rune(c), 1, cs.kind == kindASCII
	}
}

// lookup decodes byte b (as it appeared in the stream) through a single-byte set,
// normalizing it to the set's home range so a set works whether it was designated
// to G0 or G1.
func (cs *charSet) lookup(b byte) (rune, bool) {
	if cs.homeG1 {
		b |= 0x80
	} else {
		b &^= 0x80
	}
	r, ok := cs.dec[b]
	return r, ok
}

// encByte returns the MARC byte for rune r in a single-byte set, building the
// inverse map on first use. When several bytes map to the same rune the lowest
// wins, so encoding is deterministic.
func (cs *charSet) encByte(r rune) (byte, bool) {
	cs.encOnce.Do(func() {
		cs.enc = make(map[rune]byte, len(cs.dec))
		keys := make([]int, 0, len(cs.dec))
		for b := range cs.dec {
			keys = append(keys, int(b))
		}
		sort.Ints(keys)
		for _, k := range keys {
			rr := cs.dec[byte(k)]
			if _, ok := cs.enc[rr]; !ok {
				cs.enc[rr] = byte(k)
			}
		}
	})
	b, ok := cs.enc[r]
	return b, ok
}

// encSingleSets is the order single-byte non-Latin sets are tried when encoding,
// so a rune present in more than one set (e.g. a Greek letter in both Basic Greek
// and Greek Symbols) maps deterministically.
var encSingleSets = []*charSet{
	csBasicCyrillic, csExtCyrillic, csBasicArabic, csExtArabic,
	csBasicHebrew, csBasicGreek, csGreekSymbols, csSubscripts, csSuperscripts,
}

// encodeSingle finds the first single-byte set that can encode r.
func encodeSingle(r rune) (*charSet, byte, bool) {
	for _, set := range encSingleSets {
		if b, ok := set.encByte(r); ok {
			return set, b, true
		}
	}
	return nil, 0, false
}

// ---- EACC (multibyte CJK) ----

const eaccRec = 6 // packed record: 3-byte MARC code + 3-byte Unicode scalar

// eaccBlob is the decoded EACC table, sorted by MARC code, decoded once at init.
var eaccBlob = func() []byte {
	b, err := base64.StdEncoding.DecodeString(eaccBlobB64)
	if err != nil {
		panic("marc8: corrupt EACC blob: " + err.Error())
	}
	return b
}()

// eaccCodeAt reads the MARC code of record i.
func eaccCodeAt(i int) uint32 {
	r := eaccBlob[i*eaccRec:]
	return uint32(r[0])<<16 | uint32(r[1])<<8 | uint32(r[2])
}

// eaccDecode maps a 3-byte EACC code (each byte in the 0x21-0x7E home range) to
// a rune via binary search.
func eaccDecode(code uint32) (rune, bool) {
	n := len(eaccBlob) / eaccRec
	i := sort.Search(n, func(i int) bool { return eaccCodeAt(i) >= code })
	if i < n && eaccCodeAt(i) == code {
		r := eaccBlob[i*eaccRec:]
		return rune(uint32(r[3])<<16 | uint32(r[4])<<8 | uint32(r[5])), true
	}
	return 0, false
}

var (
	eaccEncOnce sync.Once
	eaccEnc     map[rune]uint32
)

// eaccEncode maps a rune to its EACC code, building the inverse map on first use.
// The blob is sorted by code, so the lowest code wins for a rune with variants.
func eaccEncode(r rune) (uint32, bool) {
	eaccEncOnce.Do(func() {
		n := len(eaccBlob) / eaccRec
		eaccEnc = make(map[rune]uint32, n)
		for i := range n {
			rec := eaccBlob[i*eaccRec:]
			rr := rune(uint32(rec[3])<<16 | uint32(rec[4])<<8 | uint32(rec[5]))
			if _, ok := eaccEnc[rr]; !ok {
				eaccEnc[rr] = eaccCodeAt(i)
			}
		}
	})
	c, ok := eaccEnc[r]
	return c, ok
}
