package bibframe

import (
	"fmt"
	"io"
	"os"

	"github.com/freeeve/libcodex"
)

// ---- entry points ----

// Encode converts a record to a standalone BIBFRAME RDF/XML document.
func Encode(r *codex.Record) ([]byte, error) {
	b := make([]byte, 0, 4096)
	b = append(b, xmlHeader...)
	b = append(b, '\n')
	b = append(b, rdfOpen...)
	b = append(b, '\n')
	b = appendGraphXML(b, FromRecord(r), resolveBase(r, 0))
	b = append(b, rdfClose...)
	return append(b, '\n'), nil
}

// EncodeJSONLD converts a record to a standalone BIBFRAME JSON-LD document.
func EncodeJSONLD(r *codex.Record) ([]byte, error) {
	b := make([]byte, 0, 2048)
	b = append(b, jsonldContext...)
	b = append(b, `,"@graph":[`...)
	b = appendGraphJSONLD(b, FromRecord(r), resolveBase(r, 0))
	return append(b, "]}"...), nil
}

// RDFXML serializes a Work with N Instances to a standalone BIBFRAME RDF/XML
// document: the Work at #<workBase>Work with one bf:hasInstance per Instance, and
// each Instance at #<instanceBases[i]>Instance linked bf:instanceOf back. Every
// base is sanitized like BIBFRAME.Graph. The result parses to a graph isomorphic
// to WorkInstances.Graph(workBase, instanceBases). It errors if instanceBases
// does not match wi.Instances in length. N-Triples, Turtle and N-Quads come from
// the Graph method's serializers.
func (wi *WorkInstances) RDFXML(workBase string, instanceBases []string) ([]byte, error) {
	if len(instanceBases) != len(wi.Instances) {
		return nil, fmt.Errorf("bibframe: WorkInstances.RDFXML: %d instanceBases for %d Instances", len(instanceBases), len(wi.Instances))
	}
	b := make([]byte, 0, 4096)
	b = append(b, xmlHeader...)
	b = append(b, '\n')
	b = append(b, rdfOpen...)
	b = append(b, '\n')
	b = appendWorkInstancesXML(b, wi, workBase, instanceBases)
	b = append(b, rdfClose...)
	return append(b, '\n'), nil
}

// JSONLD serializes a Work with N Instances to a standalone BIBFRAME JSON-LD
// document, the JSON-LD counterpart of RDFXML with the same graph shape and
// length contract.
func (wi *WorkInstances) JSONLD(workBase string, instanceBases []string) ([]byte, error) {
	if len(instanceBases) != len(wi.Instances) {
		return nil, fmt.Errorf("bibframe: WorkInstances.JSONLD: %d instanceBases for %d Instances", len(instanceBases), len(wi.Instances))
	}
	b := make([]byte, 0, 2048)
	b = append(b, jsonldContext...)
	b = append(b, `,"@graph":[`...)
	b = appendWorkInstancesJSONLD(b, wi, workBase, instanceBases)
	return append(b, "]}"...), nil
}

// ---- RDF/XML writer ----

// Writer converts records and writes them as an rdf:RDF graph. Close must be
// called to emit the closing tag.
type Writer struct {
	w      io.Writer
	buf    []byte
	idx    int
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a BIBFRAME RDF/XML graph to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write converts one record and writes its Work and Instance nodes.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return errWriteAfterClose
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + rdfOpen + "\n")); err != nil {
			return err
		}
	}
	wr.buf = appendGraphXML(wr.buf[:0], FromRecord(r), resolveBase(r, wr.idx))
	wr.idx++
	return wr.writeAll(wr.buf)
}

// Close writes the closing </rdf:RDF> tag.
func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + rdfOpen + "\n")); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte(rdfClose + "\n"))
}

func (wr *Writer) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// ---- JSON-LD writer ----

// JSONLDWriter converts records and writes them into a JSON-LD @graph array.
// Close must be called to terminate the document.
type JSONLDWriter struct {
	w      io.Writer
	buf    []byte
	idx    int
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*JSONLDWriter)(nil)

// NewJSONLDWriter returns a Writer that writes a BIBFRAME JSON-LD document to w.
func NewJSONLDWriter(w io.Writer) *JSONLDWriter { return &JSONLDWriter{w: w} }

func (wr *JSONLDWriter) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return errWriteAfterClose
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(jsonldContext + `,"@graph":[`)); err != nil {
			return err
		}
	}
	wr.buf = wr.buf[:0]
	if wr.idx > 0 {
		wr.buf = append(wr.buf, ',')
	}
	wr.buf = appendGraphJSONLD(wr.buf, FromRecord(r), resolveBase(r, wr.idx))
	wr.idx++
	return wr.writeAll(wr.buf)
}

func (wr *JSONLDWriter) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(jsonldContext + `,"@graph":[`)); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte("]}\n"))
}

func (wr *JSONLDWriter) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// ---- file helpers ----

// WriteFile converts every record to a BIBFRAME RDF/XML file.
func WriteFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewWriter(w) })
}

// WriteJSONLDFile converts every record to a BIBFRAME JSON-LD file.
func WriteJSONLDFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewJSONLDWriter(w) })
}

type closableWriter interface {
	Write(*codex.Record) error
	Close() error
}

func writeFile(path string, records []*codex.Record, newW func(io.Writer) closableWriter) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := newW(f)
	for _, rec := range records {
		if err := w.Write(rec); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
