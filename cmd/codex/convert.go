package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/freeeve/libcodex"
)

// runConvert transcodes every record from the input sources into the -o output
// format, written to stdout. The input format is taken from -i or auto-detected
// per source. A wrapper-buffering output (marcxml collection, marcjson array,
// bibframe rdf:RDF) is finalized once after all sources are drained.
func runConvert(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	inFmt := fs.String("i", "", "input format (auto-detect when empty)")
	outFmt := fs.String("o", "", "output format (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outFmt == "" {
		return fmt.Errorf("-o output format is required (known: %s)", formatNames(writers))
	}

	w, err := newWriter(*outFmt, stdout)
	if err != nil {
		return err
	}

	srcs, err := sources(fs.Args())
	if err != nil {
		return err
	}
	for _, s := range srcs {
		r, _, err := s.records(*inFmt)
		if err != nil {
			s.close()
			codex.Close(w)
			return err
		}
		if err := codex.Convert(r, w); err != nil {
			s.close()
			codex.Close(w)
			return fmt.Errorf("%s: %w", s.name, err)
		}
		s.close()
	}
	return codex.Close(w)
}
