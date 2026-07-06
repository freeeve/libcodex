package main

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/freeeve/libcodex"
)

// stats accumulates a field-and-leader profile across every record read.
type stats struct {
	records  int
	tags     map[string]int // field tag -> occurrences
	recType  map[byte]int   // leader/06 record type -> records
	bibLevel map[byte]int   // leader/07 bibliographic level -> records
	unicode  int            // records declaring UTF-8 (leader/09 = 'a')
}

// runStats reads every source and prints a profile: record count, per-tag
// frequency, the record-type and bibliographic-level breakdowns, and the
// character-encoding split.
func runStats(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	inFmt := fs.String("i", "", "input format (auto-detect when empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st := &stats{tags: map[string]int{}, recType: map[byte]int{}, bibLevel: map[byte]int{}}
	srcs, err := sources(fs.Args())
	if err != nil {
		return err
	}
	for _, s := range srcs {
		err := st.addSource(s, *inFmt)
		s.close()
		if err != nil {
			return err
		}
	}
	st.report(stdout)
	return nil
}

// addSource folds every record of one source into the profile.
func (st *stats) addSource(s *source, inFmt string) error {
	r, _, err := s.records(inFmt)
	if err != nil {
		return err
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		st.add(rec)
	}
}

// add folds one record into the profile.
func (st *stats) add(rec *codex.Record) {
	st.records++
	ldr := rec.Leader()
	st.recType[ldr.RecordType()]++
	st.bibLevel[ldr.BibLevel()]++
	if ldr.IsUnicode() {
		st.unicode++
	}
	for _, f := range rec.Fields() {
		st.tags[f.Tag]++
	}
}

// report writes the accumulated profile to w.
func (st *stats) report(w io.Writer) {
	fmt.Fprintf(w, "records: %d\n", st.records)
	fmt.Fprintf(w, "encoding: %d unicode, %d marc-8\n", st.unicode, st.records-st.unicode)

	fmt.Fprintln(w, "\nrecord type (leader/06):")
	printByteCounts(w, st.recType)
	fmt.Fprintln(w, "\nbibliographic level (leader/07):")
	printByteCounts(w, st.bibLevel)

	fmt.Fprintln(w, "\nfields:")
	tags := make([]string, 0, len(st.tags))
	for t := range st.tags {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	for _, t := range tags {
		fmt.Fprintf(w, "  %s  %d\n", t, st.tags[t])
	}
}

// printByteCounts writes a code->count map ordered by code, rendering a space as
// "#" so a blank leader byte is visible.
func printByteCounts(w io.Writer, m map[byte]int) {
	codes := make([]int, 0, len(m))
	for c := range m {
		codes = append(codes, int(c))
	}
	sort.Ints(codes)
	for _, c := range codes {
		disp := byte(c)
		if disp == ' ' {
			disp = '#'
		}
		fmt.Fprintf(w, "  %c  %d\n", disp, m[byte(c)])
	}
}
