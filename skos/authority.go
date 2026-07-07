package skos

import "github.com/freeeve/libcodex"

// authorityLeader is a syntactically valid MARC authority leader (byte 6 = 'z',
// UTF-8 coding in byte 9). The codecs recompute the length [0:5] and base address
// [12:17] on write.
const authorityLeader = "00000nz  a2200000n  4500"

// Record crosswalks the concept to a MARC 21 authority record: the concept IRI
// is a 024 URI, the English preferred label the 150 established heading, other
// labels 450 see-from tracings, and broader/narrower/related concepts 550
// see-also tracings ($w g/h for the hierarchy) carrying the target's label and
// IRI. Scope notes and comments become 680 public notes.
func (c Concept) Record() *codex.Record {
	rec := codex.NewRecord().SetLeader(codex.Leader(authorityLeader))
	if c.ID != "" {
		rec.AddField(codex.NewControlField("001", c.ID))
	}
	if c.URI != "" {
		rec.AddField(codex.NewDataField("024", '7', ' ',
			codex.NewSubfield('a', c.URI), codex.NewSubfield('2', "uri")))
	}
	if heading := c.PrefLabel(); heading != "" {
		rec.AddField(codex.NewDataField("150", ' ', ' ', codex.NewSubfield('a', heading)))
	}
	// Non-English preferred labels and every alternate label are see-from tracings.
	for _, l := range c.Pref {
		if !isEnglish(l.Lang) && l.Text != "" {
			rec.AddField(seeFrom(l))
		}
	}
	for _, l := range c.Alt {
		if l.Text != "" {
			rec.AddField(seeFrom(l))
		}
	}
	for _, r := range c.Broader {
		rec.AddField(seeAlso(r, "g")) // $w/0 = g: the tracing is a broader term
	}
	for _, r := range c.Narrower {
		rec.AddField(seeAlso(r, "h")) // $w/0 = h: the tracing is a narrower term
	}
	for _, r := range c.Related {
		rec.AddField(seeAlso(r, "")) // no $w: an ordinary related term
	}
	for _, n := range c.Notes {
		if n.Text != "" {
			rec.AddField(codex.NewDataField("680", ' ', ' ', codex.NewSubfield('i', n.Text)))
		}
	}
	return rec
}

// Records crosswalks a slice of concepts to authority records, in order.
func Records(concepts []Concept) []*codex.Record {
	recs := make([]*codex.Record, len(concepts))
	for i, c := range concepts {
		recs[i] = c.Record()
	}
	return recs
}

// seeFrom builds a 450 topical see-from tracing from an alternate/other-language
// label, tagging the language in $9 when present.
func seeFrom(l Label) codex.Field {
	subs := []codex.Subfield{codex.NewSubfield('a', l.Text)}
	if l.Lang != "" {
		subs = append(subs, codex.NewSubfield('9', l.Lang))
	}
	return codex.NewDataField("450", ' ', ' ', subs...)
}

// seeAlso builds a 550 see-also tracing to a related concept: the control
// subfield $w (g broader / h narrower; empty for a plain related term) precedes
// the heading $a and the target IRI $0.
func seeAlso(r Ref, w string) codex.Field {
	var subs []codex.Subfield
	if w != "" {
		subs = append(subs, codex.NewSubfield('w', w))
	}
	heading := r.Label
	if heading == "" {
		heading = r.ID
	}
	subs = append(subs, codex.NewSubfield('a', heading))
	if r.URI != "" {
		subs = append(subs, codex.NewSubfield('0', r.URI))
	}
	return codex.NewDataField("550", ' ', ' ', subs...)
}
