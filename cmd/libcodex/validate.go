package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/freeeve/libcodex"
)

// runValidate reads each source and runs the structural check on every record,
// reporting the record's position and control number for each failure. It errors
// (non-zero exit) if any record is invalid or a stream fails to parse, so it is
// usable as a gate in scripts.
func runValidate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	inFmt := fs.String("i", "", "input format (auto-detect when empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srcs, err := sources(fs.Args())
	if err != nil {
		return err
	}
	total, bad := 0, 0
	for _, s := range srcs {
		n, b, err := validateSource(s, *inFmt, stdout)
		s.close()
		total += n
		bad += b
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "checked %d record(s), %d invalid\n", total, bad)
	if bad > 0 {
		return fmt.Errorf("%d record(s) failed validation", bad)
	}
	return nil
}

// validateSource checks one source, printing a line per invalid record. It
// returns the number of records seen, the number invalid, and any read error.
func validateSource(s *source, inFmt string, stdout io.Writer) (seen, bad int, err error) {
	r, _, err := s.records(inFmt)
	if err != nil {
		return 0, 0, err
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return seen, bad, nil
		}
		if err != nil {
			return seen, bad, fmt.Errorf("%s: record %d: %w", s.name, seen, err)
		}
		if verr := rec.Validate(); verr != nil {
			bad++
			fmt.Fprintf(stdout, "%s: record %d (001=%s): %v\n", s.name, seen, recControlNumber(rec), verr)
		}
		seen++
	}
}

// recControlNumber returns the record's 001 control number, or "?" when absent,
// for identifying a record in diagnostics.
func recControlNumber(rec *codex.Record) string {
	if cn := rec.ControlField("001"); cn != "" {
		return cn
	}
	return "?"
}
