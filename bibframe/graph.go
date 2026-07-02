package bibframe

import (
	"strconv"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// graphFromRecord builds the RDF graph of a record's BIBFRAME, the basis for the
// N-Triples and Turtle serializations. The triples mirror the RDF/XML and JSON-LD
// encoders exactly, so every serialization denotes the same graph.
func graphFromRecord(r *codex.Record) *rdf.Graph {
	return graphFromRecordAt(r, 0)
}

// graphFromRecordAt is graphFromRecord with an explicit stream index, so a
// collection writer can give each 001-less record a distinct base ("r<idx>")
// rather than colliding on "r0".
func graphFromRecordAt(r *codex.Record, idx int) *rdf.Graph {
	return graphFromBIBFRAME(FromRecord(r), resolveBase(r, idx))
}

func graphFromBIBFRAME(bib *BIBFRAME, base string) *rdf.Graph {
	s := newGraphSink(base)
	emitWork(s, &bib.Work, base, instanceIRIVal(base), nil)
	emitInstance(s, &bib.Instance, base, base)
	return s.gb.g
}

// WorkInstances is a Work together with the Instances that realize it — different
// editions, formats or translations of the same intellectual content. It
// serializes to one BIBFRAME grain in which the Work's triples appear once and
// each Instance is linked to it in both directions.
type WorkInstances struct {
	Work      Work
	Instances []Instance
}

// Graph assembles the Work at #<workBase>Work and each Instance i at
// #<instanceBases[i]>Instance, linked bf:hasInstance (Work -> each Instance) and
// bf:instanceOf (each Instance -> Work). The work and instance bases are
// independent, so a caller can mint opaque ids at both tiers; every base is
// sanitized like BIBFRAME.Graph. len(instanceBases) must equal len(wi.Instances).
//
// The whole grain is built with one blank-node counter, so blank labels are unique
// across the Work and all Instances and RDFC-1.0 canonicalization of the result is
// stable. Serialize it with rdf's NQuads, NTriples or Turtle encoders; a
// zero-Instance Work yields just the Work node. It panics if instanceBases does not
// match wi.Instances in length, which is a caller programming error.
func (wi *WorkInstances) Graph(workBase string, instanceBases []string) *rdf.Graph {
	if len(instanceBases) != len(wi.Instances) {
		panic("bibframe: WorkInstances.Graph: len(instanceBases) != len(Instances)")
	}
	wb := sanitizeID(workBase)
	s := newGraphSink(wb)
	emitWork(s, &wi.Work, wb, iriVal{}, instanceIRIs(instanceBases))
	for i := range wi.Instances {
		emitInstance(s, &wi.Instances[i], sanitizeID(instanceBases[i]), wb)
	}
	return s.gb.g
}

// Graph builds the RDF graph of a BIBFRAME Work/Instance pair, using base as the
// local-identifier stem for the node IRIs (#<base>Work and #<base>Instance). It
// is the entry point for callers that assemble a BIBFRAME directly from a
// non-MARC source and want the same graph shape FromRecord produces; serialize
// the result with rdf's NQuads, NTriples or Turtle encoders. FromRecord(r).Graph(
// base) is equivalent to the graph the record writers emit.
//
// base is sanitized to the characters valid in an IRI fragment (dropping spaces,
// '#', '/', etc.), so a caller-supplied identifier cannot produce an invalid node
// IRI (e.g. "#my idWork") or defeat the reader's controlNumber recovery.
func (bib *BIBFRAME) Graph(base string) *rdf.Graph {
	return graphFromBIBFRAME(bib, sanitizeID(base))
}

type graphBuilder struct {
	g      *rdf.Graph
	prefix string // blank-label namespace (the node base) so separately built graphs merge safely
	blanks int
}

// fresh mints a blank node namespaced by the builder's base, so two graphs built
// for different records (different bases) never collide on _:b1 when their triples
// are merged into one document.
func (gb *graphBuilder) fresh() rdf.Term {
	gb.blanks++
	return rdf.NewBlank(gb.prefix + "b" + strconv.Itoa(gb.blanks))
}

// typ adds an rdf:type triple.
func (gb *graphBuilder) typ(s rdf.Term, classIRI string) {
	gb.g.Add(s, rdf.NewIRI(pType), rdf.NewIRI(classIRI))
}

// lit adds a literal-valued triple when the value is non-empty.
func (gb *graphBuilder) lit(s rdf.Term, predIRI, value string) {
	if value != "" {
		gb.g.Add(s, rdf.NewIRI(predIRI), rdf.NewLiteral(value, "", ""))
	}
}
