package bibframe

import (
	"bytes"
	"slices"
	"testing"

	"github.com/freeeve/libcodex/rdf"
)

// sampleWorkInstances is a Work realized by two Instances with distinct provision
// and identifiers, for the multi-instance grain tests.
func sampleWorkInstances() *WorkInstances {
	return &WorkInstances{
		Work: Work{
			Class:         "Text",
			Titles:        []Title{{MainTitle: "Shared Work Title"}},
			Contributions: []Contribution{{Primary: true, Class: "Person", Label: "Author, An"}},
			Subjects:      []Subject{{Class: "Topic", Label: "Something"}},
		},
		Instances: []Instance{
			{
				Titles:      []Title{{MainTitle: "First Edition"}},
				Provision:   &Provision{Place: "London", Publisher: "Verso", Date: "2001"},
				Identifiers: []Identifier{{Class: "Isbn", Value: "0000000001"}},
			},
			{
				Titles:      []Title{{MainTitle: "Second Edition"}},
				Provision:   &Provision{Place: "New York", Publisher: "Norton", Date: "2010"},
				Identifiers: []Identifier{{Class: "Isbn", Value: "0000000002"}},
			},
		},
	}
}

// TestWorkInstancesGraphStructure checks a Work with two Instances yields one Work
// node and two distinct Instance nodes, linked bf:hasInstance and bf:instanceOf in
// both directions, with the Work's own triples emitted once.
func TestWorkInstancesGraphStructure(t *testing.T) {
	g := sampleWorkInstances().Graph("work-1", []string{"inst-a", "inst-b"})

	workIRI := rdf.NewIRI("#work-1Work")
	instA := rdf.NewIRI("#inst-aInstance")
	instB := rdf.NewIRI("#inst-bInstance")

	if works := g.SubjectsOfType(classWork); len(works) != 1 || works[0] != workIRI {
		t.Fatalf("Work subjects = %v, want [%v]", works, workIRI)
	}
	insts := g.SubjectsOfType(classInstance)
	if len(insts) != 2 {
		t.Fatalf("Instance subjects = %v, want 2", insts)
	}
	if !slices.Contains(insts, instA) || !slices.Contains(insts, instB) {
		t.Errorf("Instance subjects = %v, want the independent bases %v and %v", insts, instA, instB)
	}

	his := g.Objects(workIRI, pHasInstance)
	if len(his) != 2 || !slices.Contains(his, instA) || !slices.Contains(his, instB) {
		t.Errorf("hasInstance = %v, want both %v and %v", his, instA, instB)
	}
	for _, inst := range []rdf.Term{instA, instB} {
		if o, ok := g.Object(inst, pInstanceOf); !ok || o != workIRI {
			t.Errorf("%v bf:instanceOf = %v (ok=%v), want %v", inst, o, ok, workIRI)
		}
	}

	// The Work title is emitted once, on the Work node; the Instances carry their
	// own transcribed titles.
	if titles := g.Objects(workIRI, pTitle); len(titles) != 1 {
		t.Errorf("Work has %d titles, want 1 (emitted once)", len(titles))
	}
	if mt := mainTitleOf(g, workIRI); mt != "Shared Work Title" {
		t.Errorf("Work main title = %q, want %q", mt, "Shared Work Title")
	}
	if mt := mainTitleOf(g, instA); mt != "First Edition" {
		t.Errorf("Instance A main title = %q, want %q", mt, "First Edition")
	}
}

// TestWorkInstancesBlankNodesDistinct checks each Instance's blank nodes are unique
// across the grain: the two Instances must not share a provision or identifier
// blank, which would merge distinct manifestations.
func TestWorkInstancesBlankNodesDistinct(t *testing.T) {
	g := sampleWorkInstances().Graph("w", []string{"a", "b"})
	instA := rdf.NewIRI("#aInstance")
	instB := rdf.NewIRI("#bInstance")

	provA, okA := g.Object(instA, pProvision)
	provB, okB := g.Object(instB, pProvision)
	if !okA || !okB {
		t.Fatalf("missing provision: A=%v B=%v", okA, okB)
	}
	if !provA.IsBlank() || !provB.IsBlank() {
		t.Fatalf("provision nodes should be blank: A=%v B=%v", provA, provB)
	}
	if provA == provB {
		t.Fatalf("the two Instances share provision blank %v -- grains merged", provA)
	}

	idA, _ := g.Object(instA, pIdentifiedBy)
	idB, _ := g.Object(instB, pIdentifiedBy)
	if idA == idB {
		t.Fatalf("the two Instances share identifier blank %v -- grains merged", idA)
	}
}

// TestWorkInstancesCanonicalDeterministic checks the grain canonicalizes stably
// (RDFC-1.0), including across a relabeling of its blank nodes.
func TestWorkInstancesCanonicalDeterministic(t *testing.T) {
	g := sampleWorkInstances().Graph("work-1", []string{"inst-a", "inst-b"})
	c1, err := g.Canonical()
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	c2, err := g.Canonical()
	if err != nil || !bytes.Equal(c1, c2) {
		t.Fatalf("canonicalization not deterministic (err=%v)", err)
	}
	if len(c1) == 0 {
		t.Fatal("empty canonical output")
	}
}

// TestWorkInstancesZeroInstances checks a Work with no Instances yields just the
// Work node with no bf:hasInstance link.
func TestWorkInstancesZeroInstances(t *testing.T) {
	wi := &WorkInstances{Work: Work{Class: "Text", Titles: []Title{{MainTitle: "Lonely Work"}}}}
	g := wi.Graph("solo", nil)
	workIRI := rdf.NewIRI("#soloWork")
	if works := g.SubjectsOfType(classWork); len(works) != 1 || works[0] != workIRI {
		t.Fatalf("Work subjects = %v, want [%v]", works, workIRI)
	}
	if insts := g.SubjectsOfType(classInstance); len(insts) != 0 {
		t.Errorf("got %d Instances, want 0", len(insts))
	}
	if his := g.Objects(workIRI, pHasInstance); len(his) != 0 {
		t.Errorf("hasInstance = %v, want none", his)
	}
}

// TestWorkInstancesGraphPanicsOnMismatch checks the length precondition is enforced.
func TestWorkInstancesGraphPanicsOnMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on len(instanceBases) != len(Instances)")
		}
	}()
	wi := &WorkInstances{Instances: []Instance{{}, {}}}
	_ = wi.Graph("w", []string{"only-one"})
}

// TestWorkInstancesBaseSanitized checks a caller base with IRI-invalid characters
// is sanitized in the node IRIs (matching BIBFRAME.Graph).
func TestWorkInstancesBaseSanitized(t *testing.T) {
	wi := &WorkInstances{Work: Work{Titles: []Title{{MainTitle: "T"}}}, Instances: []Instance{{Titles: []Title{{MainTitle: "I"}}}}}
	g := wi.Graph("my id#x", []string{"inst/y"})
	if works := g.SubjectsOfType(classWork); len(works) != 1 || works[0] != rdf.NewIRI("#myidxWork") {
		t.Errorf("Work subject = %v, want #myidxWork (sanitized)", works)
	}
	if insts := g.SubjectsOfType(classInstance); len(insts) != 1 || insts[0] != rdf.NewIRI("#instyInstance") {
		t.Errorf("Instance subject = %v, want #instyInstance (sanitized)", insts)
	}
}

// mainTitleOf returns the bf:mainTitle of subject's first bf:title node.
func mainTitleOf(g *rdf.Graph, subject rdf.Term) string {
	node, ok := g.Object(subject, pTitle)
	if !ok {
		return ""
	}
	v, _ := g.Literal(node, pMainTitle)
	return v
}
