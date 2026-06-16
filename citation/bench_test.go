package citation

import (
	"bytes"
	"testing"
)

func BenchmarkRIS(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := RIS(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBibTeX(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := BibTeX(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRISWriterStream(b *testing.B) {
	rec := sample()
	buf := bytes.NewBuffer(make([]byte, 0, 256*1024))
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		w := NewRISWriter(buf)
		for range 100 {
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.SetBytes(int64(buf.Len()))
}
