package iso2709

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/freeeve/libcodex"
)

// tfield is a test description of a field used by buildRecord.
type tfield struct {
	tag        string
	control    string
	ind1, ind2 byte
	subs       []codex.Subfield
}

// buildRecord assembles a valid ISO 2709 record from fields, computing the
// leader length, base address and directory offsets. leaderTmpl supplies the
// static leader bytes (record type, encoding, etc.); its [0:5] and [12:17] are
// overwritten.
func buildRecord(tb testing.TB, leaderTmpl string, fields []tfield) []byte {
	tb.Helper()
	if len(leaderTmpl) != leaderLen {
		tb.Fatalf("leader template must be %d bytes, got %d", leaderLen, len(leaderTmpl))
	}

	var dir, data bytes.Buffer
	for _, f := range fields {
		start := data.Len()
		if f.tag < "010" {
			data.WriteString(f.control)
		} else {
			data.WriteByte(f.ind1)
			data.WriteByte(f.ind2)
			for _, s := range f.subs {
				data.WriteByte(SubfieldDelimiter)
				data.WriteByte(s.Code)
				data.WriteString(s.Value)
			}
		}
		data.WriteByte(FieldTerminator)
		fmt.Fprintf(&dir, "%s%04d%05d", f.tag, data.Len()-start, start)
	}
	dir.WriteByte(FieldTerminator)

	base := leaderLen + dir.Len()
	total := base + data.Len() + 1
	leader := []byte(leaderTmpl)
	copy(leader[0:5], fmt.Sprintf("%05d", total))
	copy(leader[12:17], fmt.Sprintf("%05d", base))

	var rec bytes.Buffer
	rec.Write(leader)
	rec.Write(dir.Bytes())
	rec.Write(data.Bytes())
	rec.WriteByte(RecordTerminator)
	return rec.Bytes()
}

const utf8Leader = "00000nam a2200000   4500"
const marc8Leader = "00000nam  2200000   4500" // byte 9 is blank => MARC-8

func sampleFields() []tfield {
	return []tfield{
		{tag: "001", control: "ocm12345"},
		{tag: "008", control: "210101s2021    nyu"},
		{tag: "245", ind1: '1', ind2: '0', subs: []codex.Subfield{
			{Code: 'a', Value: "Stone butch blues :"},
			{Code: 'b', Value: "a novel"},
			{Code: 'c', Value: "Leslie Feinberg."},
		}},
		{tag: "650", ind1: ' ', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Lesbians"}}},
		{tag: "650", ind1: ' ', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Gender identity"}}},
	}
}

func TestDecodeAccessors(t *testing.T) {
	raw := buildRecord(t, utf8Leader, sampleFields())
	rec, _, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got := rec.ControlField("001"); got != "ocm12345" {
		t.Errorf("ControlField(001) = %q", got)
	}
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
	if got := len(rec.DataFields("650")); got != 2 {
		t.Errorf("DataFields(650) = %d, want 2", got)
	}
	if got := len(rec.Fields()); got != 5 {
		t.Errorf("Fields() = %d, want 5", got)
	}
}

func TestDecodePreservesLeader(t *testing.T) {
	raw := buildRecord(t, utf8Leader, sampleFields())
	rec, _, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	l := rec.Leader()
	if got := l.RecordLength(); got != len(raw) {
		t.Errorf("RecordLength = %d, want %d", got, len(raw))
	}
	if got := l.BaseAddress(); got <= leaderLen {
		t.Errorf("BaseAddress = %d, want > %d", got, leaderLen)
	}
	if !l.IsUnicode() {
		t.Error("IsUnicode = false, want true")
	}
}

func TestReaderMultiple(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(buildRecord(t, utf8Leader, sampleFields()))
	stream.Write(buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "second"}}))
	stream.WriteByte('\n') // tolerate an inter-record newline
	stream.Write(buildRecord(t, utf8Leader, []tfield{
		{tag: "245", ind1: '0', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Third"}}},
	}))

	r := NewReader(&stream)
	var recs []*codex.Record
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		recs = append(recs, rec)
	}

	if len(recs) != 3 {
		t.Fatalf("read %d records, want 3", len(recs))
	}
	if got := recs[1].ControlField("001"); got != "second" {
		t.Errorf("record 2 control 001 = %q, want second", got)
	}
	if got := recs[2].SubfieldValue("245", 'a'); got != "Third" {
		t.Errorf("record 3 245a = %q, want Third", got)
	}
}

func TestReaderEmpty(t *testing.T) {
	cases := map[string]io.Reader{
		"empty":      bytes.NewReader(nil),
		"whitespace": bytes.NewReader([]byte("\n\r\n")),
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewReader(src).Read(); err != io.EOF {
				t.Errorf("Read = %v, want io.EOF", err)
			}
		})
	}
}

func TestReaderFallbackLength(t *testing.T) {
	raw := buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "fallback"}})
	copy(raw[0:5], "xxxxx") // corrupt the declared length to force terminator scan

	rec, err := NewReader(bytes.NewReader(raw)).Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := rec.ControlField("001"); got != "fallback" {
		t.Errorf("control 001 = %q, want fallback", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	good := buildRecord(t, utf8Leader, sampleFields())

	badBase := append([]byte(nil), good...)
	copy(badBase[12:17], "abcde")

	outOfRange := append([]byte(nil), good...)
	copy(outOfRange[12:17], "99999")

	cases := map[string][]byte{
		"too short":     []byte("short"),
		"bad base":      badBase,
		"base too high": outOfRange,
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Decode(b); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestDecodeSkipsBadDirectoryEntry(t *testing.T) {
	raw := buildRecord(t, utf8Leader, []tfield{
		{tag: "001", control: "ok"},
		{tag: "245", ind1: '0', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Title"}}},
	})
	// Point the second directory entry's start offset far past the field data
	// (each standard MARC 21 directory entry is 12 bytes: 3 tag + 4 length + 5
	// start position).
	dirSecond := leaderLen + 12
	copy(raw[dirSecond+7:dirSecond+12], "99999")

	rec, _, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(rec.Fields()) != 1 {
		t.Errorf("expected 1 surviving field, got %d", len(rec.Fields()))
	}
}

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mrc")
	var buf bytes.Buffer
	buf.Write(buildRecord(t, utf8Leader, sampleFields()))
	buf.Write(buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "two"}}))
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	recs, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("ReadFile returned %d records, want 2", len(recs))
	}

	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.mrc")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRoundTripDecodeEncode(t *testing.T) {
	raw := buildRecord(t, utf8Leader, sampleFields())
	first, _, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	var buf bytes.Buffer
	if err := NewWriter(&buf).Write(first); err != nil {
		t.Fatalf("Write: %v", err)
	}

	second, _, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("round trip mismatch:\n first  = %#v\n second = %#v", first, second)
	}
	if got := second.Leader().Encoding(); got != 'a' {
		t.Errorf("written encoding = %q, want 'a'", got)
	}
	if got := second.Leader().RecordLength(); got != buf.Len() {
		t.Errorf("written RecordLength = %d, want %d", got, buf.Len())
	}
}

func TestBuildFromScratchRoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "scratch-001")).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Built from scratch"),
			codex.NewSubfield('c', "by a test."))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Testing")))

	var buf bytes.Buffer
	if err := NewWriter(&buf).Write(rec); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := NewReader(&buf).Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if v := got.ControlField("001"); v != "scratch-001" {
		t.Errorf("001 = %q, want scratch-001", v)
	}
	if v := got.SubfieldValue("245", 'a'); v != "Built from scratch" {
		t.Errorf("245a = %q", v)
	}
	if f, _ := got.DataField("245"); f.Ind1 != '1' || f.Ind2 != '0' {
		t.Errorf("245 indicators = %q %q, want '1' '0'", f.Ind1, f.Ind2)
	}
	if v := got.SubfieldValue("650", 'a'); v != "Testing" {
		t.Errorf("650a = %q", v)
	}
}

func TestEncodeRejectsDelimiters(t *testing.T) {
	// ISO 2709 cannot carry a value, indicator or code that is itself a reserved
	// structural delimiter byte; Encode must reject rather than corrupt.
	cases := map[string]*codex.Record{
		"control value":  codex.NewRecord().AddField(codex.NewControlField("001", "a\x1eb")),
		"subfield value": codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "x\x1fy"))),
		"indicator":      codex.NewRecord().AddField(codex.NewDataField("245", 0x1d, '0', codex.NewSubfield('a', "x"))),
		"subfield code":  codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield(0x1f, "x"))),
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode(rec); err == nil {
				t.Error("expected error for a reserved delimiter byte")
			}
		})
	}
}

func TestEncodeErrors(t *testing.T) {
	t.Run("bad tag", func(t *testing.T) {
		rec := codex.NewRecord().AddField(codex.Field{Tag: "12", Value: "x"})
		if _, err := Encode(rec); err == nil {
			t.Error("expected error for short tag")
		}
	})
	t.Run("field too long", func(t *testing.T) {
		big := make([]byte, 10000)
		for i := range big {
			big[i] = 'a'
		}
		rec := codex.NewRecord().AddField(codex.NewDataField("500", ' ', ' ', codex.NewSubfield('a', string(big))))
		if _, err := Encode(rec); err == nil {
			t.Error("expected error for oversized field")
		}
	})
}

func TestMARC8Decoding(t *testing.T) {
	// ANSEL combining diacritics (0xE2 acute, 0xE8 diaeresis) precede their base
	// letter, the reverse of Unicode. The decoder must reorder and compose.
	fields := []tfield{
		{tag: "100", ind1: '1', ind2: ' ', subs: []codex.Subfield{
			{Code: 'a', Value: "Beyonc\xe2e"}, // Beyonce + acute on final e
		}},
		{tag: "245", ind1: '1', ind2: '0', subs: []codex.Subfield{
			{Code: 'a', Value: "na\xe8ive"}, // na + (diaeresis)i + ve
		}},
	}
	raw := buildRecord(t, marc8Leader, fields)

	rec, lossy, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if lossy {
		t.Error("expected non-lossy decode: all values are in-scope ANSEL")
	}
	if rec.Leader().IsUnicode() {
		t.Fatal("expected MARC-8 (non-Unicode) leader")
	}
	if got := rec.SubfieldValue("100", 'a'); got != "Beyoncé" {
		t.Errorf("100a = %q (% x), want %q", got, got, "Beyoncé")
	}
	if got := rec.SubfieldValue("245", 'a'); got != "naïve" {
		t.Errorf("245a = %q (% x), want %q", got, got, "naïve")
	}
}

func TestEncodeForcesLeaderGeometry(t *testing.T) {
	// A caller-supplied leader with the wrong indicator count, subfield code
	// count and entry map must be normalized to the MARC 21 fixed geometry the
	// encoder actually emits, so the declared geometry matches the bytes.
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a3300000   9876")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	out, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	l := codex.Leader(out[:24])
	if l[10] != '2' || l[11] != '2' {
		t.Errorf("indicator/subfield counts = %q %q, want '2' '2'", l[10], l[11])
	}
	if got := l.String()[20:24]; got != "4500" {
		t.Errorf("entry map = %q, want 4500", got)
	}
	if _, _, err := Decode(out); err != nil {
		t.Errorf("re-decode of normalized record: %v", err)
	}
}

func TestDecodeHonorsEntryMap(t *testing.T) {
	// A non-standard but well-formed ISO 2709 directory: entry map "4600" means
	// 6-digit start positions, so each directory entry is 13 bytes (3 tag + 4
	// length + 6 start). A decoder hardcoded to 12-byte entries would misparse.
	var buf bytes.Buffer
	buf.WriteString("00041nam a2200038   4600") // leader: base 38, entry map 4600
	buf.WriteString("0010002000000")            // tag 001, length 0002, start 000000
	buf.WriteByte(FieldTerminator)              // directory terminator
	buf.WriteString("x")
	buf.WriteByte(FieldTerminator) // field terminator
	buf.WriteByte(RecordTerminator)

	if buf.Len() != 41 {
		t.Fatalf("constructed record is %d bytes, want 41", buf.Len())
	}
	rec, _, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := rec.ControlField("001"); got != "x" {
		t.Errorf("ControlField(001) = %q, want x (entry map not honored)", got)
	}
}

func TestOctetLengthCounting(t *testing.T) {
	// MARC 21 counts directory lengths/offsets and the leader length in octets,
	// not characters. Multibyte UTF-8 values must round-trip and the declared
	// length must equal the byte length.
	raw := buildRecord(t, utf8Leader, []tfield{
		{tag: "245", ind1: '1', ind2: '0', subs: []codex.Subfield{
			{Code: 'a', Value: "café 日本語"}, // "café 日本語"
		}},
	})
	rec, _, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := rec.SubfieldValue("245", 'a'); got != "café 日本語" {
		t.Errorf("245a = %q, want %q", got, "café 日本語")
	}
	out, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	re, _, err := Decode(out)
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if got := re.Leader().RecordLength(); got != len(out) {
		t.Errorf("declared RecordLength = %d, want octet length %d", got, len(out))
	}
}

func TestMARC8StatePersistsAcrossSubfields(t *testing.T) {
	// A character-set designation in $a must persist into $b of the same field.
	// $a designates G1 to an unsupported set; $b's 0xB5 must then pass through as
	// Latin-1 'µ' rather than be ANSEL-decoded to 'æ' (which a per-subfield reset
	// would wrongly produce).
	raw := buildRecord(t, marc8Leader, []tfield{
		{tag: "100", ind1: '1', ind2: ' ', subs: []codex.Subfield{
			{Code: 'a', Value: "\x1b)1"}, // ESC ) 1 : designate G1 to an unsupported set
			{Code: 'b', Value: "\xb5"},   // 0xB5: ANSEL 'æ', Latin-1 'µ'
		}},
	})
	rec, lossy, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !lossy {
		t.Error("expected lossy decode: $a designates an out-of-scope set")
	}
	f, ok := rec.DataField("100")
	if !ok {
		t.Fatal("DataField(100) not found")
	}
	if got := f.SubfieldValue('b'); got != "µ" {
		t.Errorf("100b = %q (% x), want µ (designation should persist across subfields)", got, got)
	}
}

func TestEncodeInto(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "a1")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Title")))

	want, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := EncodeInto(nil, rec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeInto(nil) != Encode:\n got  %q\n want %q", got, want)
	}

	// Appending a second record into the same buffer must concatenate two records
	// that read back independently.
	two, err := EncodeInto(got, rec)
	if err != nil {
		t.Fatal(err)
	}
	r := NewReader(bytes.NewReader(two))
	n := 0
	for {
		rr, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if v := rr.SubfieldValue("245", 'a'); v != "Title" {
			t.Errorf("record %d 245a = %q, want Title", n, v)
		}
		n++
	}
	if n != 2 {
		t.Errorf("read %d records, want 2", n)
	}

	// A tag error leaves dst unchanged.
	bad := codex.NewRecord().AddField(codex.Field{Tag: "12", Value: "x"})
	if out, err := EncodeInto(want, bad); err == nil || !bytes.Equal(out, want) {
		t.Errorf("EncodeInto with bad tag = (%q, %v), want dst unchanged + error", out, err)
	}
}

func TestWriterReuseNoLeak(t *testing.T) {
	// The Writer's reused buffer must not leak bytes between records of different
	// sizes.
	var buf bytes.Buffer
	w := NewWriter(&buf)
	recs := []*codex.Record{
		codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A long-ish title here"))),
		codex.NewRecord().AddField(codex.NewControlField("001", "z")), // shorter
		codex.NewRecord().AddField(codex.NewControlField("001", "third-record")),
	}
	for i, rec := range recs {
		if err := w.Write(rec); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	got, err := func() ([]*codex.Record, error) {
		r := NewReader(&buf)
		var out []*codex.Record
		for {
			rr, err := r.Read()
			if err == io.EOF {
				return out, nil
			}
			if err != nil {
				return out, err
			}
			out = append(out, rr)
		}
	}()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d records, want 3", len(got))
	}
	if v := got[0].SubfieldValue("245", 'a'); v != "A long-ish title here" {
		t.Errorf("record 0 245a = %q", v)
	}
	if v := got[2].ControlField("001"); v != "third-record" {
		t.Errorf("record 2 001 = %q, want third-record", v)
	}
}

func TestReaderAll(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(buildRecord(t, utf8Leader, sampleFields()))
	stream.Write(buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "two"}}))

	var ids []string
	for rec, err := range NewReader(&stream).All() {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		ids = append(ids, rec.ControlField("001"))
	}
	if want := []string{"ocm12345", "two"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestReaderLossy(t *testing.T) {
	lossyRaw := buildRecord(t, marc8Leader, []tfield{
		{tag: "100", ind1: '1', ind2: ' ', subs: []codex.Subfield{
			{Code: 'a', Value: "\x1b)1"}, // designate an out-of-scope set
			{Code: 'b', Value: "\xb5"},
		}},
	})
	cleanRaw := buildRecord(t, utf8Leader, sampleFields())

	for name, tc := range map[string]struct {
		raw  []byte
		want bool
	}{
		"lossy": {lossyRaw, true},
		"clean": {cleanRaw, false},
	} {
		t.Run(name, func(t *testing.T) {
			r := NewReader(bytes.NewReader(tc.raw))
			if _, err := r.Read(); err != nil {
				t.Fatal(err)
			}
			if r.Lossy() != tc.want {
				t.Errorf("Lossy() = %v, want %v", r.Lossy(), tc.want)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	recs := []*codex.Record{
		codex.NewRecord().AddField(codex.NewControlField("001", "one")),
		codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Two"))),
	}
	path := filepath.Join(t.TempDir(), "out.mrc")
	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d records, want 2", len(got))
	}
	if got[0].ControlField("001") != "one" || got[1].SubfieldValue("245", 'a') != "Two" {
		t.Error("round-trip content mismatch")
	}

	if err := WriteFile(filepath.Join(t.TempDir(), "missing-dir", "x.mrc"), recs); err == nil {
		t.Error("expected error writing into a nonexistent directory")
	}
}

func TestEncodeZeroIndicators(t *testing.T) {
	// A data field built without indicators (zero value) must serialize as blanks.
	rec := codex.NewRecord().AddField(codex.Field{
		Tag:       "500",
		Subfields: []codex.Subfield{{Code: 'a', Value: "note"}},
	})
	out, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := got.DataField("500")
	if i1, i2 := f.Indicators(); i1 != ' ' || i2 != ' ' {
		t.Errorf("indicators = %q %q, want ' ' ' '", i1, i2)
	}
}

func TestEncodeMalformedLeaderUsesDefault(t *testing.T) {
	// A record whose leader is not 24 bytes encodes using the default template.
	rec := codex.NewRecord().SetLeader(codex.Leader("short")).
		AddField(codex.NewControlField("001", "x"))
	out, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(out)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ControlField("001") != "x" {
		t.Errorf("001 = %q, want x", got.ControlField("001"))
	}
	if got.Leader().RecordType() != 'a' { // byte 6 of the default template
		t.Errorf("record type = %q, want 'a'", got.Leader().RecordType())
	}
}

func TestUnexportedHelpers(t *testing.T) {
	t.Run("atoiBytes", func(t *testing.T) {
		if n, ok := atoiBytes([]byte("0042")); !ok || n != 42 {
			t.Errorf("atoiBytes(0042) = %d,%v", n, ok)
		}
		if _, ok := atoiBytes(nil); ok {
			t.Error("atoiBytes(nil) ok = true, want false")
		}
		if _, ok := atoiBytes([]byte("1x3")); ok {
			t.Error("atoiBytes(1x3) ok = true, want false")
		}
	})
	t.Run("leaderDigit", func(t *testing.T) {
		b := []byte(utf8Leader)
		if got := leaderDigit(b, 20, 9); got != 4 {
			t.Errorf("leaderDigit(20) = %d, want 4", got)
		}
		if got := leaderDigit(b, 5, 9); got != 9 { // 'n' is not a digit -> default
			t.Errorf("leaderDigit(5) = %d, want default 9", got)
		}
		if got := leaderDigit(b, 99, 7); got != 7 { // out of range -> default
			t.Errorf("leaderDigit(99) = %d, want default 7", got)
		}
	})
	t.Run("prealloc", func(t *testing.T) {
		for _, c := range []struct{ in, want int }{{-1, 0}, {7, 7}, {1 << 20, 1 << 16}} {
			if got := prealloc(c.in); got != c.want {
				t.Errorf("prealloc(%d) = %d, want %d", c.in, got, c.want)
			}
		}
	})
}

func TestReaderErrors(t *testing.T) {
	t.Run("truncated leader", func(t *testing.T) {
		_, err := NewReader(bytes.NewReader([]byte("0001234"))).Read()
		if err == nil || err == io.EOF {
			t.Errorf("err = %v, want a truncated-leader error", err)
		}
	})
	t.Run("truncated body", func(t *testing.T) {
		raw := buildRecord(t, utf8Leader, sampleFields())
		_, err := NewReader(bytes.NewReader(raw[:len(raw)-5])).Read() // declared length exceeds stream
		if err == nil || err == io.EOF {
			t.Errorf("err = %v, want a truncated-body error", err)
		}
	})
}

func TestReadFileMalformed(t *testing.T) {
	good := buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "ok"}})
	bad := buildRecord(t, utf8Leader, []tfield{{tag: "001", control: "bad"}})
	copy(bad[12:17], "99999") // base address out of range -> Decode error

	var buf bytes.Buffer
	buf.Write(good)
	buf.Write(bad)
	path := filepath.Join(t.TempDir(), "mix.mrc")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	recs, err := ReadFile(path)
	if err == nil {
		t.Error("expected error from the malformed second record")
	}
	if len(recs) != 1 || recs[0].ControlField("001") != "ok" {
		t.Errorf("got %d records, want the 1 good record before the error", len(recs))
	}
}

func TestWriteErrors(t *testing.T) {
	bad := codex.NewRecord().AddField(codex.Field{Tag: "12", Value: "x"}) // tag not 3 bytes

	if err := NewWriter(&bytes.Buffer{}).Write(bad); err == nil {
		t.Error("Write: expected error for bad tag")
	}
	path := filepath.Join(t.TempDir(), "bad.mrc")
	if err := WriteFile(path, []*codex.Record{bad}); err == nil {
		t.Error("WriteFile: expected error for un-encodable record")
	}
}

// FuzzDecode ensures the parser never panics on arbitrary input and that, when a
// record decodes and re-encodes, Decode->Encode->Decode is stable.
func FuzzDecode(f *testing.F) {
	good := buildRecord(f, utf8Leader, sampleFields())
	f.Add(good)
	f.Add(buildRecord(f, marc8Leader, []tfield{
		{tag: "100", ind1: '1', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "Beyonc\xe2e"}}},
	}))
	f.Add(buildRecord(f, "00000nam a2200000   4600", []tfield{{tag: "001", control: "x"}})) // odd entry map
	f.Add(good[:len(good)/2])                                                               // truncated
	badBase := append([]byte(nil), good...)
	copy(badBase[12:17], "99999")
	f.Add(badBase)
	f.Add([]byte("short"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := Decode(data)
		if err != nil || rec == nil {
			return
		}
		// Encode normalizes the leader (UTF-8, fixed geometry, recomputed
		// length/base), so the first encode is the canonical form; encoding it
		// again must be byte-stable.
		b1, err := Encode(rec)
		if err != nil {
			return // unrepresentable (e.g. a reserved delimiter byte in a value)
		}
		rec1, _, err := Decode(b1)
		if err != nil {
			t.Fatalf("re-decode of encoded record failed: %v", err)
		}
		b2, err := Encode(rec1)
		if err != nil {
			t.Fatalf("re-encode failed: %v", err)
		}
		if !bytes.Equal(b1, b2) {
			t.Errorf("Encode is not idempotent:\n b1 = %q\n b2 = %q", b1, b2)
		}
	})
}

// FuzzEncodeRoundTrip builds a record from fuzzed field data, then checks that
// Encode->Decode reproduces it exactly (when the record is representable).
func FuzzEncodeRoundTrip(f *testing.F) {
	f.Add("ocm1", byte('1'), byte('0'), byte('a'), "Stone butch blues :", "a novel /")
	f.Add("", byte(' '), byte(' '), byte('z'), "", "")
	f.Add("c", byte(0), byte(0), byte('a'), "café", "naïve")

	f.Fuzz(func(t *testing.T, ctrl string, ind1, ind2, code byte, v1, v2 string) {
		norm := func(b byte) byte {
			if b == 0 { // an unset indicator serializes as blank; normalize the input
				return ' '
			}
			return b
		}
		rec := codex.NewRecord().
			AddField(codex.NewControlField("001", ctrl)).
			AddField(codex.NewDataField("245", norm(ind1), norm(ind2),
				codex.NewSubfield(code, v1),
				codex.NewSubfield('b', v2)))

		b, err := Encode(rec)
		if err != nil {
			return // reserved delimiter byte, oversized field, etc.
		}
		got, _, err := Decode(b)
		if err != nil {
			t.Fatalf("decode of encoded record: %v", err)
		}
		// The leader's length/base are recomputed on encode, so compare the field
		// data, which must survive the round trip exactly.
		if !reflect.DeepEqual(rec.Fields(), got.Fields()) {
			t.Errorf("encode round-trip changed fields:\n in  = %#v\n out = %#v", rec.Fields(), got.Fields())
		}
	})
}
