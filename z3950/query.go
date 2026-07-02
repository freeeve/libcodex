package z3950

import (
	"fmt"
	"strings"
)

// Query is a Z39.50 Type-1 (RPN) query over the bib-1 attribute set. Build one
// with [Term] and combine with [And], [Or] and [AndNot]:
//
//	q := z3950.And(z3950.Term("author", "melville"), z3950.Term("title", "moby dick"))
//
// A term carries a use attribute plus a structure attribute chosen
// automatically: phrase for multi-word terms, word otherwise (strict servers
// reject multi-word terms without one). [Query.Phrase], [Query.Word],
// [Query.Truncated] and [Query.Exact] refine a term, and a trailing "*" means
// right truncation ("mob*" finds moby; escape a literal asterisk as "\*").
// Full bib-1 generality (proximity, other relations) is out of scope.
type Query struct {
	index string // access point for a leaf term
	term  string
	op    int // rpn operator for a branch: opAnd/opOr/opAndNot
	left  *Query
	right *Query

	structure int  // 0 auto, else the bib-1 structure attribute (1 phrase, 2 word)
	truncated bool // right truncation (5=1)
	exact     bool // relation equal + first position + complete field
}

const (
	opAnd    = 0
	opOr     = 1
	opAndNot = 2
)

// bib-1 structure attribute values.
const (
	structPhrase = 1
	structWord   = 2
)

// bib1Use maps an access-point name to its bib-1 use attribute (type 1).
var bib1Use = map[string]int64{
	"any":     1016,
	"title":   4,
	"author":  1003,
	"subject": 21,
	"isbn":    7,
	"issn":    8,
	"lccn":    9,
	"id":      12, // local record id
}

// Term is a single-term query against a named access point: one of "any",
// "title", "author", "subject", "isbn", "issn", "lccn" or "id".
func Term(index, term string) Query { return Query{index: index, term: term} }

// And matches records satisfying both queries.
func And(a, b Query) Query { return Query{op: opAnd, left: &a, right: &b} }

// Or matches records satisfying either query.
func Or(a, b Query) Query { return Query{op: opOr, left: &a, right: &b} }

// AndNot matches records satisfying a but not b.
func AndNot(a, b Query) Query { return Query{op: opAndNot, left: &a, right: &b} }

// Phrase forces structure=phrase, overriding the automatic choice.
func (q Query) Phrase() Query { q.structure = structPhrase; return q }

// Word forces structure=word, overriding the automatic choice.
func (q Query) Word() Query { q.structure = structWord; return q }

// Truncated searches the term as a right-truncated stem ("mob" finds moby).
func (q Query) Truncated() Query { q.truncated = true; return q }

// Exact matches the complete field exactly: relation equal, first-in-field
// position, complete-field completeness. Suits control numbers and uniform
// identifiers more than free text.
func (q Query) Exact() Query { q.exact = true; return q }

// rpn renders the query as a BER RPNStructure: a leaf is op [0] wrapping an
// AttributesPlusTerm [102]; a branch is rpnRpnOp [1] holding both operands and
// the Operator [46].
func (q Query) rpn() ([]byte, error) {
	if q.left != nil && q.right != nil {
		l, err := q.left.rpn()
		if err != nil {
			return nil, err
		}
		r, err := q.right.rpn()
		if err != nil {
			return nil, err
		}
		var body []byte
		body = append(body, l...)
		body = append(body, r...)
		// Operator [46] is an explicit tag around the operator CHOICE, whose
		// members are implicit NULLs.
		opElem := appendElem(nil, classContext, false, uint32(q.op), nil)
		body = appendElem(body, classContext, true, 46, opElem)
		return appendElem(nil, classContext, true, 1, body), nil
	}
	use, ok := bib1Use[q.index]
	if !ok {
		return nil, fmt.Errorf("z3950: unknown access point %q (have any, title, author, subject, isbn, issn, lccn, id)", q.index)
	}
	term, truncated := q.term, q.truncated
	switch {
	case strings.HasSuffix(term, `\*`):
		term = term[:len(term)-2] + "*"
	case strings.HasSuffix(term, "*"):
		term, truncated = term[:len(term)-1], true
	}
	if term == "" {
		return nil, fmt.Errorf("z3950: empty search term")
	}
	structure := int64(q.structure)
	if structure == 0 {
		structure = structWord
		if strings.ContainsAny(term, " \t") {
			structure = structPhrase
		}
	}

	// AttributeList [44]: AttributeElements in ascending attribute-type order
	// (use 1, relation 2, position 3, structure 4, truncation 5, completeness 6),
	// emitting only what the query asks for beyond use and structure.
	attrList := attributeElement(nil, 1, use)
	if q.exact {
		attrList = attributeElement(attrList, 2, 3) // relation: equal
		attrList = attributeElement(attrList, 3, 1) // position: first in field
	}
	attrList = attributeElement(attrList, 4, structure)
	if truncated {
		attrList = attributeElement(attrList, 5, 1) // truncation: right
	}
	if q.exact {
		attrList = attributeElement(attrList, 6, 3) // completeness: complete field
	}

	var apt []byte
	apt = appendElem(apt, classContext, true, 44, attrList)
	apt = appendString(apt, classContext, 45, term) // Term: general [45]

	operand := appendElem(nil, classContext, true, 102, apt) // AttributesPlusTerm
	return appendElem(nil, classContext, true, 0, operand), nil
}

// attributeElement appends one AttributeElement (a plain SEQUENCE carrying
// attributeType [120] and numeric attributeValue [121]).
func attributeElement(dst []byte, attrType, value int64) []byte {
	var attr []byte
	attr = appendInt(attr, classContext, 120, attrType)
	attr = appendInt(attr, classContext, 121, value)
	return appendElem(dst, classUniversal, true, tagSequence, attr)
}
