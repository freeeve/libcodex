package codex

// Accessibility summarizes the accessibility metadata a record carries: the 008
// form of item, the 007 physical-description category, the 341 Accessibility
// Content field and the 532 Accessibility Note field. It is the MARC-side view a
// crosswalk (e.g. to schema.org accessibility properties) can map from.
type Accessibility struct {
	LargePrint  bool     // 008 form of item 'd'
	Braille     bool     // 008 form of item 'f'
	Tactile     bool     // 007 category of material 'f' (tactile material)
	AccessModes []string // 341 $a content access modes (textual, visual, auditory, tactile)
	Features    []string // 341 $b-$e assistive features (textual, visual, auditory, tactile)
	Notes       []string // 532 $a accessibility notes
}

// Empty reports whether the record carried no accessibility metadata at all.
func (a Accessibility) Empty() bool {
	return !a.LargePrint && !a.Braille && !a.Tactile &&
		len(a.AccessModes) == 0 && len(a.Features) == 0 && len(a.Notes) == 0
}

// Accessibility gathers the record's accessibility metadata from the 008, 007,
// 341 and 532 fields. The result is always valid (its zero value means none was
// found); see [Accessibility.Empty].
func (r *Record) Accessibility() Accessibility {
	var a Accessibility
	if c, ok := r.Control008(); ok {
		a.LargePrint = c.IsLargePrint()
		a.Braille = c.IsBraille()
	}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "007":
			if len(f.Value) > 0 && f.Value[0] == 'f' {
				a.Tactile = true
			}
		case "341":
			a.AccessModes = append(a.AccessModes, f.SubfieldValues('a')...)
			for _, code := range []byte{'b', 'c', 'd', 'e'} {
				a.Features = append(a.Features, f.SubfieldValues(code)...)
			}
		case "532":
			a.Notes = append(a.Notes, f.SubfieldValues('a')...)
		}
	}
	return a
}
