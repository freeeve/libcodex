package iso2709

import (
	"bytes"
	"io"
	"testing"

	"github.com/freeeve/libcodex"
)

// benchFields returns a realistic ~15-field bibliographic record used to size
// the read/write benchmarks against representative catalog data rather than a
// toy record.
func benchFields() []tfield {
	return []tfield{
		{tag: "001", control: "ocm00000001"},
		{tag: "003", control: "OCoLC"},
		{tag: "005", control: "20210101000000.0"},
		{tag: "008", control: "210101s2021    nyu           000 1 eng d"},
		{tag: "020", ind1: ' ', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "9781555838539"}}},
		{tag: "040", ind1: ' ', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "DLC"}, {Code: 'c', Value: "DLC"}}},
		{tag: "100", ind1: '1', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "Feinberg, Leslie,"}, {Code: 'd', Value: "1949-2014."}}},
		{tag: "245", ind1: '1', ind2: '0', subs: []codex.Subfield{
			{Code: 'a', Value: "Stone butch blues :"},
			{Code: 'b', Value: "a novel /"},
			{Code: 'c', Value: "Leslie Feinberg."},
		}},
		{tag: "250", ind1: ' ', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "First edition."}}},
		{tag: "264", ind1: ' ', ind2: '1', subs: []codex.Subfield{
			{Code: 'a', Value: "Ithaca, New York :"},
			{Code: 'b', Value: "Firebrand Books,"},
			{Code: 'c', Value: "[1993]"},
		}},
		{tag: "300", ind1: ' ', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "301 pages ;"}, {Code: 'c', Value: "22 cm"}}},
		{tag: "336", ind1: ' ', ind2: ' ', subs: []codex.Subfield{{Code: 'a', Value: "text"}, {Code: 'b', Value: "txt"}, {Code: '2', Value: "rdacontent"}}},
		{tag: "650", ind1: ' ', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Lesbians"}, {Code: 'v', Value: "Fiction."}}},
		{tag: "650", ind1: ' ', ind2: '0', subs: []codex.Subfield{{Code: 'a', Value: "Gender identity"}, {Code: 'v', Value: "Fiction."}}},
		{tag: "655", ind1: ' ', ind2: '7', subs: []codex.Subfield{{Code: 'a', Value: "Bildungsromans."}, {Code: '2', Value: "lcgft"}}},
	}
}

// BenchmarkDecode measures decoding a single record from its raw bytes.
func BenchmarkDecode(b *testing.B) {
	raw := buildRecord(b, utf8Leader, benchFields())
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Decode(raw); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncode measures encoding a single record to ISO 2709 bytes.
func BenchmarkEncode(b *testing.B) {
	rec, _, err := Decode(buildRecord(b, utf8Leader, benchFields()))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Encode(rec); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReaderStream measures streaming many records through a Reader,
// exercising leader-length framing and per-record buffer allocation.
func BenchmarkReaderStream(b *testing.B) {
	const n = 100
	stream := bytes.Repeat(buildRecord(b, utf8Leader, benchFields()), n)
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	for b.Loop() {
		r := NewReader(bytes.NewReader(stream))
		for {
			if _, err := r.Read(); err != nil {
				if err == io.EOF {
					break
				}
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkWriterStream measures writing many records through a Writer into a
// reused buffer, isolating per-record encoding cost from buffer growth.
func BenchmarkWriterStream(b *testing.B) {
	rec, _, err := Decode(buildRecord(b, utf8Leader, benchFields()))
	if err != nil {
		b.Fatal(err)
	}
	const n = 100
	buf := bytes.NewBuffer(make([]byte, 0, 64*1024))
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		w := NewWriter(buf)
		for range n {
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.SetBytes(int64(buf.Len()))
}

// BenchmarkRoundTrip measures a full decode-then-encode cycle.
func BenchmarkRoundTrip(b *testing.B) {
	raw := buildRecord(b, utf8Leader, benchFields())
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		rec, _, err := Decode(raw)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := Encode(rec); err != nil {
			b.Fatal(err)
		}
	}
}
