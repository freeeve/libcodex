package bibframe

import (
	"io"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/rdf"
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

// WriteNTriplesFile writes every record to path as one N-Triples document.
func WriteNTriplesFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewNTriplesWriter(w) })
}

// WriteTurtleFile writes every record to path as one Turtle document.
func WriteTurtleFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewTurtleWriter(w) })
}
