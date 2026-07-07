package main

import (
	"flag"
	"io"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/mrk"
)

// runCat reads the input sources and writes each record in a human-readable form:
// the mrk line format by default, or MARC-in-JSON with --json. --tags keeps only
// the listed fields (comma-separated, e.g. "084,650"); --limit caps the record
// count across all inputs.
func runCat(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	inFmt := fs.String("i", "", "input format (auto-detect when empty)")
	tags := fs.String("t", "", "comma-separated tags to keep (e.g. 084,650)")
	limit := fs.Int("n", 0, "maximum records to print (0 = all)")
	asJSON := fs.Bool("json", false, "emit MARC-in-JSON instead of the mrk format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	keep := tagSet(*tags)
	w := catWriter(*asJSON, stdout)

	srcs, err := sources(fs.Args())
	if err != nil {
		return err
	}
	n := 0
	for _, s := range srcs {
		if err := catSource(s, *inFmt, keep, limit, &n, w); err != nil {
			s.close()
			codex.Close(w)
			return err
		}
		s.close()
	}
	return codex.Close(w)
}

// catSource streams one source through the writer, applying the tag filter and
// honoring the shared record limit via n.
func catSource(s *source, inFmt string, keep map[string]bool, limit, n *int, w codex.RecordWriter) error {
	r, _, err := s.records(inFmt)
	if err != nil {
		return err
	}
	for {
		if *limit > 0 && *n >= *limit {
			return nil
		}
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if keep != nil {
			rec = filterTags(rec, keep)
		}
		if err := w.Write(rec); err != nil {
			return err
		}
		*n++
	}
}

// catWriter selects the readable writer: MARC-in-JSON or the mrk line format.
func catWriter(asJSON bool, stdout io.Writer) codex.RecordWriter {
	if asJSON {
		return marcjson.NewWriter(stdout)
	}
	return mrk.NewWriter(stdout)
}

// tagSet parses a comma-separated tag list into a set, or returns nil when the
// list is empty (meaning "keep every field").
func tagSet(list string) map[string]bool {
	list = strings.TrimSpace(list)
	if list == "" {
		return nil
	}
	set := map[string]bool{}
	for t := range strings.SplitSeq(list, ",") {
		if t = strings.TrimSpace(t); t != "" {
			set[t] = true
		}
	}
	return set
}

// filterTags returns a copy of rec keeping its leader and only the fields whose
// tag is in keep. The original record is left untouched.
func filterTags(rec *codex.Record, keep map[string]bool) *codex.Record {
	out := codex.NewRecord().SetLeader(rec.Leader())
	for _, f := range rec.Fields() {
		if keep[f.Tag] {
			out.AddField(f)
		}
	}
	return out
}
