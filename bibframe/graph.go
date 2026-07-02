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
	gb := &graphBuilder{g: &rdf.Graph{}, prefix: base}
	work := rdf.NewIRI(workURI(base))
	inst := rdf.NewIRI(instanceURI(base))
	gb.work(work, &bib.Work)
	gb.g.Add(work, rdf.NewIRI(pHasInstance), inst)
	gb.instance(work, inst, &bib.Instance)
	return gb.g
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
	gb := &graphBuilder{g: &rdf.Graph{}, prefix: wb}
	work := rdf.NewIRI(workURI(wb))
	gb.work(work, &wi.Work)
	for i := range wi.Instances {
		inst := rdf.NewIRI(instanceURI(sanitizeID(instanceBases[i])))
		gb.g.Add(work, rdf.NewIRI(pHasInstance), inst)
		gb.instance(work, inst, &wi.Instances[i])
	}
	return gb.g
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

// labeled adds parent -pred-> [a class; rdfs:label label], the shape used for
// subjects, genre forms, summaries, extents, places and publishers.
func (gb *graphBuilder) labeled(parent rdf.Term, predIRI, classIRI, label string) {
	node := gb.fresh()
	gb.g.Add(parent, rdf.NewIRI(predIRI), node)
	gb.typ(node, classIRI)
	gb.lit(node, pLabel, label)
}

// work emits the Work's triples. The bf:hasInstance links are added by the caller
// (one per Instance), so a Work with several Instances emits its own triples once.
func (gb *graphBuilder) work(work rdf.Term, w *Work) {
	gb.typ(work, classWork)
	if w.Class != "" {
		gb.typ(work, bfNS+w.Class)
	}
	for _, t := range w.Titles {
		gb.title(work, t)
	}
	for _, c := range w.Contributions {
		gb.contribution(work, c)
	}
	for _, s := range w.Subjects {
		gb.labeled(work, pSubject, bfNS+s.Class, s.Label)
	}
	for _, gf := range w.GenreForms {
		gb.labeled(work, pGenreForm, bfNS+"GenreForm", gf)
	}
	for _, code := range w.Languages {
		lang := rdf.NewIRI(langVocab + code)
		gb.g.Add(work, rdf.NewIRI(pLanguage), lang)
		gb.typ(lang, bfNS+"Language")
		gb.lit(lang, pLabel, code)
	}
	for _, c := range w.Classifications {
		node := gb.fresh()
		gb.g.Add(work, rdf.NewIRI(pClassif), node)
		gb.typ(node, bfNS+c.Class)
		gb.lit(node, pClassPortion, c.Value)
		if c.Source != "" {
			gb.labeled(node, pSource, classSource, c.Source)
		}
	}
	for _, s := range w.Summary {
		gb.labeled(work, pSummary, bfNS+"Summary", s)
	}
}

func (gb *graphBuilder) instance(work, inst rdf.Term, in *Instance) {
	gb.typ(inst, classInstance)
	gb.g.Add(inst, rdf.NewIRI(pInstanceOf), work)
	for _, t := range in.Titles {
		gb.title(inst, t)
	}
	gb.lit(inst, pRespStmt, in.ResponsibilityStatement)
	gb.lit(inst, pEdition, in.EditionStatement)
	if p := in.Provision; p != nil {
		node := gb.fresh()
		gb.g.Add(inst, rdf.NewIRI(pProvision), node)
		gb.typ(node, bfNS+"Publication")
		if p.Place != "" {
			gb.labeled(node, pPlace, bfNS+"Place", p.Place)
		}
		if p.Publisher != "" {
			gb.labeled(node, pAgent, bfNS+"Agent", p.Publisher)
		}
		gb.lit(node, pDate, p.Date)
	}
	for _, e := range in.Extent {
		gb.labeled(inst, pExtent, bfNS+"Extent", e)
	}
	for _, id := range in.Identifiers {
		node := gb.fresh()
		gb.g.Add(inst, rdf.NewIRI(pIdentifiedBy), node)
		gb.typ(node, bfNS+id.Class)
		gb.lit(node, pValue, id.Value)
		if id.Source != "" {
			gb.labeled(node, pSource, classSource, id.Source)
		}
	}
	for _, u := range in.ElectronicLocator {
		gb.g.Add(inst, rdf.NewIRI(pLocator), rdf.NewIRI(u))
	}
	gb.adminMetadata(inst, in.Admin)
}

// adminMetadata renders the bf:AdminMetadata provenance node on the instance: a
// generation-process marker plus the control number, change date and cataloging
// conventions the record carries.
func (gb *graphBuilder) adminMetadata(inst rdf.Term, am *AdminMetadata) {
	if am == nil {
		return
	}
	node := gb.fresh()
	gb.g.Add(inst, rdf.NewIRI(pAdminMetadata), node)
	gb.typ(node, classAdminMetadata)
	gb.labeled(node, pGenerationProcess, classGenerationProcess, generatorLabel)
	gb.lit(node, pChangeDate, am.ChangeDate)
	gb.lit(node, pDescriptionConventions, am.DescriptionConventions)
	if am.ControlNumber != "" {
		id := gb.fresh()
		gb.g.Add(node, rdf.NewIRI(pIdentifiedBy), id)
		gb.typ(id, classLocal)
		gb.lit(id, pValue, am.ControlNumber)
	}
}

// title adds parent -bf:title-> [a bf:Title; bf:mainTitle …; …].
func (gb *graphBuilder) title(parent rdf.Term, t Title) {
	node := gb.fresh()
	gb.g.Add(parent, rdf.NewIRI(pTitle), node)
	gb.typ(node, bfNS+"Title")
	gb.lit(node, pMainTitle, t.MainTitle)
	gb.lit(node, pSubtitle, t.Subtitle)
	gb.lit(node, pPartNumber, t.PartNumber)
	gb.lit(node, pPartName, t.PartName)
}

func (gb *graphBuilder) contribution(work rdf.Term, c Contribution) {
	node := gb.fresh()
	gb.g.Add(work, rdf.NewIRI(pContribution), node)
	if c.Primary {
		gb.typ(node, primaryContribution)
	} else {
		gb.typ(node, bfNS+"Contribution")
	}
	agent := gb.fresh()
	gb.g.Add(node, rdf.NewIRI(pAgent), agent)
	gb.typ(agent, bfNS+c.Class)
	gb.lit(agent, pLabel, c.Label)
	if c.Role != "" {
		gb.labeled(node, pRole, bfNS+"Role", c.Role)
	}
}
