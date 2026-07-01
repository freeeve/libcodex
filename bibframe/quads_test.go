package bibframe

import (
	"bytes"
	"io"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// blankLabels returns the set of _:label tokens in an N-Triples or Turtle
// fragment.
func blankLabels(b []byte) map[string]bool {
	s, m := string(b), map[string]bool{}
	name := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
	}
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '_' && s[i+1] == ':' {
			j := i + 2
			for j < len(s) && name(s[j]) {
				j++
			}
			m[s[i:j]] = true
			i = j
		}
	}
	return m
}

type recWriter interface {
	Write(*codex.Record) error
	Close() error
}

// TestStreamingWritersBlankScope checks the N-Triples and Turtle collection
// writers keep blank-node labels unique across records, so concatenating records
// into one document never merges their blank nodes.
func TestStreamingWritersBlankScope(t *testing.T) {
	recs := []*codex.Record{
		provRecord("rec-A", "Title A", "Publisher A"),
		provRecord("rec-B", "Title B", "Publisher B"),
	}
	for _, tc := range []struct {
		name string
		make func(io.Writer) recWriter
	}{
		{"ntriples", func(w io.Writer) recWriter { return NewNTriplesWriter(w) }},
		{"turtle", func(w io.Writer) recWriter { return NewTurtleWriter(w) }},
	} {
		var buf bytes.Buffer
		w := tc.make(&buf)
		if err := w.Write(recs[0]); err != nil {
			t.Fatal(err)
		}
		n1 := buf.Len()
		if err := w.Write(recs[1]); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()

		first, second := blankLabels(buf.Bytes()[:n1]), blankLabels(buf.Bytes()[n1:])
		if len(first) == 0 {
			t.Fatalf("%s: first record emitted no blank nodes; test is vacuous", tc.name)
		}
		for lbl := range first {
			if second[lbl] {
				t.Errorf("%s: blank %q reused in the second record — records merged", tc.name, lbl)
			}
		}
	}
}

func provRecord(id, title, pub string) *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", id)).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', title))).
		AddField(codex.NewDataField("264", ' ', '1', codex.NewSubfield('b', pub)))
}

// TestNQuadsWriterProvenance streams two records through one NQuadsWriter and
// checks each record's statements land in their own provenance graph and that
// blank nodes never leak between records, even though each numbers its blanks
// from scratch.
func TestNQuadsWriterProvenance(t *testing.T) {
	recs := []*codex.Record{
		provRecord("rec-A", "Title A", "Publisher A"),
		provRecord("rec-B", "Title B", "Publisher B"),
	}

	var buf bytes.Buffer
	w := NewNQuadsWriter(&buf, RecordGraph)
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	ds, err := rdf.ParseNQuads(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	gA, gB := RecordGraph(recs[0], 0), RecordGraph(recs[1], 1)
	if gA == gB {
		t.Fatal("distinct records produced the same provenance graph")
	}
	tA, tB := ds.Graph(gA).Triples, ds.Graph(gB).Triples
	if len(tA) == 0 || len(tB) == 0 {
		t.Fatalf("missing provenance graph(s): A=%d B=%d\n%s", len(tA), len(tB), buf.String())
	}

	blanksIn := func(ts []rdf.Triple) map[string]bool {
		m := map[string]bool{}
		for _, tr := range ts {
			if tr.S.IsBlank() {
				m[tr.S.Value] = true
			}
			if tr.O.IsBlank() {
				m[tr.O.Value] = true
			}
		}
		return m
	}
	bA, bB := blanksIn(tA), blanksIn(tB)
	if len(bA) == 0 {
		t.Fatal("expected the provision activity to emit a blank node; test is vacuous")
	}
	for lbl := range bA {
		if bB[lbl] {
			t.Errorf("blank %q shared across record graphs — records merged", lbl)
		}
	}
}

// TestEncodeNQuadsDefaultGraph checks EncodeNQuads with a zero graph term emits
// plain N-Triples (three-term lines), matching EncodeNTriples.
func TestEncodeNQuadsDefaultGraph(t *testing.T) {
	r := provRecord("x1", "T", "P")
	nq, err := EncodeNQuads(r, rdf.Term{})
	if err != nil {
		t.Fatal(err)
	}
	nt, _ := EncodeNTriples(r)
	if !bytes.Equal(nq, nt) {
		t.Errorf("default-graph N-Quads should equal N-Triples:\n nq: %s\n nt: %s", nq, nt)
	}
}
