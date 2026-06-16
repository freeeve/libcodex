package codex

import "strings"

// Linkage is the parsed content of a subfield $6, which ties a field to its
// alternate-script equivalent in field 880. In a regular field $6 points at the
// 880 occurrence ("880-01"); in the 880 field it points back at the regular tag
// ("245-01/(N/r"), optionally carrying a script identification code and a
// right-to-left field orientation.
type Linkage struct {
	Tag         string // the linked field's tag (e.g. "880" in a regular field)
	Occurrence  string // two-digit occurrence number; "00" means no linked field
	Script      string // script identification code (e.g. "(N" Cyrillic, "$1" CJK), or ""
	RightToLeft bool   // field orientation code 'r'
}

// ScriptName returns a human-readable name for the $6 script identification code,
// or "" if absent or unrecognized. The codes are the MARC-8 set designations.
func (l Linkage) ScriptName() string {
	switch l.Script {
	case "(3":
		return "Arabic"
	case "(B":
		return "Latin"
	case "$1":
		return "CJK"
	case "(N":
		return "Cyrillic"
	case "(S":
		return "Greek"
	case "(2":
		return "Hebrew"
	default:
		return ""
	}
}

// Linked reports whether the linkage refers to an actual partner field (a nonzero
// occurrence number).
func (l Linkage) Linked() bool { return l.Occurrence != "" && l.Occurrence != "00" }

// Link parses the field's subfield $6 linkage, returning false if it has none or
// the linkage is malformed.
func (f Field) Link() (Linkage, bool) {
	v, ok := f.Subfield('6')
	if !ok {
		return Linkage{}, false
	}
	parts := strings.Split(v, "/")
	tagOcc := parts[0]
	// The reference is "TAG-OO": a 3-character tag, a hyphen, two occurrence digits.
	if len(tagOcc) < 6 || tagOcc[3] != '-' {
		return Linkage{}, false
	}
	l := Linkage{Tag: tagOcc[:3], Occurrence: tagOcc[4:6]}
	for _, p := range parts[1:] {
		switch {
		case p == "r":
			l.RightToLeft = true
		case len(p) >= 2 && (p[0] == '(' || p[0] == '$'):
			l.Script = p
		}
	}
	return l, true
}

// AlternateGraphic returns the field linked to f through subfield $6: the 880
// field when f is a regular field, or the regular field when f is an 880. It
// matches on the tag and occurrence number and returns false when f carries no
// linkage or no partner is present.
func (r *Record) AlternateGraphic(f Field) (Field, bool) {
	link, ok := f.Link()
	if !ok || !link.Linked() {
		return Field{}, false
	}
	for _, g := range r.Fields() {
		gl, ok := g.Link()
		if !ok || gl.Occurrence != link.Occurrence {
			continue
		}
		if f.Tag == "880" {
			if g.Tag == link.Tag && gl.Tag == "880" {
				return g, true
			}
		} else if g.Tag == "880" && gl.Tag == f.Tag {
			return g, true
		}
	}
	return Field{}, false
}

// Vernacular returns subfield code from the 880 field linked to the first field
// with the given tag, or "" if there is no linked alternate-script field. It is a
// shortcut for the common case of reading an original-script title or name.
func (r *Record) Vernacular(tag string, code byte) string {
	for _, f := range r.Fields() {
		if f.Tag == tag {
			if alt, ok := r.AlternateGraphic(f); ok {
				return alt.SubfieldValue(code)
			}
		}
	}
	return ""
}
