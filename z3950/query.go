package z3950

import "fmt"

// Query is a Z39.50 Type-1 (RPN) query over the bib-1 attribute set. Build one
// with [Term] and combine with [And], [Or] and [AndNot]:
//
//	q := z3950.And(z3950.Term("author", "melville"), z3950.Term("title", "moby dick"))
//
// The builder covers the common access points; full bib-1 generality (relation,
// truncation, proximity) is out of scope.
type Query struct {
	index string // access point for a leaf term
	term  string
	op    int // rpn operator for a branch: opAnd/opOr/opAndNot
	left  *Query
	right *Query
}

const (
	opAnd    = 0
	opOr     = 1
	opAndNot = 2
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
	if q.term == "" {
		return nil, fmt.Errorf("z3950: empty search term")
	}
	// AttributeList [44]: one AttributeElement (a plain SEQUENCE) carrying
	// attributeType [120] = 1 (use) and numeric attributeValue [121].
	var attr []byte
	attr = appendInt(attr, classContext, 120, 1)
	attr = appendInt(attr, classContext, 121, use)
	attrElem := appendElem(nil, classUniversal, true, tagSequence, attr)
	attrList := appendElem(nil, classContext, true, 44, attrElem)

	var apt []byte
	apt = append(apt, attrList...)
	apt = appendString(apt, classContext, 45, q.term) // Term: general [45]

	operand := appendElem(nil, classContext, true, 102, apt) // AttributesPlusTerm
	return appendElem(nil, classContext, true, 0, operand), nil
}
