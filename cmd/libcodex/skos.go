package main

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/skos"
)

// runSkos reads a SKOS concept scheme (RDF, serialization autodetected) and, by
// default, prints a readable concept view with a summary header; with -o it
// crosswalks each concept to a MARC authority record and writes it in that
// output format.
func runSkos(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("skos", flag.ContinueOnError)
	outFmt := fs.String("o", "", "convert to MARC authority records in this format (marc, marcxml, marcjson, mrk); omit for a concept view")
	limit := fs.Int("n", 0, "limit to the first N concepts (0 = all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	concepts, err := readConcepts(fs.Args())
	if err != nil {
		return err
	}
	if *limit > 0 && *limit < len(concepts) {
		concepts = concepts[:*limit]
	}

	if *outFmt == "" {
		return skosView(concepts, stdout)
	}
	w, err := newWriter(*outFmt, stdout)
	if err != nil {
		return err
	}
	for _, c := range concepts {
		if err := w.Write(c.Record()); err != nil {
			codex.Close(w)
			return err
		}
	}
	return codex.Close(w)
}

// readConcepts parses every input path (or stdin when none) into one concept
// list. Each RDF document is parsed whole, so the scheme fits in memory.
func readConcepts(paths []string) ([]skos.Concept, error) {
	srcs, err := sources(paths)
	if err != nil {
		return nil, err
	}
	var all []skos.Concept
	for _, s := range srcs {
		data, err := io.ReadAll(s.br)
		s.close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		cs, err := skos.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		all = append(all, cs...)
	}
	return all, nil
}

// skosView prints the summary header and one block per concept.
func skosView(concepts []skos.Concept, w io.Writer) error {
	bases, schemes, langs := tally(concepts)
	fmt.Fprintf(w, "# %d concepts\n", len(concepts))
	fmt.Fprintf(w, "# IRI base:    %s\n", formatTally(bases))
	if len(schemes) > 0 {
		mixed := ""
		if len(schemes) > 1 {
			mixed = "  [mixed]"
		}
		fmt.Fprintf(w, "# inScheme:    %s%s\n", formatTally(schemes), mixed)
	}
	if len(langs) > 0 {
		fmt.Fprintf(w, "# label langs: %s\n", formatTally(langs))
	}
	fmt.Fprintln(w)
	for _, c := range concepts {
		fmt.Fprintf(w, "%s  %s%s\n", c.ID, c.PrefLabel(), langSuffix(c))
		if c.URI != "" {
			fmt.Fprintf(w, "  uri: %s\n", c.URI)
		}
		if alts := labelList(c.Alt); alts != "" {
			fmt.Fprintf(w, "  alt: %s\n", alts)
		}
		writeRefs(w, "broader", c.Broader)
		writeRefs(w, "narrower", c.Narrower)
		writeRefs(w, "related", c.Related)
	}
	return nil
}

// tally counts, over all concepts, the IRI bases (concept IRI minus its final
// segment), the inScheme targets, and the languages seen on preferred labels.
func tally(concepts []skos.Concept) (bases, schemes, langs map[string]int) {
	bases, schemes, langs = map[string]int{}, map[string]int{}, map[string]int{}
	for _, c := range concepts {
		bases[iriBase(c.URI)]++
		if c.Scheme != "" {
			schemes[c.Scheme]++
		}
		for _, l := range c.Pref {
			if l.Lang != "" {
				langs[l.Lang]++
			}
		}
	}
	return bases, schemes, langs
}

// iriBase returns an IRI up to and including its final '/', the version/namespace
// stem that a version mismatch shows up in.
func iriBase(iri string) string {
	if i := strings.LastIndex(iri, "/"); i >= 0 {
		return iri[:i+1]
	}
	return iri
}

// formatTally renders a count map as "value (n)" entries, most frequent first.
func formatTally(m map[string]int) string {
	type kv struct {
		k string
		n int
	}
	pairs := make([]kv, 0, len(m))
	for k, n := range m {
		pairs = append(pairs, kv{k, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s (%d)", p.k, p.n)
	}
	return strings.Join(parts, ", ")
}

// langSuffix renders the primary preferred label's language as " [lang]", or "".
func langSuffix(c skos.Concept) string {
	for _, l := range c.Pref {
		if isEnglishLang(l.Lang) {
			return " [" + l.Lang + "]"
		}
	}
	if len(c.Pref) > 0 && c.Pref[0].Lang != "" {
		return " [" + c.Pref[0].Lang + "]"
	}
	return ""
}

// isEnglishLang reports whether a language tag names English.
func isEnglishLang(lang string) bool { return lang == "en" || strings.HasPrefix(lang, "en-") }

// labelList renders alternate labels as "text [lang], …".
func labelList(labels []skos.Label) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Lang != "" {
			parts = append(parts, l.Text+" ["+l.Lang+"]")
		} else {
			parts = append(parts, l.Text)
		}
	}
	return strings.Join(parts, ", ")
}

// writeRefs prints a labeled reference line ("broader: Label (id), …") when refs
// is non-empty.
func writeRefs(w io.Writer, kind string, refs []skos.Ref) {
	if len(refs) == 0 {
		return
	}
	parts := make([]string, len(refs))
	for i, r := range refs {
		label := r.Label
		if label == "" {
			label = r.ID
		}
		parts[i] = fmt.Sprintf("%s (%s)", label, r.ID)
	}
	fmt.Fprintf(w, "  %s: %s\n", kind, strings.Join(parts, ", "))
}
