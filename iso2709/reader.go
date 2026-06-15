package iso2709

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/freeeve/libcodex"
)

// Reader reads ISO 2709 records one at a time from an underlying stream. It does
// not buffer the whole stream: each record is read using the length declared in
// its leader (bytes [0:5]), falling back to the record terminator when that
// length is absent or invalid. This handles records larger than any default
// buffer.
type Reader struct {
	br    *bufio.Reader
	buf   []byte // reused across reads; safe because Decode copies values out
	lossy bool
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader that reads records from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}

// Read returns the next record from the stream, or io.EOF when the stream is
// exhausted. A non-EOF error indicates that the current record was malformed;
// the leader length is still used to advance past it, so the caller may
// continue calling Read for subsequent records.
func (rd *Reader) Read() (*codex.Record, error) {
	if err := rd.skipSeparators(); err != nil {
		return nil, err
	}

	if cap(rd.buf) < leaderLen {
		rd.buf = make([]byte, leaderLen)
	}
	leader := rd.buf[:leaderLen]
	if _, err := io.ReadFull(rd.br, leader); err != nil {
		if err == io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("iso2709: truncated leader: %w", err)
		}
		return nil, err
	}

	record, err := rd.readBody()
	if err != nil {
		return nil, err
	}
	rec, lossy, err := Decode(record)
	rd.lossy = lossy
	return rec, err
}

// Lossy reports whether the record returned by the most recent successful Read
// decoded out-of-scope MARC-8 best-effort (see Decode), meaning it may contain
// mojibake. It is false until the first such Read.
func (rd *Reader) Lossy() bool { return rd.lossy }

// All returns an iterator over the remaining records in the stream, for use as
//
//	for rec, err := range r.All() { ... }
//
// It is shorthand for codex.All(rd) and stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}

// skipSeparators advances past any inter-record newlines or stray record
// terminators so the next read starts at a leader. It returns io.EOF at a clean
// end of stream.
func (rd *Reader) skipSeparators() error {
	for {
		b, err := rd.br.ReadByte()
		if err != nil {
			return err
		}
		if b == '\n' || b == '\r' || b == RecordTerminator {
			continue
		}
		return rd.br.UnreadByte()
	}
}

// readBody reads the remainder of a record into the reused buffer rd.buf, whose
// first leaderLen bytes already hold the leader. It prefers the declared length
// and falls back to reading up to the record terminator. The returned slice
// aliases rd.buf and is only valid until the next Read; Decode copies the values
// out, so streaming callers are unaffected.
func (rd *Reader) readBody() ([]byte, error) {
	if n, ok := atoiBytes(rd.buf[0:5]); ok && n >= leaderLen {
		if cap(rd.buf) < n {
			grown := make([]byte, n)
			copy(grown, rd.buf[:leaderLen])
			rd.buf = grown
		} else {
			rd.buf = rd.buf[:n]
		}
		if _, err := io.ReadFull(rd.br, rd.buf[leaderLen:n]); err != nil {
			return nil, fmt.Errorf("iso2709: truncated record body: %w", err)
		}
		return rd.buf[:n], nil
	}

	// Declared length absent or invalid: scan to the record terminator. This
	// uncommon path allocates a fresh slice rather than reusing rd.buf.
	rest, err := rd.br.ReadBytes(RecordTerminator)
	if err != nil && err != io.EOF {
		return nil, err
	}
	out := make([]byte, leaderLen+len(rest))
	copy(out, rd.buf[:leaderLen])
	copy(out[leaderLen:], rest)
	return out, nil
}

// ReadFile reads every record from the named file. On the first malformed record
// it returns the records parsed so far together with the error.
func ReadFile(path string) ([]*codex.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := NewReader(f)
	var out []*codex.Record
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, rec)
	}
}
