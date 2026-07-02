package z3950

import (
	"testing"
)

// encodeOPACExternalT builds an OPAC EXTERNAL wrapping a MARC bibliographic
// record and one holding. explicitBib selects whether bibliographicRecord [1]
// wraps the inner EXTERNAL explicitly or replaces its tag implicitly -- servers
// emit both.
func encodeOPACExternalT(marc []byte, explicitBib bool) []byte {
	// Inner bibliographic EXTERNAL (marc21, octet-aligned).
	var bibExt []byte
	bibExt = appendOID(bibExt, classUniversal, tagOID, oidMARC21)
	bibExt = appendElem(bibExt, classContext, false, 1, marc)
	bibExtEl := appendElem(nil, classUniversal, true, tagExternal, bibExt)

	var bib []byte
	if explicitBib {
		bib = appendElem(nil, classContext, true, 1, bibExtEl)
	} else {
		bib = appendElem(nil, classContext, true, 1, bibExt) // implicit: content only
	}

	// One CircRecord.
	var circ []byte
	circ = appendBool(circ, classContext, 1, true)        // availableNow
	circ = appendString(circ, classContext, 5, "item-42") // itemId
	circ = appendBool(circ, classContext, 6, false)       // renewable
	circ = appendBool(circ, classContext, 7, false)       // onHold
	circSeq := appendElem(nil, classUniversal, true, tagSequence, circ)
	circList := appendElem(nil, classContext, true, 19, circSeq)

	// HoldingsAndCircData [2].
	var hold []byte
	hold = appendString(hold, classContext, 8, "s-FM/GC")           // nucCode
	hold = appendString(hold, classContext, 9, "Main Reading Room") // localLocation
	hold = appendString(hold, classContext, 11, "PS3556 .E446")     // callNumber
	hold = appendString(hold, classContext, 13, "c.2")              // copyNumber
	hold = appendString(hold, classContext, 17, "v.1 1993")         // enumAndChron
	hold = append(hold, circList...)
	holdEl := appendElem(nil, classContext, true, 2, hold)
	holdings := appendElem(nil, classContext, true, 2, holdEl)

	// OPACRecord SEQUENCE inside the outer EXTERNAL's single-ASN1-type [0].
	var opac []byte
	opac = append(opac, bib...)
	opac = append(opac, holdings...)
	opacSeq := appendElem(nil, classUniversal, true, tagSequence, opac)

	var ext []byte
	ext = appendOID(ext, classUniversal, tagOID, oidOPAC)
	ext = appendElem(ext, classContext, true, 0, opacSeq)
	return appendElem(nil, classUniversal, true, tagExternal, ext)
}

// TestOPACRecordParse covers the OPAC unwrap for both bibliographicRecord
// tagging variants: the embedded MARC decodes transparently and the holdings
// carry location, call number and circulation.
func TestOPACRecordParse(t *testing.T) {
	marc := sampleMARC(t, "op01", "Held Title")
	for name, explicit := range map[string]bool{"explicit-external": true, "implicit-external": false} {
		t.Run(name, func(t *testing.T) {
			ext, _, err := berParse(encodeOPACExternalT(marc, explicit))
			if err != nil {
				t.Fatal(err)
			}
			rec, err := parseExternal(ext)
			if err != nil {
				t.Fatalf("parseExternal: %v", err)
			}
			if rec.Syntax != "marc21" {
				t.Errorf("syntax = %q, want marc21 (unwrapped bib)", rec.Syntax)
			}
			dec, err := rec.Decode()
			if err != nil || dec.SubfieldValue("245", 'a') != "Held Title" {
				t.Errorf("bib decode: %v (%+v)", err, dec)
			}
			if len(rec.Holdings) != 1 {
				t.Fatalf("holdings = %+v, want 1", rec.Holdings)
			}
			h := rec.Holdings[0]
			if h.NUCCode != "s-FM/GC" || h.LocalLocation != "Main Reading Room" ||
				h.CallNumber != "PS3556 .E446" || h.CopyNumber != "c.2" || h.EnumAndChron != "v.1 1993" {
				t.Errorf("holding = %+v", h)
			}
			if len(h.Circulation) != 1 || !h.Circulation[0].AvailableNow || h.Circulation[0].ItemID != "item-42" {
				t.Errorf("circulation = %+v", h.Circulation)
			}
		})
	}
}

// TestOPACWithoutBib checks an OPAC record with holdings but no embedded
// bibliographic record stays syntax "opac" and never fails.
func TestOPACWithoutBib(t *testing.T) {
	var hold []byte
	hold = appendString(hold, classContext, 11, "X 123")
	holdEl := appendElem(nil, classContext, true, 2, hold)
	holdings := appendElem(nil, classContext, true, 2, holdEl)
	opacSeq := appendElem(nil, classUniversal, true, tagSequence, holdings)
	var ext []byte
	ext = appendOID(ext, classUniversal, tagOID, oidOPAC)
	ext = appendElem(ext, classContext, true, 0, opacSeq)

	v, _, err := berParse(appendElem(nil, classUniversal, true, tagExternal, ext))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := parseExternal(v)
	if err != nil {
		t.Fatalf("parseExternal: %v", err)
	}
	if rec.Syntax != "opac" || len(rec.Holdings) != 1 || rec.Holdings[0].CallNumber != "X 123" {
		t.Errorf("record = %+v", rec)
	}
	if _, err := rec.Decode(); err == nil {
		t.Error("Decode without an embedded bib should error")
	}
}
