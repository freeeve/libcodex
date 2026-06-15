package marcxml

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
)

// benchRecord is a representative ~10-field bibliographic record.
func benchRecord() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "ocm00000001")).
		AddField(codex.NewControlField("003", "OCoLC")).
		AddField(codex.NewControlField("008", "210101s2021    nyu           000 1 eng d")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "9781555838539"))).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Feinberg, Leslie,"), codex.NewSubfield('d', "1949-2014."))).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Stone butch blues :"),
			codex.NewSubfield('b', "a novel /"),
			codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("264", ' ', '1',
			codex.NewSubfield('a', "Ithaca, New York :"),
			codex.NewSubfield('b', "Firebrand Books,"),
			codex.NewSubfield('c', "[1993]"))).
		AddField(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "301 pages ;"), codex.NewSubfield('c', "22 cm"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction."))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Gender identity"), codex.NewSubfield('v', "Fiction.")))
}

func BenchmarkDecode(b *testing.B) {
	raw, err := Encode(benchRecord())
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	rec := benchRecord()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Encode(rec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReaderStream(b *testing.B) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for range 100 {
		if err := w.Write(benchRecord()); err != nil {
			b.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	data := buf.String()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		r := NewReader(strings.NewReader(data))
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

func BenchmarkWriterStream(b *testing.B) {
	rec := benchRecord()
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
		if err := w.Close(); err != nil {
			b.Fatal(err)
		}
	}
	b.SetBytes(int64(buf.Len()))
}
