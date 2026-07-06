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
// leader (five digits), an XML document (marcxml vs bibframe RDF/XML), a JSON
// array/object (marcjson), or an mrk "=LDR" line. It returns "" when the format
// is not recognized. The bufio.Reader is left unconsumed.
func sniff(br *bufio.Reader) string {
	head, _ := br.Peek(512)
	head = bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	trimmed := strings.TrimLeft(string(head), " \t\r\n")
	switch {
	case trimmed == "":
		return ""
	case strings.HasPrefix(trimmed, "=LDR") || strings.HasPrefix(trimmed, "=00"):
		return "mrk"
	case trimmed[0] == '{' || trimmed[0] == '[':
		return "marcjson"
	case trimmed[0] == '<':
		if strings.Contains(trimmed, "RDF") || strings.Contains(trimmed, "bf:") {
			return "bibframe"
		}
		return "marcxml"
	case len(trimmed) >= 5 && isDigits(trimmed[:5]):
		return "iso2709"
	}
	return ""
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
