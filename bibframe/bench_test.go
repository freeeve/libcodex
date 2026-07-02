package bibframe

import (
	"bytes"
	"testing"
)

func BenchmarkFromRecord(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		_ = FromRecord(rec)
	}
}

func BenchmarkEncode(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Encode(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeJSONLD(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := EncodeJSONLD(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriterStream(b *testing.B) {
	rec := sample()
	buf := bytes.NewBuffer(make([]byte, 0, 512*1024))
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		w := NewWriter(buf)
		for range 100 {
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
		w.Close()
	}
	b.SetBytes(int64(buf.Len()))
}

// BenchmarkNTriplesWriterStream covers the N-Triples collection writer, whose
// per-record buffer is reused across Write calls rather than allocated fresh.
func BenchmarkNTriplesWriterStream(b *testing.B) {
	rec := sample()
	buf := bytes.NewBuffer(make([]byte, 0, 512*1024))
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		w := NewNTriplesWriter(buf)
		for range 100 {
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
		w.Close()
	}
	b.SetBytes(int64(buf.Len()))
}

func BenchmarkJSONLDWriterStream(b *testing.B) {
	rec := sample()
	buf := bytes.NewBuffer(make([]byte, 0, 512*1024))
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		w := NewJSONLDWriter(buf)
		for range 100 {
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
		w.Close()
	}
	b.SetBytes(int64(buf.Len()))
}
