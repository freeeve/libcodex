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
	return graphFromBIBFRAME(FromRecord(r), resolveBase(r, 0))
}

func graphFromBIBFRAME(bib *BIBFRAME, base string) *rdf.Graph {
	gb := &graphBuilder{g: &rdf.Graph{}}
	work := rdf.NewIRI(workURI(base))
	inst := rdf.NewIRI(instanceURI(base))
	gb.work(work, inst, bib)
	gb.instance(work, inst, bib)
	return gb.g
}

// Graph builds the RDF graph of a BIBFRAME Work/Instance pair, using base as the
// local-identifier stem for the node IRIs (#<base>Work and #<base>Instance). It
// is the entry point for callers that assemble a BIBFRAME directly from a
// non-MARC source and want the same graph shape FromRecord produces; serialize
// the result with rdf's NQuads, NTriples or Turtle encoders. FromRecord(r).Graph(
// base) is equivalent to the graph the record writers emit.
func (bib *BIBFRAME) Graph(base string) *rdf.Graph {
	return graphFromBIBFRAME(bib, base)
}

type graphBuilder struct {
	g      *rdf.Graph
	blanks int
}

func (gb *graphBuilder) fresh() rdf.Term {
	gb.blanks++
	return rdf.NewBlank("b" + strconv.Itoa(gb.blanks))
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

func (gb *graphBuilder) work(work, inst rdf.Term, bib *BIBFRAME) {
	gb.typ(work, classWork)
	if bib.Work.Class != "" {
		gb.typ(work, bfNS+bib.Work.Class)
	}
	for _, t := range bib.Work.Titles {
		gb.title(work, t)
	}
	for _, c := range bib.Work.Contributions {
		gb.contribution(work, c)
	}
	for _, s := range bib.Work.Subjects {
		gb.labeled(work, pSubject, bfNS+s.Class, s.Label)
	}
	for _, gf := range bib.Work.GenreForms {
		gb.labeled(work, pGenreForm, bfNS+"GenreForm", gf)
	}
	for _, code := range bib.Work.Languages {
		lang := rdf.NewIRI(langVocab + code)
		gb.g.Add(work, rdf.NewIRI(pLanguage), lang)
		gb.typ(lang, bfNS+"Language")
		gb.lit(lang, pLabel, code)
	}
	for _, c := range bib.Work.Classifications {
		node := gb.fresh()
		gb.g.Add(work, rdf.NewIRI(pClassif), node)
		gb.typ(node, bfNS+c.Class)
		gb.lit(node, pClassPortion, c.Value)
	}
	for _, s := range bib.Work.Summary {
		gb.labeled(work, pSummary, bfNS+"Summary", s)
	}
	gb.g.Add(work, rdf.NewIRI(pHasInstance), inst)
}

func (gb *graphBuilder) instance(work, inst rdf.Term, bib *BIBFRAME) {
	gb.typ(inst, classInstance)
	gb.g.Add(inst, rdf.NewIRI(pInstanceOf), work)
	for _, t := range bib.Instance.Titles {
		gb.title(inst, t)
	}
	gb.lit(inst, pRespStmt, bib.Instance.ResponsibilityStatement)
	gb.lit(inst, pEdition, bib.Instance.EditionStatement)
	if p := bib.Instance.Provision; p != nil {
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
	for _, e := range bib.Instance.Extent {
		gb.labeled(inst, pExtent, bfNS+"Extent", e)
	}
	for _, id := range bib.Instance.Identifiers {
		node := gb.fresh()
		gb.g.Add(inst, rdf.NewIRI(pIdentifiedBy), node)
		gb.typ(node, bfNS+id.Class)
		gb.lit(node, pValue, id.Value)
	}
	for _, u := range bib.Instance.ElectronicLocator {
		gb.g.Add(inst, rdf.NewIRI(pLocator), rdf.NewIRI(u))
	}
	gb.adminMetadata(inst, bib.Instance.Admin)
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
