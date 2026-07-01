package rdf

import (
	"sort"
	"strings"
)

// ---- Turtle serialization ----

// Turtle serializes the graph as Turtle, declaring the given namespace prefixes
// (prefix label -> namespace IRI) and compacting IRIs against them. Triples are
// grouped by subject in first-seen order, using `a` for rdf:type.
func (g *Graph) Turtle(prefixes map[string]string) []byte {
	return append(TurtleHeader(prefixes), g.TurtleBody(prefixes)...)
}

// TurtleHeader returns the @prefix declaration block for the given prefixes,
// terminated by a blank line. Collection writers emit it once.
func TurtleHeader(prefixes map[string]string) []byte {
	if len(prefixes) == 0 {
		return nil
	}
	labels := make([]string, 0, len(prefixes))
	for k := range prefixes {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	var b []byte
	for _, k := range labels {
		b = append(b, "@prefix "...)
		b = append(b, k...)
		b = append(b, ": <"...)
		b = appendEscapedIRI(b, prefixes[k])
		b = append(b, "> .\n"...)
	}
	return append(b, '\n')
}

// TurtleBody serializes the triples grouped by subject, without a prefix header.
func (g *Graph) TurtleBody(prefixes map[string]string) []byte {
	var e Encoder
	return e.AppendTurtle(nil, g, prefixes)
}

// AppendTurtle appends g's triples to b as a Turtle body (no @prefix header),
// grouped by subject, in a fresh blank-node scope. It groups without a
// per-subject map: subjects are ranked by first appearance, then a stable sort
// of triple indices by that rank makes each subject's triples contiguous in
// document order — turning thousands of small allocations into a handful.
func (e *Encoder) AppendTurtle(b []byte, g *Graph, prefixes map[string]string) []byte {
	n := len(g.Triples)
	if n == 0 {
		return b
	}
	e.bn.newScope()
	rank := make(map[Term]int, n)
	rankOf := make([]int, n)
	next := 0
	for i, t := range g.Triples {
		r, ok := rank[t.S]
		if !ok {
			r, next = next, next+1
			rank[t.S] = r
		}
		rankOf[i] = r
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return rankOf[idx[a]] < rankOf[idx[b]] })

	var done []bool // reused across subjects
	for i := 0; i < n; {
		j := i + 1
		for j < n && g.Triples[idx[j]].S == g.Triples[idx[i]].S {
			j++
		}
		b = appendTurtleSubject(b, g.Triples, idx[i:j], prefixes, &done, &e.bn)
		i = j
	}
	return b
}

// appendTurtleSubject writes one subject's predicate-object list, grouping objects
// by predicate with a linear scan over the subject's (contiguous) triples and a
// caller-reused scratch buffer.
func appendTurtleSubject(b []byte, triples []Triple, idxs []int, prefixes map[string]string, scratch *[]bool, bn *blankNamer) []byte {
	b = appendTurtleTerm(b, triples[idxs[0]].S, prefixes, false, bn)

	done := (*scratch)[:0]
	for range idxs {
		done = append(done, false)
	}
	*scratch = done

	first := true
	for a := range idxs {
		if done[a] {
			continue
		}
		ta := triples[idxs[a]]
		if first {
			b = append(b, ' ')
			first = false
		} else {
			b = append(b, " ;\n    "...)
		}
		b = appendTurtleTerm(b, ta.P, prefixes, true, bn)
		b = append(b, ' ')
		b = appendTurtleTerm(b, ta.O, prefixes, false, bn)
		done[a] = true
		for c := a + 1; c < len(idxs); c++ {
			if !done[c] && triples[idxs[c]].P == ta.P {
				b = append(b, ", "...)
				b = appendTurtleTerm(b, triples[idxs[c]].O, prefixes, false, bn)
				done[c] = true
			}
		}
	}
	return append(b, " .\n"...)
}

// appendTurtleTerm writes a term in Turtle syntax. In predicate position rdf:type
// is written as `a`.
func appendTurtleTerm(b []byte, t Term, prefixes map[string]string, predicate bool, bn *blankNamer) []byte {
	switch t.Kind {
	case IRI:
		if predicate && t.Value == TypeIRI {
			return append(b, 'a')
		}
		if nb, ok := appendCompactIRI(b, t.Value, prefixes); ok {
			return nb
		}
		b = append(b, '<')
		b = appendEscapedIRI(b, t.Value)
		return append(b, '>')
	case Blank:
		b = append(b, '_', ':')
		return append(b, bn.name(t.Value)...)
	default:
		b = append(b, '"')
		b = appendEscapedLiteral(b, t.Value)
		b = append(b, '"')
		if t.Lang != "" {
			b = append(b, '@')
			return append(b, t.Lang...)
		}
		if t.Datatype != "" && t.Datatype != XSDString {
			b = append(b, "^^"...)
			if nb, ok := appendCompactIRI(b, t.Datatype, prefixes); ok {
				return nb
			}
			b = append(b, '<')
			b = appendEscapedIRI(b, t.Datatype)
			return append(b, '>')
		}
		return b
	}
}

// appendCompactIRI appends iri as a prefixed name against the longest matching
// namespace when the remaining local part is a valid bare local name, reporting
// whether it compacted. It writes straight to b, allocating no intermediate
// string.
func appendCompactIRI(b []byte, iri string, prefixes map[string]string) ([]byte, bool) {
	bestLabel, bestNS := "", ""
	for label, ns := range prefixes {
		if len(ns) > len(bestNS) && strings.HasPrefix(iri, ns) {
			if local := iri[len(ns):]; validLocal(local) {
				bestLabel, bestNS = label, ns
			}
		}
	}
	if bestNS == "" {
		return b, false
	}
	b = append(b, bestLabel...)
	b = append(b, ':')
	b = append(b, iri[len(bestNS):]...)
	return b, true
}

// validLocal reports whether s is a non-empty local name that can be written bare
// (matching what the reader accepts), so a round-trip is lossless.
func validLocal(s string) bool {
	if s == "" || s[len(s)-1] == '.' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isNameChar(s[i]) {
			return false
		}
	}
	return true
}
