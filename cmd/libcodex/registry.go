package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/bibframe"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/dublincore"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/mods"
	"github.com/freeeve/libcodex/mrk"
	"github.com/freeeve/libcodex/schemaorg"
	"github.com/freeeve/libcodex/unimarc"
)

// readerFactory adapts a format's NewReader to the common RecordReader interface.
type readerFactory func(io.Reader) codex.RecordReader

// writerFactory adapts a format's NewWriter to the common RecordWriter interface.
type writerFactory func(io.Writer) codex.RecordWriter

// readers maps a canonical input format name to its reader constructor. Aliases
// (marc/iso2709, xml/marcxml, json/marcjson) resolve to the same codec.
var readers = map[string]readerFactory{
	"marc":     func(r io.Reader) codex.RecordReader { return iso2709.NewReader(r) },
	"iso2709":  func(r io.Reader) codex.RecordReader { return iso2709.NewReader(r) },
	"marcxml":  func(r io.Reader) codex.RecordReader { return marcxml.NewReader(r) },
	"xml":      func(r io.Reader) codex.RecordReader { return marcxml.NewReader(r) },
	"marcjson": func(r io.Reader) codex.RecordReader { return marcjson.NewReader(r) },
	"json":     func(r io.Reader) codex.RecordReader { return marcjson.NewReader(r) },
	"mrk":      func(r io.Reader) codex.RecordReader { return mrk.NewReader(r) },
	"unimarc":  func(r io.Reader) codex.RecordReader { return unimarc.NewReader(r) },
	"bibframe": func(r io.Reader) codex.RecordReader { return bibframe.NewReader(r) },
}

// writers maps a canonical output format name to its writer constructor. The
// record-model MARC formats round-trip; dublincore, mods and schemaorg are
// lossy display projections and are write-only.
var writers = map[string]writerFactory{
	"marc":       func(w io.Writer) codex.RecordWriter { return iso2709.NewWriter(w) },
	"iso2709":    func(w io.Writer) codex.RecordWriter { return iso2709.NewWriter(w) },
	"marcxml":    func(w io.Writer) codex.RecordWriter { return marcxml.NewWriter(w) },
	"xml":        func(w io.Writer) codex.RecordWriter { return marcxml.NewWriter(w) },
	"marcjson":   func(w io.Writer) codex.RecordWriter { return marcjson.NewWriter(w) },
	"json":       func(w io.Writer) codex.RecordWriter { return marcjson.NewWriter(w) },
	"mrk":        func(w io.Writer) codex.RecordWriter { return mrk.NewWriter(w) },
	"bibframe":   func(w io.Writer) codex.RecordWriter { return bibframe.NewWriter(w) },
	"dublincore": func(w io.Writer) codex.RecordWriter { return dublincore.NewWriter(w) },
	"mods":       func(w io.Writer) codex.RecordWriter { return mods.NewWriter(w) },
	"schemaorg":  func(w io.Writer) codex.RecordWriter { return schemaorg.NewWriter(w) },
	"ris":        func(w io.Writer) codex.RecordWriter { return citation.NewRISWriter(w) },
	"bibtex":     func(w io.Writer) codex.RecordWriter { return citation.NewBibTeXWriter(w) },
}

// formatNames returns the distinct canonical names of a registry, sorted, for
// help and error text.
func formatNames[V any](m map[string]V) string {
	seen := make([]string, 0, len(m))
	for k := range m {
		seen = append(seen, k)
	}
	sort.Strings(seen)
	return strings.Join(seen, ", ")
}

// newReader resolves a format name to a reader over r, or errors if the format
// is unknown.
func newReader(format string, r io.Reader) (codex.RecordReader, error) {
	f, ok := readers[strings.ToLower(format)]
	if !ok {
		return nil, fmt.Errorf("unknown input format %q (known: %s)", format, formatNames(readers))
	}
	return f(r), nil
}

// newWriter resolves a format name to a writer over w, or errors if the format
// is unknown.
func newWriter(format string, w io.Writer) (codex.RecordWriter, error) {
	f, ok := writers[strings.ToLower(format)]
	if !ok {
		return nil, fmt.Errorf("unknown output format %q (known: %s)", format, formatNames(writers))
	}
	return f(w), nil
}

// sniff peeks the leading bytes of br to guess the serialization: an ISO 2709
// leader (five digits), an mrk "=LDR" line, JSON (marcjson vs BIBFRAME JSON-LD),
// or one of the RDF text forms — Turtle, N-Triples/N-Quads and RDF/XML all route
// to the bibframe reader, which then autodetects its own sub-format, while plain
// MARCXML routes to marcxml. Returns "" when the format is not recognized. The
// bufio.Reader is left unconsumed.
func sniff(br *bufio.Reader) string {
	head, _ := br.Peek(1024)
	head = bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	s := strings.TrimLeft(string(head), " \t\r\n")
	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "=LDR") || strings.HasPrefix(s, "=00"):
		return "mrk"
	case s[0] == '{' || s[0] == '[':
		if looksJSONLD(s) {
			return "bibframe" // BIBFRAME JSON-LD, not MARC-in-JSON
		}
		return "marcjson"
	case strings.HasPrefix(s, "@prefix"), strings.HasPrefix(s, "@base"),
		strings.HasPrefix(s, "PREFIX"), strings.HasPrefix(s, "BASE"),
		strings.HasPrefix(s, "_:"):
		return "bibframe" // Turtle directive or an N-Triples blank-node subject
	case s[0] == '<':
		return sniffAngle(s)
	case len(s) >= 5 && isDigits(s[:5]):
		return "iso2709"
	}
	return ""
}

// looksJSONLD reports whether a JSON document carries JSON-LD keywords, which
// distinguish a BIBFRAME graph from a MARC-in-JSON record (`leader`/`fields`).
func looksJSONLD(s string) bool {
	return strings.Contains(s, "@context") || strings.Contains(s, "@graph") ||
		strings.Contains(s, "@id") || strings.Contains(s, "@type")
}

// sniffAngle classifies a document that begins with '<': an RDF term IRI
// (`<scheme://…>` or `<urn:…>`) marks Turtle/N-Triples/N-Quads, an rdf:RDF /
// bibframe marker marks RDF/XML — both handled by the bibframe reader — and
// anything else is plain MARCXML.
func sniffAngle(s string) string {
	first := s
	if i := strings.IndexAny(s, "> \t\r\n"); i >= 0 {
		first = s[:i]
	}
	if strings.Contains(first, "://") || strings.HasPrefix(first, "<urn:") {
		return "bibframe" // an N-Triples/N-Quads/Turtle subject IRI
	}
	if strings.Contains(s, "rdf:RDF") || strings.Contains(s, "/bibframe/") || strings.Contains(s, "bf:") {
		return "bibframe" // BIBFRAME RDF/XML
	}
	return "marcxml"
}

// isDigits reports whether every byte of s is an ASCII digit.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
