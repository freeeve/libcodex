package bibframe

// This file is the single source of truth for the BIBFRAME Work/Instance node
// shape. One traversal (emitWork/emitInstance and the emit* node helpers) walks a
// record's BIBFRAME, calling a sink; three sinks (in shape_render.go) turn those
// calls into an rdf.Graph, RDF/XML or JSON-LD. The traversal owns element/key
// order and which properties are single vs repeated; each sink owns only
// formatting. Adding a node property is a one-line change here that every
// serialization picks up. Class and predicate names are qnames from vocab.go.

// iriVal is an IRI expressed as up to three parts whose concatenation is the IRI.
// The XML and JSON sinks append the parts directly, so a node IRI or reference
// costs no string concatenation there; the graph sink joins them, which allocates
// because the string becomes a graph term -- exactly where the old builder
// allocated. The zero iriVal denotes a blank node.
type iriVal struct{ a, b, c string }

func (v iriVal) blank() bool { return v.a == "" && v.b == "" && v.c == "" }

func (v iriVal) join() string {
	if v.b == "" && v.c == "" {
		return v.a
	}
	return v.a + v.b + v.c
}

func workIRIVal(base string) iriVal     { return iriVal{"#", base, "Work"} }
func instanceIRIVal(base string) iriVal { return iriVal{"#", base, "Instance"} }
func langIRIVal(code string) iriVal     { return iriVal{langVocab, code, ""} }

// sink receives the shape as a stream of calls. beginChild/endChild bracket a
// single child node; beginList/endList bracket a repeated one (JSON renders it as
// an array); a bare beginNode/endNode is a root node. iri is the zero iriVal for a
// blank node. extra on beginNode is one additional rdf:type (only the Work's genre
// subclass uses it), or the zero qname.
type sink interface {
	beginNode(class qname, iri iriVal, extra qname)
	endNode()
	lit(pred qname, text string)
	ref(pred qname, iri iriVal)
	refList(pred qname, iris []string)
	beginChild(pred qname)
	endChild()
	beginList(pred qname)
	endList()
}

// emitWork drives the Work node. singleInst, when non-empty, is the sole Instance
// IRI of a single Work/Instance pair, rendered as one bf:hasInstance reference;
// otherwise instList holds the Instance IRIs of a multi-instance grain, rendered as
// a list.
func emitWork(s sink, w *Work, base string, singleInst iriVal, instList []string) {
	s.beginNode(qcWork, workIRIVal(base), workSubclass(w))
	emitWorkBody(s, w)
	switch {
	case !singleInst.blank():
		s.ref(qpHasInstance, singleInst)
	case len(instList) > 0:
		s.refList(qpHasInstance, instList)
	}
	s.endNode()
}

// workSubclass is the Work's genre subclass as a bf: qname, or the zero qname.
func workSubclass(w *Work) qname {
	if w.Class == "" {
		return qname{}
	}
	return bfName(w.Class)
}

func emitWorkBody(s sink, w *Work) {
	if len(w.Titles) > 0 {
		s.beginList(qpTitle)
		for _, t := range w.Titles {
			emitTitle(s, t)
		}
		s.endList()
	}
	if len(w.Contributions) > 0 {
		s.beginList(qpContribution)
		for _, c := range w.Contributions {
			emitContribution(s, c)
		}
		s.endList()
	}
	if len(w.Subjects) > 0 {
		s.beginList(qpSubject)
		for _, sub := range w.Subjects {
			emitLabeled(s, bfName(sub.Class), sub.Label)
		}
		s.endList()
	}
	if len(w.GenreForms) > 0 {
		s.beginList(qpGenreForm)
		for _, gf := range w.GenreForms {
			emitLabeled(s, qcGenreForm, gf)
		}
		s.endList()
	}
	if len(w.Languages) > 0 {
		s.beginList(qpLanguage)
		for _, code := range w.Languages {
			emitLanguage(s, code)
		}
		s.endList()
	}
	if len(w.Classifications) > 0 {
		s.beginList(qpClassification)
		for _, c := range w.Classifications {
			emitClassification(s, c)
		}
		s.endList()
	}
	if len(w.Summary) > 0 {
		s.beginList(qpSummary)
		for _, sm := range w.Summary {
			emitLabeled(s, qcSummary, sm)
		}
		s.endList()
	}
}

// emitInstance drives an Instance node under instBase, linked bf:instanceOf back to
// #<workBase>Work.
func emitInstance(s sink, in *Instance, instBase, workBase string) {
	s.beginNode(qcInstance, instanceIRIVal(instBase), qname{})
	s.ref(qpInstanceOf, workIRIVal(workBase))
	if len(in.Titles) > 0 {
		s.beginList(qpTitle)
		for _, t := range in.Titles {
			emitTitle(s, t)
		}
		s.endList()
	}
	if in.ResponsibilityStatement != "" {
		s.lit(qpResponsibilityStmt, in.ResponsibilityStatement)
	}
	if in.EditionStatement != "" {
		s.lit(qpEditionStatement, in.EditionStatement)
	}
	if p := in.Provision; p != nil {
		emitProvision(s, p)
	}
	if len(in.Extent) > 0 {
		s.beginList(qpExtent)
		for _, e := range in.Extent {
			emitLabeled(s, qcExtent, e)
		}
		s.endList()
	}
	if in.Media != "" {
		s.beginChild(qpMedia)
		emitLabeled(s, qcMedia, in.Media)
		s.endChild()
	}
	if in.Carrier != "" {
		s.beginChild(qpCarrier)
		emitLabeled(s, qcCarrier, in.Carrier)
		s.endChild()
	}
	if len(in.Identifiers) > 0 {
		s.beginList(qpIdentifiedBy)
		for _, id := range in.Identifiers {
			emitIdentifier(s, id)
		}
		s.endList()
	}
	if len(in.ElectronicLocator) > 0 {
		s.refList(qpElectronicLocator, in.ElectronicLocator)
	}
	if in.Admin != nil {
		emitAdmin(s, in.Admin)
	}
	s.endNode()
}

// emitTitle emits a bf:Title. mainTitle is always present (rendered even when
// empty, matching the graph builder's skip-if-empty and the encoders' unconditional
// emit); the rest are optional.
func emitTitle(s sink, t Title) {
	s.beginNode(qcTitle, iriVal{}, qname{})
	s.lit(qpMainTitle, t.MainTitle)
	if t.Subtitle != "" {
		s.lit(qpSubtitle, t.Subtitle)
	}
	if t.PartNumber != "" {
		s.lit(qpPartNumber, t.PartNumber)
	}
	if t.PartName != "" {
		s.lit(qpPartName, t.PartName)
	}
	s.endNode()
}

// emitLabeled emits the ubiquitous "[a class; rdfs:label label]" node used for
// subjects, genre forms, summaries, extents, media, carriers, places, agents and
// sources.
func emitLabeled(s sink, class qname, label string) {
	s.beginNode(class, iriVal{}, qname{})
	s.lit(qpLabel, label)
	s.endNode()
}

// emitContribution wraps the agent (and optional role) in bflc:PrimaryContribution
// for the primary contributor, bf:Contribution otherwise.
func emitContribution(s sink, c Contribution) {
	class := qcContribution
	if c.Primary {
		class = qcPrimaryContribution
	}
	s.beginNode(class, iriVal{}, qname{})
	s.beginChild(qpAgent)
	emitLabeled(s, bfName(c.Class), c.Label)
	s.endChild()
	if c.Role != "" {
		s.beginChild(qpRole)
		emitLabeled(s, qcRole, c.Role)
		s.endChild()
	}
	s.endNode()
}

// emitLanguage emits a bf:Language IRI node in the LoC languages vocabulary,
// labeled with its three-letter code.
func emitLanguage(s sink, code string) {
	s.beginNode(qcLanguage, langIRIVal(code), qname{})
	s.lit(qpLabel, code)
	s.endNode()
}

func emitClassification(s sink, c Classification) {
	s.beginNode(bfName(c.Class), iriVal{}, qname{})
	s.lit(qpClassificationPortion, c.Value)
	if c.Source != "" {
		s.beginChild(qpSource)
		emitLabeled(s, qcSource, c.Source)
		s.endChild()
	}
	s.endNode()
}

func emitIdentifier(s sink, id Identifier) {
	s.beginNode(bfName(id.Class), iriVal{}, qname{})
	s.lit(qpValue, id.Value)
	if id.Source != "" {
		s.beginChild(qpSource)
		emitLabeled(s, qcSource, id.Source)
		s.endChild()
	}
	s.endNode()
}

func emitProvision(s sink, p *Provision) {
	s.beginChild(qpProvisionActivity)
	s.beginNode(qcPublication, iriVal{}, qname{})
	if p.Place != "" {
		s.beginChild(qpPlace)
		emitLabeled(s, qcPlace, p.Place)
		s.endChild()
	}
	if p.Publisher != "" {
		s.beginChild(qpAgent)
		emitLabeled(s, qcAgent, p.Publisher)
		s.endChild()
	}
	if p.Date != "" {
		s.lit(qpDate, p.Date)
	}
	s.endNode()
	s.endChild()
}

// emitAdmin emits the bf:AdminMetadata provenance node: the generation-process
// marker plus the control number, change date and cataloging conventions.
func emitAdmin(s sink, am *AdminMetadata) {
	s.beginChild(qpAdminMetadata)
	s.beginNode(qcAdminMetadata, iriVal{}, qname{})
	s.beginChild(qpGenerationProcess)
	emitLabeled(s, qcGenerationProcess, generatorLabel)
	s.endChild()
	if am.ChangeDate != "" {
		s.lit(qpChangeDate, am.ChangeDate)
	}
	if am.DescriptionConventions != "" {
		s.lit(qpDescriptionConventions, am.DescriptionConventions)
	}
	if am.ControlNumber != "" {
		s.beginChild(qpIdentifiedBy)
		s.beginNode(qcLocal, iriVal{}, qname{})
		s.lit(qpValue, am.ControlNumber)
		s.endNode()
		s.endChild()
	}
	s.endNode()
	s.endChild()
}

// instanceIRIs maps sanitized instance bases to their node IRIs, for the
// multi-instance bf:hasInstance list.
func instanceIRIs(bases []string) []string {
	if len(bases) == 0 {
		return nil
	}
	out := make([]string, len(bases))
	for i, b := range bases {
		out[i] = instanceURI(sanitizeID(b))
	}
	return out
}
