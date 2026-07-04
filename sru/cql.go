package sru

import "strings"

// cqlEscaper escapes the characters CQL requires escaping inside a quoted term.
var cqlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// Quote wraps a CQL search term in double quotes, escaping embedded backslashes
// and quotes, so a user-supplied term is safe to place in a CQL query:
//
//	client.NewReader(ctx, "dc.title = "+sru.Quote(userInput))
//
// Raw CQL strings pass through the client verbatim; [Term] and the boolean
// combinators build them safely.
func Quote(term string) string {
	return `"` + cqlEscaper.Replace(term) + `"`
}

// Query is a typed CQL query, mirroring the z3950 package's builder so one
// query shape drives either transport:
//
//	q := sru.And(sru.Term("author", "melville"), sru.Term("title", "moby dick"))
//	rd := client.NewReader(ctx, q.String())
//
// Access points map to the context set most deployments index -- Dublin Core
// for descriptive fields (dc.title, dc.author, ...) and the Bath profile for
// identifiers (bath.isbn, bath.issn, bath.lccn); an index name containing a dot
// (e.g. "bath.possessingInstitution") passes through unchanged for servers using
// another set. This is a query writer only -- it does not parse CQL.
type Query struct {
	index string
	term  string
	op    string // "and", "or", "not" for a branch
	left  *Query
	right *Query
}

// cqlIndex maps a builder access-point name to its CQL index. "any" renders as
// a bare term (the server-choice index). Identifier access points use the Bath
// profile (bath.isbn/issn/lccn) rather than Dublin Core, which defines no
// identifier indexes -- servers such as LOC's reject the dc.isbn/dc.issn forms
// with an "unsupported index" diagnostic.
var cqlIndex = map[string]string{
	"any":     "",
	"title":   "dc.title",
	"author":  "dc.author",
	"subject": "dc.subject",
	"isbn":    "bath.isbn",
	"issn":    "bath.issn",
	"lccn":    "bath.lccn",
	"id":      "rec.id",
}

// Term is a single-term query against a named access point ("any", "title",
// "author", "subject", "isbn", "issn", "lccn", "id"); any other name (e.g.
// "bath.possessingInstitution") is used as the CQL index verbatim.
func Term(index, term string) Query { return Query{index: index, term: term} }

// And matches records satisfying both queries.
func And(a, b Query) Query { return Query{op: "and", left: &a, right: &b} }

// Or matches records satisfying either query.
func Or(a, b Query) Query { return Query{op: "or", left: &a, right: &b} }

// Not matches records satisfying a but not b.
func Not(a, b Query) Query { return Query{op: "not", left: &a, right: &b} }

// String renders the query as CQL with every term quoted, parenthesizing
// boolean branches so nesting is unambiguous.
func (q Query) String() string {
	if q.left != nil && q.right != nil {
		return "(" + q.left.String() + ") " + q.op + " (" + q.right.String() + ")"
	}
	index := q.index
	if mapped, ok := cqlIndex[index]; ok {
		index = mapped
	}
	// An unmapped name passes through verbatim (dotted names deliberately, and a
	// typo'd plain name surfaces as the server's "unsupported index" diagnostic
	// rather than silently broadening to a server-choice search).
	if index == "" {
		return Quote(q.term)
	}
	return index + " = " + Quote(q.term)
}
