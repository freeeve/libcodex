package rdf

import (
	"bufio"
	"encoding/xml"
	"errors"
	"io"
	"iter"
	"slices"
	"strings"
)

// Format identifies a serialization the streaming Decoder reads.
type Format int

const (
	// NTriples is the line-based N-Triples format.
	NTriples Format = iota
	// NQuads is N-Quads; the optional graph label on each line is ignored, so each
	// statement still yields a triple.
	NQuads
	// RDFXML is RDF/XML; triples stream as each node element is parsed.
	RDFXML
	// Turtle streams '.'-terminated statements; @prefix/@base carry forward.
	// SPARQL-style PREFIX/BASE directives (no '.') are not supported when streaming.
	Turtle
)

// Decoder streams RDF statements from an io.Reader one triple at a time, in
// constant memory — for inputs far too large to materialize into a Graph
// (multi-gigabyte dumps such as the LC authority files). Each returned triple owns
// its strings, so it is safe to retain after the next call.
//
// JSON-LD is not streamable here (its document must be materialized); use
// ParseJSONLD for that.
type Decoder struct {
	nextQuad func() (Quad, error)
	stop     func() // signals the producer goroutine to stop early (nil for the line path)
	skipBad  bool   // skip malformed lines instead of failing (line formats only)
}

// SkipMalformed makes the decoder skip lines it cannot parse instead of failing
// on them, and returns the decoder so it can be chained onto [NewDecoder]. It
// affects only the line-based formats (N-Triples, N-Quads); RDF/XML and Turtle
// have always reported a syntax error.
//
// The default is to fail. A parser that skips what it cannot read turns a
// truncated dump into a smaller, well-formed graph, and the caller has no way to
// tell. Opt in only where the input is known to carry noise that is safe to drop
// -- and where a short read is not a lie you would ship.
func (d *Decoder) SkipMalformed(skip bool) *Decoder {
	d.skipBad = skip
	return d
}

// NewDecoder returns a streaming Decoder reading the given format from r.
func NewDecoder(r io.Reader, format Format) *Decoder {
	switch format {
	case RDFXML:
		return pipe(func(emit func(s, pr, o Term)) error {
			return (&xmlParser{dec: xml.NewDecoder(r), emit: emit}).run()
		})
	case Turtle:
		return pipe(func(emit func(s, pr, o Term)) error {
			return streamTurtle(r, emit)
		})
	default: // NTriples, NQuads
		br := bufio.NewReader(r)
		d := &Decoder{}
		lineNo := 0
		d.nextQuad = func() (Quad, error) {
			for {
				line, err := br.ReadString('\n')
				if len(line) > 0 {
					lineNo++
					switch q, kind := parseNQuadLine(line, nil); kind {
					case lineStatement:
						return q, nil
					case lineMalformed:
						if !d.skipBad {
							return Quad{}, &SyntaxError{Line: lineNo, Text: strings.TrimSpace(line)}
						}
					}
				}
				if err != nil {
					return Quad{}, err // io.EOF at a clean end
				}
			}
		}
		return d
	}
}

// Decode returns the next triple, or io.EOF when the input is exhausted. Any
// fourth (graph) term on an N-Quads line is dropped; use DecodeQuad to keep it.
// Blank and comment lines are skipped; a malformed one is a *SyntaxError naming
// the line, unless [Decoder.SkipMalformed] was set. io.EOF therefore means the
// input ended, not that it ended and some of it was unreadable.
func (d *Decoder) Decode() (Triple, error) {
	q, err := d.nextQuad()
	return q.Triple(), err
}

// DecodeQuad returns the next statement as a quad, preserving the graph term of
// an N-Quads line; the triple-only formats (N-Triples, RDF/XML, Turtle) yield a
// zero-value graph term (the default graph). It returns io.EOF at end of input.
func (d *Decoder) DecodeQuad() (Quad, error) { return d.nextQuad() }

// Close stops a streaming decoder early, releasing its producer goroutine. It is a
// no-op for the line-based formats and after the stream is exhausted.
func (d *Decoder) Close() error {
	if d.stop != nil {
		d.stop()
	}
	return nil
}

// All returns an iterator over the remaining triples, ending at the first error
// (io.EOF, the normal end, is not surfaced). Breaking out of the loop stops the
// producer:
//
//	for tr := range dec.All() { ... }
//
// Use Decode directly when a non-EOF read error must be observed.
func (d *Decoder) All() iter.Seq[Triple] {
	return func(yield func(Triple) bool) {
		if d.stop != nil {
			defer d.stop()
		}
		for {
			q, err := d.nextQuad()
			if err != nil {
				return
			}
			if !yield(q.Triple()) {
				return
			}
		}
	}
}

// AllQuads returns an iterator over the remaining statements as quads,
// preserving N-Quads graph terms. Like All, it ends at the first error and
// breaking out of the loop stops the producer:
//
//	for q := range dec.AllQuads() { ... }
func (d *Decoder) AllQuads() iter.Seq[Quad] {
	return func(yield func(Quad) bool) {
		if d.stop != nil {
			defer d.stop()
		}
		for {
			q, err := d.nextQuad()
			if err != nil {
				return
			}
			if !yield(q) {
				return
			}
		}
	}
}

// errStop unwinds a producer when the consumer stops early.
var errStop = errors.New("rdf: decode stopped")

// pipe runs producer in a goroutine, emitting each triple to an unbuffered channel
// that Decode drains; the emit blocks until the consumer reads (so memory stays
// bounded by the producer's per-statement state), and stop aborts it.
func pipe(producer func(emit func(s, pr, o Term)) error) *Decoder {
	ch := make(chan Triple)
	done := make(chan struct{})
	errc := make(chan error, 1)

	emit := func(s, pr, o Term) {
		select {
		case ch <- Triple{s, pr, o}:
		case <-done:
			panic(errStop)
		}
	}
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil && r != errStop {
				panic(r)
			}
		}()
		errc <- producer(emit)
	}()

	var termErr error
	finished := false
	return &Decoder{
		// The pipe formats (RDF/XML, Turtle) carry no graph term, so each triple
		// becomes a default-graph quad.
		nextQuad: func() (Quad, error) {
			if finished {
				return Quad{}, termErr
			}
			if tr, ok := <-ch; ok {
				return Quad{tr.S, tr.P, tr.O, Term{}}, nil
			}
			finished, termErr = true, io.EOF
			select {
			case err := <-errc:
				if err != nil {
					termErr = err
				}
			default:
			}
			return Quad{}, termErr
		},
		stop: func() {
			select {
			case <-done:
			default:
				close(done)
			}
		},
	}
}

// streamTurtle parses a Turtle stream statement by statement, emitting each
// statement's triples and carrying @prefix/@base state forward, in constant
// memory.
func streamTurtle(r io.Reader, emit func(s, pr, o Term)) error {
	p := &turtleParser{
		emit:     emit,
		prefixes: map[string]string{},
		iriCache: map[string]map[string]string{},
		// strs left nil: streaming copies each triple's strings (no arena)
	}
	sc := &turtleSplitter{br: bufio.NewReader(r)}
	for {
		stmt, err := sc.next()
		if stmt != "" {
			p.s, p.pos = stmt, 0
			if ok, done := p.stmt(); !ok && !done {
				return errTurtle(p)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// maxStatementBytes caps a single buffered Turtle statement. Without it, input
// that never yields a statement terminator (e.g. one enormous predicate-object
// list, or an unterminated string) would buffer without bound — defeating the
// constant-memory guarantee — while statementEnd re-scans the whole buffer on
// every read, which is quadratic in the statement length. No real statement is
// anywhere near this large.
const maxStatementBytes = 1 << 24 // 16 MiB

// errStatementTooLarge aborts a stream whose current statement exceeds
// maxStatementBytes without terminating.
var errStatementTooLarge = errors.New("rdf: Turtle statement too large")

// turtleSplitter yields one complete '.'-terminated Turtle statement at a time
// from a reader, buffering only the current statement plus a read chunk.
type turtleSplitter struct {
	br  *bufio.Reader
	buf []byte
}

func (s *turtleSplitter) next() (string, error) {
	for {
		if end := statementEnd(s.buf); end >= 0 {
			stmt := string(s.buf[:end])
			s.buf = s.buf[end:]
			return stmt, nil
		}
		if len(s.buf) > maxStatementBytes {
			return "", errStatementTooLarge
		}
		// Read directly into buf's spare capacity, growing it once when full, rather
		// than allocating a fresh 64 KiB chunk and copying it in on every read.
		const chunkSize = 64 * 1024
		if cap(s.buf)-len(s.buf) < chunkSize {
			s.buf = slices.Grow(s.buf, chunkSize)
		}
		n, err := s.br.Read(s.buf[len(s.buf):cap(s.buf)])
		s.buf = s.buf[:len(s.buf)+n]
		if err != nil {
			if err == io.EOF {
				if strings.TrimSpace(string(s.buf)) != "" {
					stmt := string(s.buf)
					s.buf = nil
					return stmt, nil
				}
				return "", io.EOF
			}
			return "", err
		}
	}
}

// statementEnd returns the index just past the terminating '.' of the first
// complete statement in b, or -1 if b holds no complete statement. It skips IRIs,
// strings and comments, and treats a '.' as a terminator only when followed by
// whitespace (so a decimal like 1.5 is not split).
func statementEnd(b []byte) int {
	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '#':
			for i < len(b) && b[i] != '\n' {
				i++
			}
		case c == '<':
			i++
			for i < len(b) && b[i] != '>' {
				if b[i] == '\\' {
					i++
				}
				i++
			}
			i++
		case c == '"' || c == '\'':
			i = skipQuoted(b, i)
		case c == '.':
			if i+1 >= len(b) {
				return -1 // ambiguous at buffer end; read more (EOF handled by caller)
			}
			// A statement terminator is a "." not inside a token, and it need not be
			// followed by whitespace: the next statement, comment, or directive can
			// begin immediately. Only followers that can never occur inside a
			// prefixed name or a number are treated as terminators here -- an IRI
			// ("<"), comment ("#"), directive ("@"), blank-node list ("[") or
			// collection ("(") subject. A "." followed by a letter, digit, or "_" is
			// left to the whole-document parser: it is ambiguous between a statement
			// boundary and a decimal or a prefixed local name such as "ex:a.b".
			switch n := b[i+1]; {
			case isWS(n), n == '#', n == '<', n == '@', n == '[', n == '(':
				return i + 1
			}
			i++
		default:
			i++
		}
	}
	return -1
}

// skipQuoted returns the index just past a Turtle string literal beginning at i,
// or len(b) if it is unterminated within b.
func skipQuoted(b []byte, i int) int {
	q := b[i]
	long := i+2 < len(b) && b[i+1] == q && b[i+2] == q
	if long {
		i += 3
	} else {
		i++
	}
	for i < len(b) {
		switch {
		case b[i] == '\\':
			i += 2
		case b[i] == q && !long:
			return i + 1
		case b[i] == q && long && i+2 < len(b) && b[i+1] == q && b[i+2] == q:
			return i + 3
		default:
			i++
		}
	}
	return len(b)
}
