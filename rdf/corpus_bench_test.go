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

// The next four benchmarks measure what libcat reported in task 099: the
// per-triple call overhead of the iter.Seq yielded by GraphView.Triples, against
// a direct loop over the dataset's quads. The overhead is per triple, so it is
// visible on a corpus-scale walk and immaterial at per-grain sizes.

// sinkLen keeps the walk benchmarks from being optimized away. It accumulates a
// scalar rather than storing the Triple, so the two loops are compared on their
// iteration cost and not on a 168-byte copy into a global that dwarfs it.
var sinkLen int

func benchTriplesIter(b *testing.B, d *Dataset, graph Term) {
	v := d.GraphView(graph)
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for t := range v.Triples() {
			n += len(t.S.Value)
		}
		sinkLen = n
	}
}

func benchTriplesDirect(b *testing.B, d *Dataset, graph Term) {
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for i := range d.Quads {
			if q := &d.Quads[i]; q.G == graph {
				t := q.Triple() // materialize the same value Triples() yields
				n += len(t.S.Value)
			}
		}
		sinkLen = n
	}
}

// grainDataset is one grain's worth of quads (~200), the size libcat's
// per-grain paths walk.
func grainDataset(b *testing.B) (*Dataset, Term) {
	b.Helper()
	d, err := ParseNQuads(corpusNQ(6))
	if err != nil {
		b.Fatal(err)
	}
	return d, NewIRI("http://catalog.example.org/graphs/feed")
}

func BenchmarkCorpusTriplesIter(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	benchTriplesIter(b, d, NewIRI("http://catalog.example.org/graphs/feed"))
}

func BenchmarkCorpusTriplesDirect(b *testing.B) {
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	benchTriplesDirect(b, d, NewIRI("http://catalog.example.org/graphs/feed"))
}

// singleGraphDataset models libcat's merge path (task 099): a dataset with only
// one graph, which a direct loop can walk with no graph filter at all. Compared
// against the iterator over the same dataset it isolates the cost of
// GraphView.Triples's per-quad graph-term comparison from the iter.Seq yield.
// Measured on Go 1.25 / M3 Max the iterator still wins, so the per-quad filter
// does not explain the iterator overhead libcat reported at 12.7M quads.
func singleGraphDataset(b *testing.B) (*Dataset, Term) {
	b.Helper()
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	graph := NewIRI("http://catalog.example.org/graphs/feed")
	one := &Dataset{Quads: make([]Quad, 0, len(d.Quads))}
	for _, q := range d.Quads {
		if q.G == graph {
			one.Quads = append(one.Quads, q)
		}
	}
	return one, graph
}

func BenchmarkSingleGraphTriplesIter(b *testing.B) {
	d, graph := singleGraphDataset(b)
	benchTriplesIter(b, d, graph)
}

func BenchmarkSingleGraphTriplesUnfiltered(b *testing.B) {
	d, _ := singleGraphDataset(b)
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for i := range d.Quads {
			t := d.Quads[i].Triple()
			n += len(t.S.Value)
		}
		sinkLen = n
	}
}

func BenchmarkGrainTriplesIter(b *testing.B) {
	d, graph := grainDataset(b)
	benchTriplesIter(b, d, graph)
}

func BenchmarkGrainTriplesDirect(b *testing.B) {
	d, graph := grainDataset(b)
	benchTriplesDirect(b, d, graph)
}

// The next three benchmarks reproduce libcat's merge shape (task 100): read a
// populated feed graph and an *empty* editorial overlay out of one dataset. It is
// the case that exposed the real cost of a view — one full-dataset pass per view,
// including for the graph that turns out to be empty.

// mergeDataset is a corpus whose editorial overlay graph carries no statements,
// the common no-editorial case libcat's projector merges.
func mergeDataset(b *testing.B) (d *Dataset, feed, editorial Term) {
	b.Helper()
	d, err := ParseNQuads(corpusNQ(corpusWorks))
	if err != nil {
		b.Fatal(err)
	}
	return d, NewIRI("http://catalog.example.org/graphs/feed"),
		NewIRI("http://catalog.example.org/graphs/editorial")
}

// BenchmarkMergeFused is libcat's shipping hand-written merge: one count pass
// switching on the graph term, then one append pass for feed, with the editorial
// pass skipped entirely when the count says zero. Two passes.
func BenchmarkMergeFused(b *testing.B) {
	d, feed, editorial := mergeDataset(b)
	b.ReportAllocs()
	for b.Loop() {
		nf, ne := 0, 0
		for i := range d.Quads {
			switch d.Quads[i].G {
			case feed:
				nf++
			case editorial:
				ne++
			}
		}
		out := make([]Triple, 0, nf+ne)
		for i := range d.Quads {
			if q := &d.Quads[i]; q.G == feed {
				out = append(out, q.Triple())
			}
		}
		if ne > 0 {
			for i := range d.Quads {
				if q := &d.Quads[i]; q.G == editorial {
					out = append(out, q.Triple())
				}
			}
		}
		sinkLen = len(out)
	}
}

// BenchmarkMergeViewsNoSkip is the view version libcat removed: Len on each view
// plus a Triples walk on each, with no emptiness check. The counts pass is one,
// shared, but the editorial graph is still walked across the whole dataset only to
// yield nothing. Three passes.
func BenchmarkMergeViewsNoSkip(b *testing.B) {
	d, feed, editorial := mergeDataset(b)
	b.ReportAllocs()
	for b.Loop() {
		d.counts = nil // each merge sees a freshly parsed dataset, so pay the counts pass
		fv, ev := d.GraphView(feed), d.GraphView(editorial)
		out := make([]Triple, 0, fv.Len()+ev.Len())
		for tr := range fv.Triples() {
			out = append(out, tr)
		}
		for tr := range ev.Triples() {
			out = append(out, tr)
		}
		sinkLen = len(out)
	}
}

// BenchmarkMergeViewsEmptySkip is the same merge written against the view API as
// it now stands: Len and Empty read the dataset's cached counts (one shared pass),
// and the empty editorial graph is skipped without a walk. Two passes, the same
// pass count as the fused hand-written merge.
func BenchmarkMergeViewsEmptySkip(b *testing.B) {
	d, feed, editorial := mergeDataset(b)
	b.ReportAllocs()
	for b.Loop() {
		d.counts = nil // each merge sees a freshly parsed dataset, so pay the counts pass
		fv, ev := d.GraphView(feed), d.GraphView(editorial)
		out := make([]Triple, 0, fv.Len()+ev.Len())
		for tr := range fv.Triples() {
			out = append(out, tr)
		}
		if !ev.Empty() {
			for tr := range ev.Triples() {
				out = append(out, tr)
			}
		}
		sinkLen = len(out)
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
