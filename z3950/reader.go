package z3950

import (
	"context"
	"io"
	"iter"

	codex "github.com/freeeve/libcodex"
)

// Reader streams a search's records as *codex.Record, dialing on first use and
// fetching Present pages on demand. It implements codex.RecordReader, so a
// Z39.50 search is a source for codex.Convert, mirroring the sru package's
// Reader. Records whose syntax has no codex decoder (e.g. SUTRS) are skipped;
// use [Conn.Present] directly to inspect them.
type Reader struct {
	c     *Client
	ctx   context.Context
	query Query
	conn  *Conn
	total int
	next  int // 1-based position of the next record to Present
	buf   []Record
	i     int
	err   error
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader over the result set for the query. The connection
// is dialed lazily on the first Read and closed automatically at the end of the
// result set or on error; call [Reader.Close] to abandon a stream early. The
// context governs the dial and every request.
func (c *Client) NewReader(ctx context.Context, q Query) *Reader {
	return &Reader{c: c, ctx: ctx, query: q, next: 1}
}

// Read returns the next decodable record, fetching further Present pages as
// needed, and io.EOF once the result set is exhausted. Errors (including
// io.EOF) are sticky.
func (rd *Reader) Read() (*codex.Record, error) {
	if rd.err != nil {
		return nil, rd.err
	}
	for {
		for rd.i < len(rd.buf) {
			rec := rd.buf[rd.i]
			rd.i++
			if rec.Diag != nil {
				continue // surrogate diagnostic: this one record was undeliverable
			}
			dec, err := rec.Decode()
			if err != nil {
				if rec.Syntax != "marc21" && rec.Syntax != "unimarc" && rec.Syntax != "xml" {
					continue // no decoder for this syntax; skip like sru's Reader
				}
				return nil, rd.fail(err)
			}
			return dec, nil
		}
		if err := rd.fetch(); err != nil {
			return nil, rd.fail(err)
		}
	}
}

// fail records a sticky error and closes the connection.
func (rd *Reader) fail(err error) error {
	rd.err = err
	rd.Close()
	return err
}

// fetch presents the next page, connecting and searching first if needed. It
// returns io.EOF when the result set is exhausted.
func (rd *Reader) fetch() error {
	if rd.conn == nil {
		conn, err := rd.c.Connect(rd.ctx)
		if err != nil {
			return err
		}
		res, err := conn.Search(rd.ctx, rd.query)
		if err != nil {
			conn.Close()
			return err
		}
		rd.conn = conn
		rd.total = res.Count
	}
	if rd.next > rd.total {
		return io.EOF
	}
	count := min(rd.c.pageSize(), rd.total-rd.next+1)
	recs, err := rd.conn.Present(rd.ctx, rd.next, count)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return io.EOF
	}
	rd.buf = recs
	rd.i = 0
	rd.next += len(recs)
	return nil
}

// Close releases the connection; safe to call at any time. Read reports io.EOF
// afterward unless another error was already sticky.
func (rd *Reader) Close() error {
	var err error
	if rd.conn != nil {
		err = rd.conn.Close()
		rd.conn = nil
	}
	if rd.err == nil {
		rd.err = io.EOF
	}
	return err
}

// All returns an iterator over the remaining records, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}
