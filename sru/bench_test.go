package sru

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkParseResponse measures parsing a two-record searchRetrieve response,
// the hot path of every page fetched by the Reader.
func BenchmarkParseResponse(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("testdata", "search_page1.xml"))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := parseResponse(data); err != nil {
			b.Fatal(err)
		}
	}
}
