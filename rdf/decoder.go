package rdf

import (
	"bufio"
	"io"
	"iter"
)

// Format identifies a streaming RDF serialization.
type Format int

const (
	// NTriples is the line-based N-Triples format.
	NTriples Format = iota
	// NQuads is N-Quads; the optional graph label on each line is ignored, so each
	// statement still yields a triple.
	NQuads
)

// Decoder streams RDF statements from an io.Reader one triple at a time, in
// constant memory — for inputs far too large to materialize into a Graph
// (multi-gigabyte dumps such as the LC authority files). Each returned triple owns
// its strings, so it is safe to retain after the next call.
//
// Only the line-based serializations (N-Triples, N-Quads) stream; for the others,
// parse the whole document with ParseRDFXML, ParseJSONLD or ParseTurtle.
type Decoder struct {
	br *bufio.Reader
}

// NewDecoder returns a streaming Decoder reading the given format from r. The
// format selects the grammar; both line-based formats are parsed identically (the
// N-Quads graph term is ignored).
func NewDecoder(r io.Reader, format Format) *Decoder {
	_ = format // NTriples and NQuads share the line parser
	return &Decoder{br: bufio.NewReader(r)}
}

// Decode returns the next triple, or io.EOF when the stream is exhausted. Blank,
// comment and malformed lines are skipped, matching ParseNTriples, so a stray line
// never aborts a large stream.
func (d *Decoder) Decode() (Triple, error) {
	for {
		line, err := d.br.ReadString('\n')
		if len(line) > 0 {
			if tr, ok := parseNTLine(line, nil); ok {
				return tr, nil
			}
		}
		if err != nil {
			return Triple{}, err // io.EOF at a clean end of stream
		}
	}
}

// All returns an iterator over the remaining triples, ending at the first error
// (io.EOF, the normal end, is not surfaced):
//
//	for tr := range dec.All() { ... }
//
// Use Decode directly when a non-EOF read error must be observed.
func (d *Decoder) All() iter.Seq[Triple] {
	return func(yield func(Triple) bool) {
		for {
			tr, err := d.Decode()
			if err != nil {
				return
			}
			if !yield(tr) {
				return
			}
		}
	}
}
