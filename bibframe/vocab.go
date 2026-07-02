package bibframe

// This file is the prefixed-name vocabulary the shape traversal (shape.go) speaks.
// A qname carries the prefix parts (ns, local), the folded prefixed form (pfx) and
// the full IRI (iri) for compile-time constants. The RDF/XML and JSON-LD sinks
// append pfx in one copy; the graph sink uses iri. Names built from record data
// leave pfx and iri empty, so the sinks fall back to appending ns:local and the
// graph sink expands ns+local on demand -- the same allocation profile the three
// hand-written emitters had.

const (
	nsBF   = "bf"
	nsBFLC = "bflc"
	nsRDF  = "rdf"
	nsRDFS = "rdfs"
)

// qname is a prefixed name. pfx ("bf:Work") and iri (the full IRI) are set for
// constants -- both constant expressions, folded at compile time -- and empty for
// data-derived names.
type qname struct{ ns, local, pfx, iri string }

// bfName builds a bf: qname from a data-derived local name (a contribution,
// subject, classification or identifier class), leaving pfx/iri to the sinks.
func bfName(local string) qname { return qname{ns: nsBF, local: local} }

// fullIRI returns the qname's IRI, expanding a data-derived name on demand (the
// same concatenation the old graph builder did) and returning the folded constant
// otherwise.
func (q qname) fullIRI() string {
	if q.iri != "" {
		return q.iri
	}
	return nsIRI(q.ns) + q.local
}

func nsIRI(ns string) string {
	switch ns {
	case nsBF:
		return bfNS
	case nsBFLC:
		return bflcNS
	case nsRDF:
		return rdfNS
	case nsRDFS:
		return rdfsNS
	}
	return ""
}

// Class qnames. The iri fields reuse the full-IRI constants declared in reader.go
// where they exist, so the two stay in lock-step.
var (
	qcWork                   = qname{nsBF, "Work", "bf:Work", classWork}
	qcInstance               = qname{nsBF, "Instance", "bf:Instance", classInstance}
	qcTitle                  = qname{nsBF, "Title", "bf:Title", bfNS + "Title"}
	qcVariantTitle           = qname{nsBF, "VariantTitle", "bf:VariantTitle", classVariantTitle}
	qcParallelTitle          = qname{nsBF, "ParallelTitle", "bf:ParallelTitle", classParallelTitle}
	qcPublication            = qname{nsBF, "Publication", "bf:Publication", bfNS + "Publication"}
	qcProduction             = qname{nsBF, "Production", "bf:Production", bfNS + "Production"}
	qcDistribution           = qname{nsBF, "Distribution", "bf:Distribution", bfNS + "Distribution"}
	qcManufacture            = qname{nsBF, "Manufacture", "bf:Manufacture", bfNS + "Manufacture"}
	qcLanguage               = qname{nsBF, "Language", "bf:Language", bfNS + "Language"}
	qcLocal                  = qname{nsBF, "Local", "bf:Local", classLocal}
	qcAdminMetadata          = qname{nsBF, "AdminMetadata", "bf:AdminMetadata", classAdminMetadata}
	qcGenerationProcess      = qname{nsBF, "GenerationProcess", "bf:GenerationProcess", classGenerationProcess}
	qcSource                 = qname{nsBF, "Source", "bf:Source", classSource}
	qcStatus                 = qname{nsBF, "Status", "bf:Status", classStatus}
	qcPlace                  = qname{nsBF, "Place", "bf:Place", bfNS + "Place"}
	qcRole                   = qname{nsBF, "Role", "bf:Role", bfNS + "Role"}
	qcAgent                  = qname{nsBF, "Agent", "bf:Agent", bfNS + "Agent"}
	qcDescriptionConventions = qname{nsBF, "DescriptionConventions", "bf:DescriptionConventions", bfNS + "DescriptionConventions"}
	qcExtent                 = qname{nsBF, "Extent", "bf:Extent", bfNS + "Extent"}
	qcContent                = qname{nsBF, "Content", "bf:Content", bfNS + "Content"}
	qcMedia                  = qname{nsBF, "Media", "bf:Media", bfNS + "Media"}
	qcCarrier                = qname{nsBF, "Carrier", "bf:Carrier", bfNS + "Carrier"}
	qcGenreForm              = qname{nsBF, "GenreForm", "bf:GenreForm", bfNS + "GenreForm"}
	qcSummary                = qname{nsBF, "Summary", "bf:Summary", bfNS + "Summary"}
	qcContribution           = qname{nsBF, "Contribution", "bf:Contribution", bfNS + "Contribution"}
	qcPrimaryContribution    = qname{nsBFLC, "PrimaryContribution", "bflc:PrimaryContribution", primaryContribution}
)

// Predicate qnames.
var (
	qpType                   = qname{nsRDF, "type", "rdf:type", pType}
	qpLabel                  = qname{nsRDFS, "label", "rdfs:label", pLabel}
	qpValue                  = qname{nsRDF, "value", "rdf:value", pValue}
	qpHasInstance            = qname{nsBF, "hasInstance", "bf:hasInstance", pHasInstance}
	qpInstanceOf             = qname{nsBF, "instanceOf", "bf:instanceOf", pInstanceOf}
	qpTitle                  = qname{nsBF, "title", "bf:title", pTitle}
	qpMainTitle              = qname{nsBF, "mainTitle", "bf:mainTitle", pMainTitle}
	qpSubtitle               = qname{nsBF, "subtitle", "bf:subtitle", pSubtitle}
	qpPartNumber             = qname{nsBF, "partNumber", "bf:partNumber", pPartNumber}
	qpPartName               = qname{nsBF, "partName", "bf:partName", pPartName}
	qpNonSortNum             = qname{nsBFLC, "nonSortNum", "bflc:nonSortNum", pNonSortNum}
	qpVariantType            = qname{nsBF, "variantType", "bf:variantType", pVariantType}
	qpContribution           = qname{nsBF, "contribution", "bf:contribution", pContribution}
	qpRelatedTo              = qname{nsBF, "relatedTo", "bf:relatedTo", pRelatedTo}
	qpAgent                  = qname{nsBF, "agent", "bf:agent", pAgent}
	qpRole                   = qname{nsBF, "role", "bf:role", pRole}
	qpSubject                = qname{nsBF, "subject", "bf:subject", pSubject}
	qpGenreForm              = qname{nsBF, "genreForm", "bf:genreForm", pGenreForm}
	qpLanguage               = qname{nsBF, "language", "bf:language", pLanguage}
	qpCode                   = qname{nsBF, "code", "bf:code", pCode}
	qpPart                   = qname{nsBF, "part", "bf:part", pPart}
	qpClassification         = qname{nsBF, "classification", "bf:classification", pClassif}
	qpClassificationPortion  = qname{nsBF, "classificationPortion", "bf:classificationPortion", pClassPortion}
	qpItemPortion            = qname{nsBF, "itemPortion", "bf:itemPortion", pItemPortion}
	qpClassEdition           = qname{nsBF, "edition", "bf:edition", pClassEdition}
	qpSummary                = qname{nsBF, "summary", "bf:summary", pSummary}
	qpResponsibilityStmt     = qname{nsBF, "responsibilityStatement", "bf:responsibilityStatement", pRespStmt}
	qpEditionStatement       = qname{nsBF, "editionStatement", "bf:editionStatement", pEdition}
	qpProvisionActivity      = qname{nsBF, "provisionActivity", "bf:provisionActivity", pProvision}
	qpCopyrightDate          = qname{nsBF, "copyrightDate", "bf:copyrightDate", pCopyright}
	qpSimplePlace            = qname{nsBFLC, "simplePlace", "bflc:simplePlace", pSimplePlace}
	qpSimpleAgent            = qname{nsBFLC, "simpleAgent", "bflc:simpleAgent", pSimpleAgent}
	qpSimpleDate             = qname{nsBFLC, "simpleDate", "bflc:simpleDate", pSimpleDate}
	qpPlace                  = qname{nsBF, "place", "bf:place", pPlace}
	qpDate                   = qname{nsBF, "date", "bf:date", pDate}
	qpExtent                 = qname{nsBF, "extent", "bf:extent", pExtent}
	qpDimensions             = qname{nsBF, "dimensions", "bf:dimensions", pDimensions}
	qpContent                = qname{nsBF, "content", "bf:content", pContent}
	qpMedia                  = qname{nsBF, "media", "bf:media", pMedia}
	qpCarrier                = qname{nsBF, "carrier", "bf:carrier", pCarrier}
	qpIssuance               = qname{nsBF, "issuance", "bf:issuance", pIssuance}
	qpIdentifiedBy           = qname{nsBF, "identifiedBy", "bf:identifiedBy", pIdentifiedBy}
	qpElectronicLocator      = qname{nsBF, "electronicLocator", "bf:electronicLocator", pLocator}
	qpAdminMetadata          = qname{nsBF, "adminMetadata", "bf:adminMetadata", pAdminMetadata}
	qpGenerationProcess      = qname{nsBF, "generationProcess", "bf:generationProcess", pGenerationProcess}
	qpChangeDate             = qname{nsBF, "changeDate", "bf:changeDate", pChangeDate}
	qpAssigner               = qname{nsBF, "assigner", "bf:assigner", pAssigner}
	qpDescriptionConventions = qname{nsBF, "descriptionConventions", "bf:descriptionConventions", pDescriptionConventions}
	qpSource                 = qname{nsBF, "source", "bf:source", pSource}
	qpQualifier              = qname{nsBF, "qualifier", "bf:qualifier", pQualifier}
	qpStatus                 = qname{nsBF, "status", "bf:status", pStatus}
)
