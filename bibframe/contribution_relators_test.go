package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// findContribution returns the first contribution whose agent label matches.
func findContribution(g *BIBFRAME, label string) *Contribution {
	for i := range g.Work.Contributions {
		if g.Work.Contributions[i].Label == label {
			return &g.Work.Contributions[i]
		}
	}
	return nil
}

func recordWith(fields ...codex.Field) *codex.Record {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
	for _, f := range fields {
		rec.AddField(f)
	}
	return rec
}

// TestContributionRelatorIRI covers the $4 relator code -> relators IRI mapping and
// the $4-over-$e preference (task 061): a $4 code becomes the role IRI, while a $e
// with no $4 stays a literal role.
func TestContributionRelatorIRI(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Writer, A."), codex.NewSubfield('e', "author")),
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An"), codex.NewSubfield('4', "edt")),
	))
	if c := findContribution(g, "Writer, A."); c == nil || len(c.Roles) != 1 || c.Roles[0].IRI != "" || c.Roles[0].Term != "author" {
		t.Errorf("100 $e author -> literal role; got %+v", c)
	}
	if c := findContribution(g, "Editor, An"); c == nil || len(c.Roles) != 1 || c.Roles[0].IRI != relatorVocab+"edt" {
		t.Errorf("700 $4 edt -> relators IRI; got %+v", c)
	}
}

// TestContributionCompoundRoles covers multiple $4/$e and a single $e split on the
// ", and &" compound-role delimiters (task 061).
func TestContributionCompoundRoles(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Person, A."),
			codex.NewSubfield('4', "edt"), codex.NewSubfield('4', "trl"),
			codex.NewSubfield('e', "editor, compiler and illustrator")),
	))
	c := findContribution(g, "Person, A.")
	if c == nil {
		t.Fatal("contribution missing")
	}
	want := []Role{
		{IRI: relatorVocab + "edt", Term: "edt"},
		{IRI: relatorVocab + "trl", Term: "trl"},
		{Term: "editor"},
		{Term: "compiler"},
		{Term: "illustrator"},
	}
	if len(c.Roles) != len(want) {
		t.Fatalf("roles = %+v, want %+v", c.Roles, want)
	}
	for i, w := range want {
		if c.Roles[i] != w {
			t.Errorf("role[%d] = %+v, want %+v", i, c.Roles[i], w)
		}
	}
}

// TestContributionAgentTyping covers ind1-based Family/Jurisdiction typing and the
// full-subfield agent label (task 061).
func TestContributionAgentTyping(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("100", '3', ' ', codex.NewSubfield('a', "Bach"), codex.NewSubfield('c', "(Family)")),
		codex.NewDataField("110", '1', ' ', codex.NewSubfield('a', "United States"), codex.NewSubfield('b', "Congress")),
	))
	if c := findContribution(g, "Bach (Family)"); c == nil || c.Class != "Family" {
		t.Errorf("100 ind1=3 -> Family with $a$c label; got %+v", c)
	}
	if c := findContribution(g, "United States Congress"); c == nil || c.Class != "Jurisdiction" {
		t.Errorf("110 ind1=1 -> Jurisdiction with $a$b label; got %+v", c)
	}
}

// TestContributionMeetingRelatorJ covers the 111/711 literal relator living in $j,
// not $e (which is read as part of the meeting name label), while a $4 code still
// maps to a relators IRI (task 061).
func TestContributionMeetingRelatorJ(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("711", '2', ' ', codex.NewSubfield('a', "Symposium"),
			codex.NewSubfield('e', "Editorial Board"),
			codex.NewSubfield('j', "honoree"), codex.NewSubfield('4', "hst")),
	))
	c := findContribution(g, "Symposium Editorial Board")
	if c == nil {
		t.Fatalf("meeting contribution missing; got %+v", g.Work.Contributions)
	}
	want := []Role{{IRI: relatorVocab + "hst", Term: "hst"}, {Term: "honoree"}}
	if c.Class != "Meeting" || len(c.Roles) != len(want) {
		t.Fatalf("711 roles = %+v, want %+v", c.Roles, want)
	}
	for i, w := range want {
		if c.Roles[i] != w {
			t.Errorf("711 role[%d] = %+v, want %+v", i, c.Roles[i], w)
		}
	}
}

// TestContributionVerbatimURIRole covers a URI-valued $4 (used verbatim) and an
// XML-unsafe $4 falling back to a literal role so the serializers stay well-formed
// (task 061).
func TestContributionVerbatimURIRole(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Safe, U."),
			codex.NewSubfield('4', "https://example.org/role/x")),
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Unsafe, U."),
			codex.NewSubfield('4', "not a <uri> & bad")),
	))
	if c := findContribution(g, "Safe, U."); c == nil || len(c.Roles) != 1 || c.Roles[0].IRI != "https://example.org/role/x" {
		t.Errorf("safe URI $4 used verbatim; got %+v", c)
	}
	if c := findContribution(g, "Unsafe, U."); c == nil || len(c.Roles) != 1 || c.Roles[0].IRI != "" || c.Roles[0].Term != "not a <uri> & bad" {
		t.Errorf("unsafe $4 -> literal role; got %+v", c)
	}
	// The unsafe value must not leak into an unescaped node IRI.
	b, err := Encode(recordWith(codex.NewDataField("700", '1', ' ',
		codex.NewSubfield('a', "Unsafe, U."), codex.NewSubfield('4', "not a <uri> & bad"))))
	if err != nil {
		t.Fatal(err)
	}
	if err := xmlWellFormed(b); err != nil {
		t.Fatalf("RDF/XML not well-formed with unsafe $4: %v\n%s", err, b)
	}
}

// TestContributionRolesRoundTrip confirms the role and agent-typing signals survive
// Encode -> Decode: a $4 code comes back as $4, a literal $e as $e, a meeting $j as
// $j, and Family/Jurisdiction indicators are preserved via the agent class.
func TestContributionRolesRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("100", '3', ' ', codex.NewSubfield('a', "Bach (Family)")),
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An"), codex.NewSubfield('4', "edt")),
		codex.NewDataField("710", '1', ' ', codex.NewSubfield('a', "United States")),
		codex.NewDataField("711", '2', ' ', codex.NewSubfield('a', "Symposium"), codex.NewSubfield('j', "hst")),
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Writer, A."), codex.NewSubfield('e', "author")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	checks := []struct {
		tag       string
		wantInd1  byte
		wantSub   byte
		wantValue string
	}{
		{"100", '3', 'a', "Bach (Family)"}, // Family typed back to ind1=3
		{"700", '1', '4', "edt"},           // relator code back to $4
		{"710", '1', 'a', "United States"}, // Jurisdiction back to x10 ind1=1
		{"711", '2', 'j', "hst"},           // meeting relator back to $j
	}
	for _, c := range checks {
		f := firstField(got, c.tag)
		if f == nil {
			t.Errorf("%s missing after round-trip", c.tag)
			continue
		}
		if f.Ind1 != c.wantInd1 {
			t.Errorf("%s ind1 = %c, want %c", c.tag, f.Ind1, c.wantInd1)
		}
		if v := f.SubfieldValue(c.wantSub); v != c.wantValue {
			t.Errorf("%s $%c = %q, want %q", c.tag, c.wantSub, v, c.wantValue)
		}
	}
	// The literal-role added entry keeps $e.
	var lit *codex.Field
	for i := range got.Fields() {
		f := got.Fields()[i]
		if f.Tag == "700" && f.SubfieldValue('a') == "Writer, A." {
			lit = &f
		}
	}
	if lit == nil || lit.SubfieldValue('e') != "author" {
		t.Errorf("literal $e author not reconstructed; got %+v", lit)
	}
}
