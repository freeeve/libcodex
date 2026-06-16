package schemaorg

import (
	"bytes"
	"testing"
)

func BenchmarkEncode(b *testing.B) {
	rec := sample()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Encode(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriterStream(b *testing.B) {
	rec := sample()
	buf := bytes.NewBuffer(make([]byte, 0, 256*1024))
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
