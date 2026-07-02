package z3950

import (
	"bytes"
	"testing"
)

// TestBerIntRoundTrip covers INTEGER encoding across sign and width boundaries.
func TestBerIntRoundTrip(t *testing.T) {
	for _, v := range []int64{0, 1, 127, 128, 255, 256, 1 << 20, 1 << 24, -1, -128, -129} {
		b := appendInt(nil, classContext, 5, v)
		parsed, rest, err := berParse(b)
		if err != nil || len(rest) != 0 {
			t.Fatalf("parse %d: %v (rest %d)", v, err, len(rest))
		}
		got, err := parsed.intVal()
		if err != nil || got != v {
			t.Errorf("int %d round-tripped to %d (%v)", v, got, err)
		}
	}
}

// TestBerHighTag covers the multi-byte tag numbers Z39.50 uses (105, 110-112,
// 120-121, 130, 205, 211).
func TestBerHighTag(t *testing.T) {
	for _, tag := range []uint32{30, 31, 105, 120, 130, 205, 211, 16383, 16384} {
		b := appendString(nil, classContext, tag, "x")
		parsed, _, err := berParse(b)
		if err != nil {
			t.Fatalf("tag %d: %v", tag, err)
		}
		if parsed.tag != tag || parsed.class != classContext {
			t.Errorf("tag %d parsed as %d (class %#x)", tag, parsed.tag, parsed.class)
		}
	}
}

// TestBerOIDRoundTrip covers the record-syntax and attribute-set OIDs.
func TestBerOIDRoundTrip(t *testing.T) {
	for _, oid := range [][]uint32{oidMARC21, oidXML, oidBib1Attributes, {2, 16, 840}} {
		b := appendOID(nil, classUniversal, tagOID, oid)
		parsed, _, err := berParse(b)
		if err != nil {
			t.Fatalf("oid %v: %v", oid, err)
		}
		got, err := parsed.oidVal()
		if err != nil || !oidEqual(got, oid) {
			t.Errorf("oid %v round-tripped to %v (%v)", oid, got, err)
		}
	}
}

// TestBerBits checks BIT STRING bit placement (bit 0 = MSB of first octet).
func TestBerBits(t *testing.T) {
	b := appendBits(nil, classContext, 3, 0, 1, 2)
	parsed, _, err := berParse(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed.content, []byte{5, 0xe0}) {
		t.Errorf("bits {0,1,2} = %x, want 05e0", parsed.content)
	}
}

// TestBerIndefiniteLength checks that a constructed element using an indefinite
// length (as some Z39.50 servers emit) parses identically to its definite form,
// including when nested.
func TestBerIndefiniteLength(t *testing.T) {
	inner := appendString(nil, classContext, 17, "default")
	// Definite form.
	definite := appendElem(nil, classContext, true, pduSearchRequest, inner)
	// Indefinite form: tag, 0x80, content, 00 00. Nested: wrap the same content
	// in an inner indefinite element too.
	var indef []byte
	indef = appendTag(indef, classContext, true, pduSearchRequest)
	indef = append(indef, 0x80)
	indef = append(indef, inner...)
	indef = append(indef, 0, 0)

	for _, b := range [][]byte{definite, indef} {
		v, rest, err := berParse(b)
		if err != nil || len(rest) != 0 {
			t.Fatalf("parse: %v", err)
		}
		kids, err := v.children()
		if err != nil || len(kids) != 1 || kids[0].stringVal() != "default" {
			t.Errorf("children = %+v (%v)", kids, err)
		}
	}

	// Nested indefinite inside indefinite.
	var nested []byte
	nested = appendTag(nested, classContext, true, 1)
	nested = append(nested, 0x80)
	nested = append(nested, indef...)
	nested = append(nested, 0, 0)
	v, _, err := berParse(nested)
	if err != nil {
		t.Fatalf("nested: %v", err)
	}
	kids, err := v.children()
	if err != nil || len(kids) != 1 || kids[0].tag != pduSearchRequest {
		t.Errorf("nested children = %+v (%v)", kids, err)
	}
}

// TestBerIncomplete checks that truncated elements report errIncomplete rather
// than a hard error, so the transport keeps reading.
func TestBerIncomplete(t *testing.T) {
	full := encodeInitRequest(&Client{})
	for n := 1; n < len(full); n++ {
		if _, err := berSize(full[:n]); err != errIncomplete {
			t.Fatalf("prefix %d/%d: err = %v, want errIncomplete", n, len(full), err)
		}
	}
	if got, err := berSize(full); err != nil || got != len(full) {
		t.Errorf("full: %d, %v", got, err)
	}
}

// FuzzBerParse asserts the BER parser never panics and never over-reads on
// arbitrary bytes.
func FuzzBerParse(f *testing.F) {
	f.Add(encodeInitRequest(&Client{}))
	f.Add(encodePresentRequest(1, 10, oidMARC21))
	f.Add([]byte{0xbf, 0x87, 0x68, 0x80, 0x00, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		v, rest, err := berParse(data)
		if err != nil {
			return
		}
		if len(v.content)+len(rest) > len(data) {
			t.Fatal("parsed more bytes than provided")
		}
		if v.constructed {
			v.children()
		} else {
			v.intVal()
			v.oidVal()
		}
	})
}
