package mods

import (
	"bytes"
	"errors"
	"testing"

	codex "github.com/freeeve/libcodex"
)

// failWriter always returns an error on the first Write call.
type failWriter struct{}

func (f *failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("simulated write error")
}

// TestTypeOfResourceAllCases covers every branch in typeOfResource, including
// the default fallback.
func TestTypeOfResourceAllCases(t *testing.T) {
	cases := []struct {
		b    byte
		want string
	}{
		{'a', "text"},
		{'t', "text"},
		{'c', "notated music"},
		{'d', "notated music"},
		{'e', "cartographic"},
		{'f', "cartographic"},
		{'g', "moving image"},
		{'i', "sound recording-nonmusical"},
		{'j', "sound recording-musical"},
		{'k', "still image"},
		{'m', "software, multimedia"},
		{'o', "mixed material"},
		{'p', "mixed material"},
		{'r', "three dimensional object"},
		{'x', "text"}, // default branch
	}
	for _, tc := range cases {
		got := typeOfResource(tc.b)
		if got != tc.want {
			t.Errorf("typeOfResource(%q) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

// TestAuthorityAllCases covers every branch in authority, including ind2 '1'
// (lcshac) and the default.
func TestAuthorityAllCases(t *testing.T) {
	cases := []struct {
		ind2 byte
		want string
	}{
		{'0', "lcsh"},
		{'1', "lcshac"},
		{'2', "mesh"},
		{'7', ""},
		{'4', ""}, // default branch
	}
	for _, tc := range cases {
		got := authority(tc.ind2)
		if got != tc.want {
			t.Errorf("authority(%q) = %q, want %q", tc.ind2, got, tc.want)
		}
	}
}

// TestNameTypeAllCases covers every branch in nameType, including the "710",
// "711" entries that production code does not reach via FromRecord.
func TestNameTypeAllCases(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{"110", "corporate"},
		{"710", "corporate"},
		{"610", "corporate"},
		{"111", "conference"},
		{"711", "conference"},
		{"611", "conference"},
		{"600", "personal"}, // default branch
		{"700", "personal"},
	}
	for _, tc := range cases {
		got := nameType(tc.tag)
		if got != tc.want {
			t.Errorf("nameType(%q) = %q, want %q", tc.tag, got, tc.want)
		}
	}
}

// TestBuildNameDateAndRoleVia4 covers the $d (date) namePart, role via $4 when
// $e is absent, and the empty-$a path that returns false.
func TestBuildNameDateAndRoleVia4(t *testing.T) {
	f := codex.NewDataField("100", '1', ' ',
		codex.NewSubfield('a', "Smith, John,"),
		codex.NewSubfield('d', "1900-1980,"),
		codex.NewSubfield('4', "aut"))
	n, ok := buildName(f, "personal")
	if !ok {
		t.Fatal("buildName returned false for valid name")
	}
	var dateFound bool
	for _, np := range n.NamePart {
		if np.Type == "date" && np.Value == "1900-1980" {
			dateFound = true
		}
	}
	if !dateFound {
		t.Errorf("namePart date not found: %+v", n.NamePart)
	}
	if n.Role == nil || n.Role.RoleTerm.Value != "aut" {
		t.Errorf("role not set via $4: %+v", n.Role)
	}

	// Empty $a must cause buildName to return false.
	f2 := codex.NewDataField("100", '1', ' ')
	if _, ok := buildName(f2, "personal"); ok {
		t.Error("expected false for name with empty $a")
	}
}

// TestTopicSubjectAllSubfields covers every subfield branch inside topicSubject
// ($a/$x → topic, $z → geographic, $y → temporal, $v → genre) and the path
// where no relevant subfields are present (returns ok=false).
func TestTopicSubjectAllSubfields(t *testing.T) {
	f := codex.NewDataField("650", ' ', '0',
		codex.NewSubfield('a', "Ecology"),
		codex.NewSubfield('x', "Research"),
		codex.NewSubfield('z', "United States"),
		codex.NewSubfield('y', "21st century"),
		codex.NewSubfield('v', "Periodicals"))
	s, ok := topicSubject(f)
	if !ok {
		t.Fatal("expected ok=true for rich 650")
	}
	if len(s.Topic) != 2 {
		t.Errorf("topic = %v, want 2 entries", s.Topic)
	}
	if len(s.Geographic) != 1 || s.Geographic[0] != "United States" {
		t.Errorf("geographic = %v, want [United States]", s.Geographic)
	}
	if len(s.Temporal) != 1 || s.Temporal[0] != "21st century" {
		t.Errorf("temporal = %v, want [21st century]", s.Temporal)
	}
	if len(s.Genre) != 1 || s.Genre[0] != "Periodicals" {
		t.Errorf("genre = %v, want [Periodicals]", s.Genre)
	}

	// A 650 with no $a/$x/$z/$y/$v must return ok=false.
	f2 := codex.NewDataField("650", ' ', '0', codex.NewSubfield('c', "irrelevant"))
	if _, ok := topicSubject(f2); ok {
		t.Error("expected ok=false for 650 with no relevant subfields")
	}
}

// TestAppendNonEmptyBranches verifies that empty and whitespace-only values are
// dropped and that non-empty values are appended.
func TestAppendNonEmptyBranches(t *testing.T) {
	if got := appendNonEmpty(nil, ""); got != nil {
		t.Errorf("empty string: expected nil dst, got %v", got)
	}
	if got := appendNonEmpty(nil, "   "); got != nil {
		t.Errorf("whitespace-only: expected nil dst, got %v", got)
	}
	got := appendNonEmpty(nil, "value")
	if len(got) != 1 || got[0] != "value" {
		t.Errorf("non-empty: expected [value], got %v", got)
	}
}

// TestWriterFailOnOpen drives the three error-path branches that appear after a
// write failure during header emission (writeAll error → wr.err set, then
// Write returns cached error, then Close returns cached error).
func TestWriterFailOnOpen(t *testing.T) {
	w := NewWriter(&failWriter{})

	// First Write triggers open() → writeAll fails → error surfaced.
	if err := w.Write(sample()); err == nil {
		t.Fatal("expected error when underlying writer always fails")
	}
	// wr.err is now set; Write must return it immediately (wr.err != nil branch).
	if err := w.Write(sample()); err == nil {
		t.Error("expected cached error on second Write")
	}
	// wr.err is set; Close must return it immediately (wr.err != nil branch).
	if err := w.Close(); err == nil {
		t.Error("expected cached error on Close after write failure")
	}
}

// TestWriterCloseFailOnOpen verifies that Close surfaces a header-write failure
// when Write has never been called (open() is invoked inside Close).
func TestWriterCloseFailOnOpen(t *testing.T) {
	w := NewWriter(&failWriter{})
	if err := w.Close(); err == nil {
		t.Error("expected error when header write fails during Close")
	}
}

// TestWriterDoubleClose checks that calling Close a second time on a
// successfully closed Writer returns nil.
func TestWriterDoubleClose(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close should return nil, got %v", err)
	}
}

// TestSubjectNamesVia610And611 exercises the 610 (corporate) and 611
// (conference) branches inside the "600"/"610"/"611" case of FromRecord, which
// routes through nameType and buildName.
func TestSubjectNamesVia610And611(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000cam a2200000 a 4500")).
		AddField(codex.NewDataField("610", '2', '0',
			codex.NewSubfield('a', "Acme Corporation."))).
		AddField(codex.NewDataField("611", '2', '0',
			codex.NewSubfield('a', "World Health Assembly.")))

	m := FromRecord(rec)
	var corp, conf bool
	for _, s := range m.Subject {
		if s.Name != nil {
			switch s.Name.Type {
			case "corporate":
				corp = true
			case "conference":
				conf = true
			}
		}
	}
	if !corp {
		t.Error("expected corporate name subject from 610")
	}
	if !conf {
		t.Error("expected conference name subject from 611")
	}
}

// TestSubjectLcshacAuthority exercises the authority "lcshac" (ind2 '1') path
// through FromRecord via 650, 651, and 655 fields.
func TestSubjectLcshacAuthority(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000cam a2200000 a 4500")).
		AddField(codex.NewDataField("650", ' ', '1',
			codex.NewSubfield('a', "Children"))).
		AddField(codex.NewDataField("651", ' ', '1',
			codex.NewSubfield('a', "Canada"))).
		AddField(codex.NewDataField("655", ' ', '1',
			codex.NewSubfield('a', "Picture books")))

	m := FromRecord(rec)
	if len(m.Subject) != 3 {
		t.Fatalf("expected 3 subjects, got %d", len(m.Subject))
	}
	for _, s := range m.Subject {
		if s.Authority != "lcshac" {
			t.Errorf("expected lcshac authority, got %q for subject %+v", s.Authority, s)
		}
	}
}

// TestTypeOfResourceViaRecord confirms that records with various leader byte 6
// values produce the expected MODS typeOfResource through the full FromRecord
// path.
func TestTypeOfResourceViaRecord(t *testing.T) {
	makeRec := func(recordType byte) *codex.Record {
		leader := "00000n" + string([]byte{recordType}) + "  a2200000 a 4500"
		return codex.NewRecord().SetLeader(codex.Leader(leader))
	}
	cases := []struct {
		b    byte
		want string
	}{
		{'t', "text"},
		{'c', "notated music"},
		{'d', "notated music"},
		{'f', "cartographic"},
		{'g', "moving image"},
		{'i', "sound recording-nonmusical"},
		{'j', "sound recording-musical"},
		{'k', "still image"},
		{'m', "software, multimedia"},
		{'o', "mixed material"},
		{'p', "mixed material"},
		{'r', "three dimensional object"},
	}
	for _, tc := range cases {
		m := FromRecord(makeRec(tc.b))
		if m.TypeOfResource != tc.want {
			t.Errorf("leader byte 6 %q: typeOfResource = %q, want %q", tc.b, m.TypeOfResource, tc.want)
		}
	}
}
