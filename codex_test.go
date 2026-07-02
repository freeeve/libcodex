package codex

import (
	"errors"
	"io"
	"reflect"
	"testing"
)

// sampleRecord builds a representative record using only the public model
// builders, so the core package tests need no serialization codec.
func sampleRecord() *Record {
	return NewRecord().
		AddField(NewControlField("001", "ocm12345")).
		AddField(NewControlField("008", "210101s2021    nyu")).
		AddField(NewDataField("245", '1', '0',
			NewSubfield('a', "Stone butch blues :"),
			NewSubfield('b', "a novel"),
			NewSubfield('c', "Leslie Feinberg."))).
		AddField(NewDataField("650", ' ', '0', NewSubfield('a', "Lesbians"))).
		AddField(NewDataField("650", ' ', '0', NewSubfield('a', "Gender identity")))
}

func TestRecordAccessors(t *testing.T) {
	rec := sampleRecord()

	t.Run("control fields", func(t *testing.T) {
		if got := rec.ControlField("001"); got != "ocm12345" {
			t.Errorf("ControlField(001) = %q", got)
		}
		if got := rec.ControlField("999"); got != "" {
			t.Errorf("ControlField(999) = %q, want empty", got)
		}
	})

	t.Run("data field and subfields", func(t *testing.T) {
		f, ok := rec.DataField("245")
		if !ok {
			t.Fatal("DataField(245) not found")
		}
		if i1, i2 := f.Indicators(); i1 != '1' || i2 != '0' {
			t.Errorf("indicators = %q %q, want '1' '0'", i1, i2)
		}
		if got := f.SubfieldValue('a'); got != "Stone butch blues :" {
			t.Errorf("subfield a = %q", got)
		}
		if v, ok := f.Subfield('z'); ok || v != "" {
			t.Errorf("unexpected subfield z = %q", v)
		}
		if _, ok := rec.DataField("999"); ok {
			t.Error("DataField(999) found, want not found")
		}
	})

	t.Run("repeated data fields", func(t *testing.T) {
		if got := len(rec.DataFields("650")); got != 2 {
			t.Fatalf("DataFields(650) = %d, want 2", got)
		}
		want := []string{"Lesbians", "Gender identity"}
		if got := rec.SubfieldValues("650", 'a'); !reflect.DeepEqual(got, want) {
			t.Errorf("SubfieldValues(650,a) = %v, want %v", got, want)
		}
		if got := rec.SubfieldValue("650", 'a'); got != "Lesbians" {
			t.Errorf("SubfieldValue(650,a) = %q, want Lesbians", got)
		}
		if got := rec.SubfieldValue("650", 'z'); got != "" {
			t.Errorf("SubfieldValue(650,z) = %q, want empty", got)
		}
	})

	t.Run("ordering and classification", func(t *testing.T) {
		fields := rec.Fields()
		if len(fields) != 5 {
			t.Fatalf("Fields() = %d, want 5", len(fields))
		}
		if !fields[0].IsControl() || fields[2].IsControl() {
			t.Error("IsControl classification wrong")
		}
	})
}

// BenchmarkSubfieldValues exercises the multi-match paths the pre-sizing targets:
// a single field with several matching subfields (regrowth in the naive impl) and
// a record with several same-tag fields (per-field intermediate slices in the
// naive impl). The single-match sample records in the exporter benchmarks never
// hit either case, so this is where the allocation win shows.
func BenchmarkSubfieldValues(b *testing.B) {
	f := NewDataField("650", ' ', '0',
		NewSubfield('a', "a1"), NewSubfield('x', "sub"),
		NewSubfield('a', "a2"), NewSubfield('a', "a3"))
	rec := NewRecord().SetLeader(Leader("00000nam a2200000 a 4500"))
	for range 4 {
		rec = rec.AddField(NewDataField("650", ' ', '0',
			NewSubfield('a', "v1"), NewSubfield('a', "v2")))
	}
	b.Run("field", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = f.SubfieldValues('a')
		}
	})
	b.Run("record", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = rec.SubfieldValues("650", 'a')
		}
	})
}

func TestFieldSubfieldValues(t *testing.T) {
	f := NewDataField("020", ' ', ' ',
		NewSubfield('a', "first"),
		NewSubfield('z', "x"),
		NewSubfield('a', "second"))
	want := []string{"first", "second"}
	if got := f.SubfieldValues('a'); !reflect.DeepEqual(got, want) {
		t.Errorf("SubfieldValues(a) = %v, want %v", got, want)
	}
	if got := f.SubfieldValues('q'); got != nil {
		t.Errorf("SubfieldValues(q) = %v, want nil", got)
	}
}

func TestRecordMutators(t *testing.T) {
	t.Run("remove and replace", func(t *testing.T) {
		rec := NewRecord().
			AddField(NewControlField("001", "a")).
			AddField(NewDataField("245", '1', '0', NewSubfield('a', "T"))).
			AddField(NewDataField("650", ' ', '0', NewSubfield('a', "x"))).
			AddField(NewDataField("650", ' ', '0', NewSubfield('a', "y")))

		rec.RemoveFields("650")
		if got := len(rec.DataFields("650")); got != 0 {
			t.Errorf("after RemoveFields, 650 count = %d, want 0", got)
		}
		if got := len(rec.Fields()); got != 2 {
			t.Errorf("Fields after remove = %d, want 2", got)
		}

		rec.ReplaceField(NewDataField("245", '0', '0', NewSubfield('a', "New")))
		if got := rec.SubfieldValue("245", 'a'); got != "New" {
			t.Errorf("after ReplaceField, 245a = %q, want New", got)
		}
		if got := len(rec.DataFields("245")); got != 1 {
			t.Errorf("245 count after replace = %d, want 1", got)
		}

		rec.ReplaceField(NewDataField("500", ' ', ' ', NewSubfield('a', "note"))) // no 500 yet -> append
		if got := rec.SubfieldValue("500", 'a'); got != "note" {
			t.Errorf("ReplaceField append 500a = %q, want note", got)
		}
	})

	t.Run("ordered insert", func(t *testing.T) {
		rec := NewRecord().
			AddField(NewControlField("001", "a")).
			AddField(NewDataField("245", '1', '0', NewSubfield('a', "T"))).
			AddField(NewDataField("700", '1', ' ', NewSubfield('a', "Z")))
		rec.InsertField(NewDataField("300", ' ', ' ', NewSubfield('a', "p")))

		var tags []string
		for _, f := range rec.Fields() {
			tags = append(tags, f.Tag)
		}
		if want := []string{"001", "245", "300", "700"}; !reflect.DeepEqual(tags, want) {
			t.Errorf("after InsertField, tags = %v, want %v", tags, want)
		}
	})
}

func TestValidate(t *testing.T) {
	if err := sampleRecord().Validate(); err != nil {
		t.Errorf("valid record: %v", err)
	}
	cases := map[string]*Record{
		"short leader":           NewRecord().SetLeader(Leader("short")).AddField(NewControlField("001", "a")),
		"bad tag":                NewRecord().AddField(Field{Tag: "12", Value: "x"}),
		"empty data field":       NewRecord().AddField(Field{Tag: "245", Ind1: '1', Ind2: '0'}),
		"control with subfields": NewRecord().AddField(Field{Tag: "001", Subfields: []Subfield{NewSubfield('a', "x")}}),
		"data with raw value":    NewRecord().AddField(Field{Tag: "245", Value: "x"}),
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if err := rec.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

// TestFieldsSnapshotSurvivesRemove documents the safe pattern for retaining a
// Fields view across a mutation: copy it first. The live view itself may change.
func TestFieldsSnapshotSurvivesRemove(t *testing.T) {
	rec := NewRecord().
		AddField(NewDataField("245", '1', '0', NewSubfield('a', "Title"))).
		AddField(NewDataField("650", ' ', '0', NewSubfield('a', "Subject")))
	snapshot := append([]Field(nil), rec.Fields()...)

	rec.RemoveFields("245")

	if snapshot[0].Tag != "245" || snapshot[0].SubfieldValue('a') != "Title" {
		t.Errorf("copied snapshot corrupted by RemoveFields: %+v", snapshot[0])
	}
	if got := len(rec.Fields()); got != 1 || rec.Fields()[0].Tag != "650" {
		t.Errorf("record after remove = %d fields, want just 650", got)
	}
}

// TestLeaderRejectsSignedNumbers ensures a leader with a sign in a numeric field
// yields 0 rather than a negative (or magnitude) length, which strconv.Atoi would
// have accepted.
func TestLeaderRejectsSignedNumbers(t *testing.T) {
	for _, prefix := range []string{"-1234", "+1234", "12 34", "abcde"} {
		l := Leader(prefix + "nam a2200000   4500")
		if got := l.RecordLength(); got != 0 {
			t.Errorf("RecordLength(%q) = %d, want 0", prefix, got)
		}
	}
	if got := Leader("12345nam a2200000   4500").RecordLength(); got != 12345 {
		t.Errorf("RecordLength of valid leader = %d, want 12345", got)
	}
}

// FuzzLeaderNumeric checks the leader numeric accessors never panic and never
// return a negative value for arbitrary bytes.
func FuzzLeaderNumeric(f *testing.F) {
	f.Add("00000nam a2200000   4500")
	f.Add("-1234nam a2200000   4500")
	f.Add("short")
	f.Fuzz(func(t *testing.T, s string) {
		l := Leader(s)
		if n := l.RecordLength(); n < 0 {
			t.Errorf("RecordLength(%q) = %d, want >= 0", s, n)
		}
		if n := l.BaseAddress(); n < 0 {
			t.Errorf("BaseAddress(%q) = %d, want >= 0", s, n)
		}
	})
}

// fakeReader yields recs in order; if failAt >= 0 it returns err at that index.
type fakeReader struct {
	recs   []*Record
	failAt int
	err    error
	i      int
}

func (f *fakeReader) Read() (*Record, error) {
	if f.failAt >= 0 && f.i == f.failAt {
		f.i++
		return nil, f.err
	}
	if f.i >= len(f.recs) {
		return nil, io.EOF
	}
	r := f.recs[f.i]
	f.i++
	return r, nil
}

// fakeWriter records writes; if failAt > 0 it errors on the failAt-th write.
type fakeWriter struct {
	failAt int
	n      int
	recs   []*Record
}

func (w *fakeWriter) Write(r *Record) error {
	w.n++
	if w.failAt > 0 && w.n >= w.failAt {
		return errors.New("write boom")
	}
	w.recs = append(w.recs, r)
	return nil
}

func TestConvert(t *testing.T) {
	r1, r2 := NewRecord(), NewRecord()

	t.Run("success", func(t *testing.T) {
		w := &fakeWriter{}
		if err := Convert(&fakeReader{recs: []*Record{r1, r2}, failAt: -1}, w); err != nil {
			t.Fatalf("Convert: %v", err)
		}
		if len(w.recs) != 2 {
			t.Errorf("wrote %d records, want 2", len(w.recs))
		}
	})

	t.Run("read error", func(t *testing.T) {
		boom := errors.New("read boom")
		err := Convert(&fakeReader{recs: []*Record{r1}, failAt: 1, err: boom}, &fakeWriter{})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want read boom", err)
		}
	})

	t.Run("write error", func(t *testing.T) {
		err := Convert(&fakeReader{recs: []*Record{r1, r2}, failAt: -1}, &fakeWriter{failAt: 1})
		if err == nil {
			t.Error("expected write error")
		}
	})
}

func TestAll(t *testing.T) {
	r1, r2 := NewRecord(), NewRecord()

	t.Run("success", func(t *testing.T) {
		var got []*Record
		for rec, err := range All(&fakeReader{recs: []*Record{r1, r2}, failAt: -1}) {
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			got = append(got, rec)
		}
		if len(got) != 2 {
			t.Errorf("iterated %d records, want 2", len(got))
		}
	})

	t.Run("stops at error", func(t *testing.T) {
		boom := errors.New("boom")
		var seen int
		var gotErr error
		for rec, err := range All(&fakeReader{recs: []*Record{r1}, failAt: 1, err: boom}) {
			if err != nil {
				gotErr = err
				break
			}
			_ = rec
			seen++
		}
		if seen != 1 {
			t.Errorf("records before error = %d, want 1", seen)
		}
		if !errors.Is(gotErr, boom) {
			t.Errorf("error = %v, want boom", gotErr)
		}
	})

	t.Run("early break", func(t *testing.T) {
		fr := &fakeReader{recs: []*Record{r1, r2}, failAt: -1}
		for range All(fr) {
			break
		}
		if fr.i != 1 {
			t.Errorf("consumed %d records after break, want 1", fr.i)
		}
	})
}

func TestNewRecordCap(t *testing.T) {
	rec := NewRecordCap(4)
	if rec.Leader().String() != defaultLeaderTemplate {
		t.Errorf("default leader = %q", rec.Leader().String())
	}
	if len(rec.Fields()) != 0 {
		t.Errorf("Fields() = %d, want 0", len(rec.Fields()))
	}
	rec.AddField(NewControlField("001", "x"))
	if got := rec.ControlField("001"); got != "x" {
		t.Errorf("after AddField, 001 = %q, want x", got)
	}
}

func TestControlFieldHasNoIndicators(t *testing.T) {
	f := NewControlField("001", "x")
	if !f.IsControl() {
		t.Error("001 should be a control field")
	}
	if i1, i2 := f.Indicators(); i1 != 0 || i2 != 0 {
		t.Errorf("control indicators = %q %q, want 0 0", i1, i2)
	}
}

func TestLeaderAccessors(t *testing.T) {
	l := Leader("01234nam a2200073   4500")
	if got := l.String(); got != "01234nam a2200073   4500" {
		t.Errorf("String() = %q", got)
	}
	if got := l.RecordStatus(); got != 'n' {
		t.Errorf("RecordStatus = %q, want 'n'", got)
	}
	if got := l.RecordType(); got != 'a' {
		t.Errorf("RecordType = %q, want 'a'", got)
	}
	if got := l.BibLevel(); got != 'm' {
		t.Errorf("BibLevel = %q, want 'm'", got)
	}
	if got := l.Encoding(); got != 'a' {
		t.Errorf("Encoding = %q, want 'a'", got)
	}
	if !l.IsUnicode() {
		t.Error("IsUnicode = false, want true")
	}
	if got := l.RecordLength(); got != 1234 {
		t.Errorf("RecordLength = %d, want 1234", got)
	}
	if got := l.BaseAddress(); got != 73 {
		t.Errorf("BaseAddress = %d, want 73", got)
	}
}

func TestLeaderMalformed(t *testing.T) {
	short := Leader("00000")
	if got := short.RecordType(); got != 0 {
		t.Errorf("RecordType on short leader = %q, want 0", got)
	}
	if got := short.BaseAddress(); got != 0 {
		t.Errorf("BaseAddress on short leader = %d, want 0", got)
	}
	bad := Leader("xxxxxnam a2200073   4500")
	if got := bad.RecordLength(); got != 0 {
		t.Errorf("RecordLength on bad digits = %d, want 0", got)
	}
}

func TestMARC8Leader(t *testing.T) {
	l := Leader("00000nam  2200000   4500") // byte 9 blank => MARC-8
	if l.IsUnicode() {
		t.Error("blank byte 9 should not be Unicode")
	}
	if got := l.Encoding(); got != ' ' {
		t.Errorf("Encoding = %q, want space", got)
	}
}

func TestRecordSetLeaderAndEncoding(t *testing.T) {
	rec := NewRecord()
	if got := rec.Leader().String(); got != defaultLeaderTemplate {
		t.Errorf("default leader = %q, want %q", got, defaultLeaderTemplate)
	}
	if rec.Encoding() != 'a' {
		t.Errorf("default Encoding = %q, want 'a'", rec.Encoding())
	}
	rec.SetLeader(Leader("00000nam  2200000   4500"))
	if rec.Encoding() != ' ' {
		t.Errorf("after SetLeader Encoding = %q, want space", rec.Encoding())
	}
}
