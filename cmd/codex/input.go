package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/freeeve/libcodex"
)

// source is one input stream (a named file or stdin) buffered so its format can
// be sniffed before the records are read.
type source struct {
	name string // display name; "<stdin>" for the standard input
	rc   io.Closer
	br   *bufio.Reader
}

// openSource opens a file by path, or standard input when path is "" or "-".
func openSource(path string) (*source, error) {
	if path == "" || path == "-" {
		return &source{name: "<stdin>", br: bufio.NewReader(os.Stdin)}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &source{name: path, rc: f, br: bufio.NewReader(f)}, nil
}

// close releases the underlying file, if any.
func (s *source) close() {
	if s.rc != nil {
		s.rc.Close()
	}
}

// records returns a reader over the source. When format is "" the serialization
// is auto-detected from the leading bytes; an unrecognizable stream is an error
// so the caller never silently reads zero records.
func (s *source) records(format string) (codex.RecordReader, string, error) {
	if format == "" {
		format = sniff(s.br)
		if format == "" {
			return nil, "", fmt.Errorf("%s: could not detect format; pass -i to set it", s.name)
		}
	}
	r, err := newReader(format, s.br)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", s.name, err)
	}
	return r, format, nil
}

// sources expands the positional file arguments into open sources, defaulting to
// a single stdin source when none are given. The caller must close each.
func sources(paths []string) ([]*source, error) {
	if len(paths) == 0 {
		s, err := openSource("-")
		if err != nil {
			return nil, err
		}
		return []*source{s}, nil
	}
	out := make([]*source, 0, len(paths))
	for _, p := range paths {
		s, err := openSource(p)
		if err != nil {
			for _, o := range out {
				o.close()
			}
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
