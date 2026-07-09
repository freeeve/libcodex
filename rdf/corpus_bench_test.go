package rdf

import (
	"strconv"
	"strings"
	"testing"
)

// corpusNQ builds an N-Quads document shaped like a corpus-scale BIBFRAME
// catalog dump: per work a cluster of subjects (the work, two instances, two
// items) carrying 4-10 triples each, IRIs sharing long prefixes, literals with
// occasional escapes, language tags and typed dates, spread across two named
// graphs. At 10,000 works it is ~325k quads / ~50MB — the scale the projector
// in libcat runs at.
func corpusNQ(works int) []byte {
	const (
		bf    = "http://id.loc.gov/ontologies/bibframe/"
		feedG = " <http://catalog.example.org/graphs/feed> .\n"
		viewG = " <http://catalog.example.org/graphs/views> .\n"
	)
	var b strings.Builder
	b.Grow(works * 5200)
	for i := range works {
		id := strconv.Itoa(i)
		w := "<http://catalog.example.org/works/w" + id + ">"
		title := "Title of work " + id
		if i%8 == 0 { // escaped literals exercise the arena path
			title = "Title \\\"of\\\" work\\t" + id
		}
		b.WriteString(w + " <" + TypeIRI + "> <" + bf + "Work>" + feedG)
		b.WriteString(w + " <" + bf + "title> \"" + title + "\"@en" + feedG)
		b.WriteString(w + " <" + bf + "language> <http://id.loc.gov/vocabulary/languages/eng>" + feedG)
		b.WriteString(w + " <" + bf + "classification> \"813." + strconv.Itoa(i%10) + "\"" + feedG)
		for c := range 2 {
			b.WriteString(w + " <" + bf + "contribution> <http://catalog.example.org/agents/a" + strconv.Itoa((i*3+c)%997) + ">" + feedG)
		}
		for s := range 3 {
			b.WriteString(w + " <" + bf + "subject> <http://catalog.example.org/subjects/s" + strconv.Itoa((i*7+s)%1499) + ">" + feedG)
		}
		for inst := range 2 {
			in := "<http://catalog.example.org/instances/w" + id + "-i" + strconv.Itoa(inst) + ">"
			b.WriteString(in + " <" + TypeIRI + "> <" + bf + "Instance>" + feedG)
			b.WriteString(in + " <" + bf + "instanceOf> " + w + feedG)
			b.WriteString(in + " <" + bf + "title> \"" + title + " (edition " + strconv.Itoa(inst) + ")\"" + feedG)
			b.WriteString(in + " <" + bf + "identifiedBy> \"978030758" + id + strconv.Itoa(inst) + "\"" + feedG)
			b.WriteString(in + " <" + bf + "provisionActivity> \"Example City : Example House\"" + feedG)
			b.WriteString(in + " <" + bf + "date> \"20" + strconv.Itoa(i%25) + "-01-01\"^^<http://www.w3.org/2001/XMLSchema#date>" + feedG)
			b.WriteString(in + " <" + bf + "extent> \"" + strconv.Itoa(100+i%400) + " pages\"" + feedG)
			b.WriteString(in + " <" + bf + "media> <http://id.loc.gov/vocabulary/mediaTypes/n>" + feedG)
			for item := range 1 {
				it := "<http://catalog.example.org/items/w" + id + "-i" + strconv.Itoa(inst) + "-c" + strconv.Itoa(item) + ">"
				b.WriteString(it + " <" + TypeIRI + "> <" + bf + "Item>" + viewG)
				b.WriteString(it + " <" + bf + "itemOf> " + in + viewG)
				b.WriteString(it + " <" + bf + "shelfMark> \"QLL " + id + "." + strconv.Itoa(inst) + "\"" + viewG)
				b.WriteString(it + " <" + bf + "heldBy> <http://catalog.example.org/orgs/branch" + strconv.Itoa(i%12) + ">" + viewG)
			}
		}
	}
	return []byte(b.String())
}

const corpusWorks = 10_000

// bfWorkClass is the rdf:type the corpus gives its Work subjects, probed by the
// graph-split benchmarks.
const bfWorkClass = "http://id.loc.gov/ontologies/bibframe/Work"

// BenchmarkCorpusParseNQuads parses the corpus-scale document — the parse half
// of the profile task 083 tracks.
func BenchmarkCorpusParseNQuads(b *testing.B) {
	data := corpusNQ(corpusWorks)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		d, err := ParseNQuads(data)
		if err != nil || len(d.Quads) == 0 {
			b.Fatalf("parse: %v (%d quads)", err, len(d.Quads))
		}
	}
}

// BenchmarkCorpusParseNQuadsShared is the parse benchmark through the zero-copy
// variant, which skips the private input copy.
func BenchmarkCorpusParseNQuadsShared(b *testing.B) {
	data := corpusNQ(corpusWorks)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		d, err := ParseNQuadsShared(data)
		if err != nil || len(d.Quads) == 0 {
			b.Fatalf("parse: %v (%d quads)", err, len(d.Quads))
		}
	}
}

// BenchmarkCorpusDatasetGraph splits one named graph out of a corpus-scale
// dataset the materializing way: Dataset.Graph copies a Triple per matching
// quad. It is the allocator libcat's profile (task 098) found at the top of both
// its per-grain scan and its projection paths.
func BenchmarkCorpusDatasetGraph(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	feed := NewIRI("http://catalog.example.org/graphs/feed")
	b.ReportAllocs()
	for b.Loop() {
		g := d.Graph(feed)
		if len(g.SubjectsOfType(bfWorkClass)) == 0 {
			b.Fatal("no works in the feed graph")
		}
	}
}

// BenchmarkCorpusDatasetGraphView is the same split-and-query through the
// zero-copy view, which indexes int32 positions into the dataset's own quads.
func BenchmarkCorpusDatasetGraphView(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	feed := NewIRI("http://catalog.example.org/graphs/feed")
	b.ReportAllocs()
	for b.Loop() {
		v := d.GraphView(feed)
		if len(v.SubjectsOfType(bfWorkClass)) == 0 {
			b.Fatal("no works in the feed graph")
		}
	}
}

// BenchmarkCorpusDatasetGraphQuery and its View counterpart cover the other half
// of the split-and-read pattern: the subject-keyed lookups (Object/Literal), which
// do build a subject index on both paths. This is the shape libcat's per-grain
// ScanGrain runs, so the view must win here too and not only on the scans.
func BenchmarkCorpusDatasetGraphQuery(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	feed := NewIRI("http://catalog.example.org/graphs/feed")
	probe := NewIRI("http://catalog.example.org/works/w1")
	b.ReportAllocs()
	for b.Loop() {
		g := d.Graph(feed)
		if _, ok := g.Object(probe, TypeIRI); !ok {
			b.Fatal("probe subject missing")
		}
	}
}

func BenchmarkCorpusDatasetGraphViewQuery(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	feed := NewIRI("http://catalog.example.org/graphs/feed")
	probe := NewIRI("http://catalog.example.org/works/w1")
	b.ReportAllocs()
	for b.Loop() {
		v := d.GraphView(feed)
		if _, ok := v.Object(probe, TypeIRI); !ok {
			b.Fatal("probe subject missing")
		}
	}
}

// BenchmarkCorpusIndex builds the lazy subject index over a corpus-scale graph
// — the index half of the profile task 083 tracks. Each iteration starts from
// a fresh Graph so the cached index is rebuilt.
func BenchmarkCorpusIndex(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	ts := d.Graph(NewIRI("http://catalog.example.org/graphs/feed")).Triples
	probe := NewIRI("http://catalog.example.org/works/w1")
	b.ReportAllocs()
	for b.Loop() {
		g := &Graph{Triples: ts}
		if _, ok := g.Object(probe, TypeIRI); !ok {
			b.Fatal("probe subject missing from index")
		}
	}
}
