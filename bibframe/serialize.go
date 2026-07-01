package bibframe

import (
	"io"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// bibframePrefixes are the namespace prefixes declared in Turtle output.
var bibframePrefixes = map[string]string{
	"bf":   bfNS,
	"bflc": bflcNS,
	"rdf":  rdfNS,
	"rdfs": rdfsNS,
}

// EncodeNTriples converts a record to a standalone BIBFRAME N-Triples document.
func EncodeNTriples(r *codex.Record) ([]byte, error) {
	return graphFromRecord(r).NTriples(), nil
}

// EncodeTurtle converts a record to a standalone BIBFRAME Turtle document.
func EncodeTurtle(r *codex.Record) ([]byte, error) {
	return graphFromRecord(r).Turtle(bibframePrefixes), nil
}

// EncodeNQuads converts a record to a BIBFRAME N-Quads document whose statements
// are all tagged with the given graph term — the record's provenance, or named
// graph. A zero-value graph term produces plain N-Triples (the default graph).
func EncodeNQuads(r *codex.Record, graph rdf.Term) ([]byte, error) {
	return graphFromRecord(r).NQuads(graph), nil
}

// RecordGraph returns a per-record provenance graph term derived from the
// record's control number (field 001), suitable as the graphFor argument to
// NewNQuadsWriter so each record's statements land in their own named graph.
func RecordGraph(r *codex.Record) rdf.Term {
	return rdf.NewIRI("#" + resolveBase(r, 0))
}

// NTriplesWriter writes a collection of records as N-Triples. Because N-Triples
// has no document framing, records simply concatenate; Close is a no-op kept for
// API symmetry with the other writers.
type NTriplesWriter struct {
	w   io.Writer
	err error
}

// NewNTriplesWriter returns an NTriplesWriter over w. It implements
// codex.RecordWriter, so it works as a codex.Convert target.
func NewNTriplesWriter(w io.Writer) *NTriplesWriter { return &NTriplesWriter{w: w} }

// Write serializes one record's BIBFRAME graph.
func (nw *NTriplesWriter) Write(r *codex.Record) error {
	if nw.err != nil {
		return nw.err
	}
	if _, err := nw.w.Write(graphFromRecord(r).NTriples()); err != nil {
		nw.err = err
	}
	return nw.err
}

// Close reports the first write error, if any.
func (nw *NTriplesWriter) Close() error { return nw.err }

// TurtleWriter writes a collection of records as Turtle, emitting the @prefix
// header once before the first record.
type TurtleWriter struct {
	w       io.Writer
	started bool
	err     error
}

// NewTurtleWriter returns a TurtleWriter over w. It implements codex.RecordWriter.
func NewTurtleWriter(w io.Writer) *TurtleWriter { return &TurtleWriter{w: w} }

// Write serializes one record, preceded by the prefix header on the first call.
func (tw *TurtleWriter) Write(r *codex.Record) error {
	if tw.err != nil {
		return tw.err
	}
	if !tw.started {
		tw.started = true
		if _, err := tw.w.Write(rdf.TurtleHeader(bibframePrefixes)); err != nil {
			tw.err = err
			return err
		}
	}
	if _, err := tw.w.Write(graphFromRecord(r).TurtleBody(bibframePrefixes)); err != nil {
		tw.err = err
	}
	return tw.err
}

// Close reports the first write error, if any.
func (tw *TurtleWriter) Close() error { return tw.err }

// NQuadsWriter writes a collection of records as N-Quads, tagging each record's
// statements with a provenance graph term. It reuses one encoder so blank-node
// labels stay unique across the whole stream — otherwise two records that each
// number their blanks from scratch would collide and merge in the output.
type NQuadsWriter struct {
	w        io.Writer
	graphFor func(*codex.Record) rdf.Term
	enc      rdf.NQuadsEncoder
	err      error
}

// NewNQuadsWriter returns an NQuadsWriter over w. graphFor maps each record to
// its provenance graph term; a nil graphFor (or one returning a zero-value term)
// writes the default graph, equivalent to N-Triples. It implements
// codex.RecordWriter, so it works as a codex.Convert target. Pass RecordGraph
// for a per-record named graph keyed on the control number.
func NewNQuadsWriter(w io.Writer, graphFor func(*codex.Record) rdf.Term) *NQuadsWriter {
	return &NQuadsWriter{w: w, graphFor: graphFor}
}

// Write serializes one record's BIBFRAME graph as N-Quads under its graph term.
func (nw *NQuadsWriter) Write(r *codex.Record) error {
	if nw.err != nil {
		return nw.err
	}
	var g rdf.Term
	if nw.graphFor != nil {
		g = nw.graphFor(r)
	}
	if _, err := nw.w.Write(nw.enc.AppendGraph(nil, graphFromRecord(r), g)); err != nil {
		nw.err = err
	}
	return nw.err
}

// Close reports the first write error, if any.
func (nw *NQuadsWriter) Close() error { return nw.err }

// WriteNTriplesFile writes every record to path as one N-Triples document.
func WriteNTriplesFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewNTriplesWriter(w) })
}

// WriteTurtleFile writes every record to path as one Turtle document.
func WriteTurtleFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewTurtleWriter(w) })
}

// WriteNQuadsFile writes every record to path as one N-Quads document, tagging
// each record's statements with the graph term returned by graphFor.
func WriteNQuadsFile(path string, records []*codex.Record, graphFor func(*codex.Record) rdf.Term) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewNQuadsWriter(w, graphFor) })
}
