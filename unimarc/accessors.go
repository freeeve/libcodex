package unimarc

import (
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
)

// Title returns the title proper and other title information (UNIMARC 200 $a $e).
func Title(r *codex.Record) string {
	if f, ok := r.DataField("200"); ok {
		return clean(crosswalk.JoinSub(f, "ae", " "))
	}
	return ""
}

// Authors returns the personal and corporate names with intellectual
// responsibility (UNIMARC 700/701/702 personal, 710/711/712 corporate) in a
// single pass over the record's fields.
func Authors(r *codex.Record) []string {
	var out []string
	for _, f := range r.Fields() {
		switch f.Tag {
		case "700", "701", "702", "710", "711", "712", "720", "721":
			if name := personName(f); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// personName joins the entry element (UNIMARC $a) with the rest of the name
// ($b). $b conventionally carries any separating punctuation, but ISO-2709
// UNIMARC records frequently omit it: when the second indicator marks a
// surname-first (inverted) entry and $b has no leading comma, the surname and
// forename are joined with ", " so downstream citation and BibTeX consumers read
// the name correctly.
func personName(f codex.Field) string {
	a := clean(strings.TrimRight(f.SubfieldValue('a'), " "))
	b := clean(strings.TrimSpace(f.SubfieldValue('b')))
	if a == "" {
		return ""
	}
	if b == "" {
		return a
	}
	if strings.HasPrefix(b, ",") {
		return a + b
	}
	if f.Ind2 == '1' { // entered under surname: "Surname, Forename"
		return a + ", " + b
	}
	return a + " " + b
}

// ISBN returns the ISBNs (UNIMARC 010 $a).
func ISBN(r *codex.Record) []string { return subValues(r, "010", 'a') }

// ISSN returns the ISSNs (UNIMARC 011 $a).
func ISSN(r *codex.Record) []string { return subValues(r, "011", 'a') }

// Language returns the language of the text (UNIMARC 101 $a), or "".
func Language(r *codex.Record) string {
	return clean(strings.TrimSpace(r.SubfieldValue("101", 'a')))
}

// Edition returns the edition statement (UNIMARC 205 $a).
func Edition(r *codex.Record) string {
	return clean(strings.TrimSpace(r.SubfieldValue("205", 'a')))
}

// Publisher returns the publisher name (UNIMARC 210 $c, or the newer 214 $c).
func Publisher(r *codex.Record) string {
	if v := r.SubfieldValue("210", 'c'); v != "" {
		return clean(strings.TrimRight(v, " ,"))
	}
	return clean(strings.TrimRight(r.SubfieldValue("214", 'c'), " ,"))
}

// PublicationDate returns the date of publication (UNIMARC 210 $d, or 214 $d).
func PublicationDate(r *codex.Record) string {
	if v := r.SubfieldValue("210", 'd'); v != "" {
		return clean(strings.TrimSpace(v))
	}
	return clean(strings.TrimSpace(r.SubfieldValue("214", 'd')))
}

// Subjects returns the subject access points (UNIMARC 600/601 names used as
// subjects, 606 topical, 607 geographic, 608 form/genre) in a single pass over
// the record's fields.
func Subjects(r *codex.Record) []string {
	var out []string
	for _, f := range r.Fields() {
		switch f.Tag {
		case "600", "601", "602", "604", "605", "606", "607", "608":
			if v := clean(crosswalk.JoinSub(f, "axyz", "--")); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// ---- helpers ----

func subValues(r *codex.Record, tag string, code byte) []string {
	var out []string
	for _, f := range r.DataFields(tag) {
		for _, s := range f.Subfields {
			if s.Code == code {
				if v := clean(strings.TrimSpace(s.Value)); v != "" {
					out = append(out, v)
				}
			}
		}
	}
	return out
}
