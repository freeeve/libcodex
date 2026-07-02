package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// findSubject returns the first subject with the given label, or nil.
func findSubject(g *BIBFRAME, label string) *Subject {
	for i := range g.Work.Subjects {
		if g.Work.Subjects[i].Label == label {
			return &g.Work.Subjects[i]
		}
	}
	return nil
}

// TestSubjectSourceFromRecord covers the thesaurus (bf:source) derived from the
// 6xx second indicator or $2 (task 060), and the reroute of a subdivided 655 from
// a flat genreForm to a topical subject.
func TestSubjectSourceFromRecord(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Cats"))).
		AddField(codex.NewDataField("650", ' ', '2', codex.NewSubfield('a', "Felines"))).
		AddField(codex.NewDataField("650", ' ', '7', codex.NewSubfield('a', "Dogs"), codex.NewSubfield('2', "fast"))).
		AddField(codex.NewDataField("655", ' ', '7', codex.NewSubfield('a', "Biography"),
			codex.NewSubfield('x', "History"), codex.NewSubfield('2', "lcgft"))).
		AddField(codex.NewDataField("655", ' ', '7', codex.NewSubfield('a', "Fiction"), codex.NewSubfield('2', "lcgft")))
	g := FromRecord(rec)

	for label, want := range map[string]string{"Cats": "lcsh", "Felines": "mesh", "Dogs": "fast"} {
		s := findSubject(g, label)
		if s == nil {
			t.Fatalf("subject %q not found in %+v", label, g.Work.Subjects)
		}
		if s.Source != want {
			t.Errorf("subject %q source = %q, want %q", label, s.Source, want)
		}
	}

	// A subdivided 655 becomes a topical subject carrying its scheme, not a genreForm.
	if s := findSubject(g, "Biography--History"); s == nil || s.Class != "Topic" || s.Source != "lcgft" {
		t.Errorf("subdivided 655 not a Topic subject with source lcgft; got %+v", g.Work.Subjects)
	}
	// A plain 655 stays a genreForm.
	if len(g.Work.GenreForms) != 1 || g.Work.GenreForms[0] != "Fiction" {
		t.Errorf("plain 655 genreForms = %v, want [Fiction]", g.Work.GenreForms)
	}
}

// TestSubjectSourceRoundTrip confirms the thesaurus survives Encode -> Decode: a
// numeric-indicator scheme comes back as its ind2, and a $2 scheme as ind2='7' $2.
func TestSubjectSourceRoundTrip(t *testing.T) {
	cases := []struct {
		ind2 byte
		sub2 string
	}{
		{'0', ""},      // lcsh
		{'2', ""},      // mesh
		{'7', "fast"},  // $2-named scheme
		{'7', "lcgft"}, // another $2 scheme
	}
	for _, tc := range cases {
		subs := []codex.Subfield{codex.NewSubfield('a', "Heading")}
		if tc.sub2 != "" {
			subs = append(subs, codex.NewSubfield('2', tc.sub2))
		}
		rec := codex.NewRecord().
			SetLeader(codex.Leader("00000nam a2200000 a 4500")).
			AddField(codex.NewControlField("001", "x")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
			AddField(codex.NewDataField("650", ' ', tc.ind2, subs...))

		encoded, err := Encode(rec)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		recs, err := Decode(encoded)
		if err != nil || len(recs) != 1 {
			t.Fatalf("Decode: %v (%d records)", err, len(recs))
		}
		f := firstField(recs[0], "650")
		if f == nil {
			t.Fatalf("650 missing after round-trip (ind2=%c $2=%q)", tc.ind2, tc.sub2)
		}
		if f.Ind2 != tc.ind2 {
			t.Errorf("650 ind2 = %c, want %c (source $2=%q)", f.Ind2, tc.ind2, tc.sub2)
		}
		if got := f.SubfieldValue('2'); got != tc.sub2 {
			t.Errorf("650 $2 = %q, want %q (ind2=%c)", got, tc.sub2, tc.ind2)
		}
	}
}
