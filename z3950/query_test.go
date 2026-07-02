package z3950

import (
	"fmt"
	"testing"
)

// rpnLeaf decodes an encoded leaf RPNStructure into its attribute (type, value)
// pairs in emission order and the term.
func rpnLeaf(t *testing.T, b []byte) (attrs [][2]int64, term string) {
	t.Helper()
	op, _, err := berParse(b) // op [0]
	if err != nil {
		t.Fatal(err)
	}
	inner, err := op.children() // AttributesPlusTerm [102]
	if err != nil || len(inner) != 1 || inner[0].tag != 102 {
		t.Fatalf("operand shape: %v (%+v)", err, inner)
	}
	apt, err := inner[0].children()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range apt {
		switch f.tag {
		case 44: // AttributeList
			elems, err := f.children()
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range elems {
				members, err := e.children()
				if err != nil {
					t.Fatal(err)
				}
				var pair [2]int64
				for _, m := range members {
					v, err := m.intVal()
					if err != nil {
						t.Fatal(err)
					}
					switch m.tag {
					case 120:
						pair[0] = v
					case 121:
						pair[1] = v
					}
				}
				attrs = append(attrs, pair)
			}
		case 45:
			term = f.stringVal()
		}
	}
	return attrs, term
}

// TestQueryAttributes pins the exact AttributeList each builder form emits.
func TestQueryAttributes(t *testing.T) {
	cases := []struct {
		name  string
		q     Query
		attrs [][2]int64
		term  string
	}{
		{"multi-word auto phrase", Term("title", "moby dick"),
			[][2]int64{{1, 4}, {4, 1}}, "moby dick"},
		{"single word auto word", Term("title", "whale"),
			[][2]int64{{1, 4}, {4, 2}}, "whale"},
		{"phrase override", Term("title", "whale").Phrase(),
			[][2]int64{{1, 4}, {4, 1}}, "whale"},
		{"word override", Term("title", "moby dick").Word(),
			[][2]int64{{1, 4}, {4, 2}}, "moby dick"},
		{"trailing star truncates", Term("title", "mob*"),
			[][2]int64{{1, 4}, {4, 2}, {5, 1}}, "mob"},
		{"escaped star literal", Term("title", `mob\*`),
			[][2]int64{{1, 4}, {4, 2}}, "mob*"},
		{"explicit truncation", Term("author", "melvil").Truncated(),
			[][2]int64{{1, 1003}, {4, 2}, {5, 1}}, "melvil"},
		{"exact", Term("isbn", "9780142437247").Exact(),
			[][2]int64{{1, 7}, {2, 3}, {3, 1}, {4, 2}, {6, 3}}, "9780142437247"},
		{"exact phrase", Term("title", "moby dick").Exact(),
			[][2]int64{{1, 4}, {2, 3}, {3, 1}, {4, 1}, {6, 3}}, "moby dick"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := c.q.rpn()
			if err != nil {
				t.Fatal(err)
			}
			attrs, term := rpnLeaf(t, b)
			if fmt.Sprint(attrs) != fmt.Sprint(c.attrs) || term != c.term {
				t.Errorf("attrs %v term %q, want %v %q", attrs, term, c.attrs, c.term)
			}
		})
	}
}

// TestQueryBooleanShape checks a boolean branch still wraps two operands and an
// operator, with the refined leaves intact.
func TestQueryBooleanShape(t *testing.T) {
	b, err := And(Term("author", "melville"), Term("title", "moby dick")).rpn()
	if err != nil {
		t.Fatal(err)
	}
	branch, _, err := berParse(b)
	if err != nil || branch.tag != 1 {
		t.Fatalf("branch tag %d (%v)", branch.tag, err)
	}
	kids, err := branch.children()
	if err != nil || len(kids) != 3 {
		t.Fatalf("branch children = %d (%v)", len(kids), err)
	}
	if kids[0].tag != 0 || kids[1].tag != 0 || kids[2].tag != 46 {
		t.Errorf("branch shape = %d,%d,%d, want 0,0,46", kids[0].tag, kids[1].tag, kids[2].tag)
	}
	attrs, term := rpnLeaf(t, appendElem(nil, kids[1].class, true, kids[1].tag, kids[1].content))
	if term != "moby dick" || fmt.Sprint(attrs) != fmt.Sprint([][2]int64{{1, 4}, {4, 1}}) {
		t.Errorf("right leaf attrs %v term %q", attrs, term)
	}
	op, err := kids[2].children()
	if err != nil || len(op) != 1 || op[0].tag != opAnd {
		t.Errorf("operator = %+v (%v)", op, err)
	}
}

// TestQueryStarOnly checks a bare "*" (empty stem after truncation) errors.
func TestQueryStarOnly(t *testing.T) {
	if _, err := Term("title", "*").rpn(); err == nil {
		t.Error("bare * should error (empty stem)")
	}
}
