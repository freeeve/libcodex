package codex

// Control008 provides typed access to the 008 fixed-length data elements. The
// header positions (00-17) and the trailing positions (35-39) are common to every
// material type; the material-specific block (18-34) is interpreted using the
// record's leader, which is why FormOfItem needs it.
//
// MARC 21 fixed fields are positional, so an accessor returns the empty string or
// a zero byte when the underlying 008 is too short to contain the position,
// rather than panicking.
type Control008 struct {
	raw        string
	recordType byte // leader byte 6
}

// Control008 returns the parsed 008 control field, or false if the record has no
// 008 or one too short to hold the common header (18 characters).
func (r *Record) Control008() (Control008, bool) {
	v := r.ControlField("008")
	if len(v) < 18 {
		return Control008{}, false
	}
	return Control008{raw: v, recordType: r.Leader().RecordType()}, true
}

// String returns the raw 008 value.
func (c Control008) String() string { return c.raw }

// at returns the byte at position i, or 0 if out of range.
func (c Control008) at(i int) byte {
	if i < 0 || i >= len(c.raw) {
		return 0
	}
	return c.raw[i]
}

// slice returns raw[start:end], or "" if the range is not fully present.
func (c Control008) slice(start, end int) string {
	if start < 0 || end > len(c.raw) || start >= end {
		return ""
	}
	return c.raw[start:end]
}

// DateEntered returns positions 00-05, the date the record was created (yymmdd).
func (c Control008) DateEntered() string { return c.slice(0, 6) }

// DateType returns position 06, the type of date / publication status code.
func (c Control008) DateType() byte { return c.at(6) }

// Date1 returns positions 07-10, the first date (often the publication year).
func (c Control008) Date1() string { return c.slice(7, 11) }

// Date2 returns positions 11-14, the second date.
func (c Control008) Date2() string { return c.slice(11, 15) }

// Place returns positions 15-17, the place of publication, production or
// execution code.
func (c Control008) Place() string { return c.slice(15, 18) }

// Language returns positions 35-37, the language code (MARC Code List for
// Languages / ISO 639-2/B), or "" if absent.
func (c Control008) Language() string { return c.slice(35, 38) }

// CatalogingSource returns position 39, the cataloging source code.
func (c Control008) CatalogingSource() byte { return c.at(39) }

// formOfItemPos returns the 008 position holding the form of item for this
// record's material type. Maps and visual materials carry it at 29; every other
// type carries it at 23.
func (c Control008) formOfItemPos() int {
	switch c.recordType {
	case 'e', 'f', 'g', 'k', 'o', 'r': // cartographic and visual materials
		return 29
	default:
		return 23
	}
}

// FormOfItem returns the form-of-item code from the material-specific block: the
// byte at position 23 for most materials, or 29 for cartographic and visual
// materials. Codes include 'd' (large print), 'f' (braille), 'o' (online),
// 'q' (direct electronic) and 's' (electronic); see [Control008.IsLargePrint]
// and [Control008.IsBraille] for the accessibility-relevant ones.
func (c Control008) FormOfItem() byte { return c.at(c.formOfItemPos()) }

// IsLargePrint reports whether the form of item is large print (code 'd').
func (c Control008) IsLargePrint() bool { return c.FormOfItem() == 'd' }

// IsBraille reports whether the form of item is braille (code 'f').
func (c Control008) IsBraille() bool { return c.FormOfItem() == 'f' }
