package unimarc

import (
	"strings"

	"github.com/freeeve/libcodex"
)

// ToMARC21 re-tags a UNIMARC record to the MARC 21 model, mapping the common
// fields so the record can be fed to any libcodex exporter. The mapping covers
// title, statement of responsibility, edition, publication, physical description,
// series, notes, subjects, names and identifiers; UNIMARC fields outside that set
// are dropped. Values are cleaned of UNIMARC's non-sort control characters.
func ToMARC21(r *codex.Record) *codex.Record {
	out := codex.NewRecordCap(len(r.Fields()) + 1).SetLeader(marc21Leader(r.Leader()))
	if c := build008(r); c != "" {
		out.AddField(codex.NewControlField("008", c))
	}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "001":
			out.AddField(codex.NewControlField("001", clean(f.Value)))
		case "010":
			addRetag(out, f, "020", "a")
		case "011":
			addRetag(out, f, "022", "a")
		case "200":
			out.AddField(title245(f))
		case "205":
			addRetag(out, f, "250", "a")
		case "210":
			addPublication(out, f, "260")
		case "214":
			addPublication(out, f, "264")
		case "215":
			addRetag(out, f, "300", "aacbdcee") // extent, other details, dimensions, accompanying
		case "225":
			addRetag(out, f, "490", "aavvxx") // series title, volume, ISSN
		case "300":
			addRetag(out, f, "500", "a")
		case "330":
			addRetag(out, f, "520", "a")
		case "500":
			addRetag(out, f, "240", "a")
		case "600":
			addName(out, f, "600")
		case "601":
			addName(out, f, "610")
		case "606":
			addSubject(out, f, "650")
		case "607":
			addSubject(out, f, "651")
		case "608":
			addRetag(out, f, "655", "a")
		case "610":
			addRetag(out, f, "653", "a")
		case "700", "720":
			addName(out, f, "100")
		case "701", "702", "721", "722":
			addName(out, f, "700")
		case "710":
			addName(out, f, "110")
		case "711", "712":
			addName(out, f, "710")
		case "856":
			out.AddField(f)
		}
	}
	return out
}

// marc21Leader adapts a UNIMARC leader to MARC 21: it keeps the record type and
// bibliographic level and marks the encoding UTF-8. iso2709 recomputes the length
// and base address on encode.
func marc21Leader(l codex.Leader) codex.Leader {
	b := []byte(l)
	for len(b) < 24 {
		b = append(b, ' ')
	}
	b = b[:24]
	b[9] = 'a' // UTF-8
	return codex.Leader(b)
}

// build008 assembles a minimal MARC 008 from the UNIMARC coded-data fields: the
// dates from 100 and the language of the text from 101.
func build008(r *codex.Record) string {
	c100 := r.SubfieldValue("100", 'a')
	b := []byte(strings.Repeat(" ", 40))
	copy(b[18:35], []byte(strings.Repeat("|", 17)))
	if len(c100) >= 17 {
		copy(b[0:6], c100[2:8])     // date entered, yymmdd
		b[6] = c100[8]              // type of date
		copy(b[7:11], c100[9:13])   // date 1
		copy(b[11:15], c100[13:17]) // date 2
	}
	if lang := Language(r); len(lang) == 3 {
		copy(b[35:38], lang)
	}
	b[38] = ' '
	b[39] = ' '
	return string(b)
}

// addRetag re-tags f to newTag, keeping only the subfields named in codes and
// remapping their codes. codes is read in pairs "fromto" (e.g. "ac" maps $a->$c);
// a single trailing character maps to itself.
func addRetag(out *codex.Record, f codex.Field, newTag, codes string) {
	nf := codex.Field{Tag: newTag, Ind1: ' ', Ind2: ' '}
	m := codeMap(codes)
	for _, s := range f.Subfields {
		if to, ok := m[s.Code]; ok {
			if v := clean(strings.TrimRight(s.Value, " ")); v != "" {
				nf.Subfields = append(nf.Subfields, codex.NewSubfield(to, v))
			}
		}
	}
	if len(nf.Subfields) > 0 {
		out.AddField(nf)
	}
}

// codeMap parses a "from1to1from2to2…" spec into a subfield-code mapping; an odd
// trailing character maps to itself.
func codeMap(codes string) map[byte]byte {
	m := map[byte]byte{}
	for i := 0; i < len(codes); i += 2 {
		if i+1 < len(codes) {
			m[codes[i]] = codes[i+1]
		} else {
			m[codes[i]] = codes[i]
		}
	}
	return m
}

// title245 maps UNIMARC 200 (title and statement of responsibility) to MARC 245.
func title245(f codex.Field) codex.Field {
	nf := codex.Field{Tag: "245", Ind1: '1', Ind2: '0'}
	add := func(from, to byte) {
		if v := clean(strings.TrimRight(f.SubfieldValue(from), " ")); v != "" {
			nf.Subfields = append(nf.Subfields, codex.NewSubfield(to, v))
		}
	}
	add('a', 'a') // title proper
	add('e', 'b') // other title information
	add('f', 'c') // first statement of responsibility
	add('h', 'n') // number of a part
	add('i', 'p') // name of a part
	return nf
}

// addPublication maps UNIMARC 210/214 to MARC 260/264.
func addPublication(out *codex.Record, f codex.Field, newTag string) {
	nf := codex.Field{Tag: newTag, Ind1: ' ', Ind2: ' '}
	if newTag == "264" {
		nf.Ind2 = '1'
	}
	add := func(from, to byte) {
		if v := clean(strings.TrimRight(f.SubfieldValue(from), " ")); v != "" {
			nf.Subfields = append(nf.Subfields, codex.NewSubfield(to, v))
		}
	}
	add('a', 'a') // place
	add('c', 'b') // publisher name
	add('d', 'c') // date
	if len(nf.Subfields) > 0 {
		out.AddField(nf)
	}
}

// addName maps a UNIMARC name field to a MARC 21 name field, joining the entry
// element and the rest of the name and mapping dates and relator codes.
func addName(out *codex.Record, f codex.Field, newTag string) {
	name := personName(f)
	if name == "" {
		return
	}
	nf := codex.Field{Tag: newTag, Ind1: '1', Ind2: ' '}
	if newTag == "110" || newTag == "710" || newTag == "610" {
		nf.Ind1 = '2' // corporate
	}
	nf.Subfields = append(nf.Subfields, codex.NewSubfield('a', name))
	if d := clean(strings.TrimRight(f.SubfieldValue('f'), " ")); d != "" {
		nf.Subfields = append(nf.Subfields, codex.NewSubfield('d', d))
	}
	if role := clean(strings.TrimSpace(f.SubfieldValue('4'))); role != "" {
		nf.Subfields = append(nf.Subfields, codex.NewSubfield('4', role))
	}
	out.AddField(nf)
}

// addSubject maps a UNIMARC topical/geographic subject to its MARC 6xx field.
func addSubject(out *codex.Record, f codex.Field, newTag string) {
	nf := codex.Field{Tag: newTag, Ind1: ' ', Ind2: '0'}
	for _, s := range f.Subfields {
		var to byte
		switch s.Code {
		case 'a':
			to = 'a'
		case 'x':
			to = 'x'
		case 'y':
			to = 'y'
		case 'z':
			to = 'z'
		default:
			continue
		}
		if v := clean(strings.TrimRight(s.Value, " ")); v != "" {
			nf.Subfields = append(nf.Subfields, codex.NewSubfield(to, v))
		}
	}
	if len(nf.Subfields) > 0 {
		out.AddField(nf)
	}
}
