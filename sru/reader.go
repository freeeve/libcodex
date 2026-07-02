package sru

import (
	"context"
	"io"
	"iter"

	codex "github.com/freeeve/libcodex"
)

// Reader streams the records of a search result set as *codex.Record, paging
// through the SRU result set on demand. It implements codex.RecordReader, so an
// SRU search is a source for codex.Convert. Only MARCXML records decode to
// codex.Record; records in other schemas are skipped (inspect them with
// [Client.SearchRetrieve] instead).
type Reader struct {
	c     *Client
	ctx   context.Context
	req   Request
	buf   []Record
	i     int
	next  int  // startRecord of the next page to fetch (1-based)
	begun bool // whether the first page has been fetched
	done  bool // whether the result set is exhausted
	err   error
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader over the result set for query, using the client's
// default schema and page size. The context governs every underlying HTTP
// request; cancel it to stop an in-progress stream.
func (c *Client) NewReader(ctx context.Context, query string) *Reader {
	return &Reader{c: c, ctx: ctx, req: Request{Query: query}, next: 1}
}

// Read returns the next MARCXML record as a *codex.Record, fetching further pages
// as needed, and io.EOF once the result set is exhausted. Records in a non-MARCXML
// schema are skipped. A transport, parse or diagnostic error is sticky: once
// returned, every later call returns it too.
func (rd *Reader) Read() (*codex.Record, error) {
	if rd.err != nil {
		return nil, rd.err
	}
	for {
		for rd.i < len(rd.buf) {
			rec := rd.buf[rd.i]
			rd.i++
			if rec.Schema != "marcxml" {
				continue // no decoder for other schemas; inspect via SearchRetrieve
			}
			dec, err := rec.Decode()
			if err != nil {
				rd.err = err
				return nil, err
			}
			return dec, nil
		}
		if err := rd.fetch(); err != nil {
			rd.err = err
			return nil, err
		}
	}
}

// fetch loads the next page into the buffer, or returns io.EOF when the result
// set is exhausted.
func (rd *Reader) fetch() error {
	if rd.done {
		return io.EOF
	}
	req := rd.req
	req.StartRecord = rd.next
	resp, err := rd.c.SearchRetrieve(rd.ctx, req)
	if err != nil {
		return err
	}
	rd.begun = true
	rd.buf = resp.Records
	rd.i = 0
	if resp.NextRecordPosition > rd.next && len(resp.Records) > 0 {
		rd.next = resp.NextRecordPosition
	} else {
		rd.done = true // no further page advertised
	}
	if len(resp.Records) == 0 {
		rd.done = true
		return io.EOF
	}
	return nil
}

// All returns an iterator over the remaining records, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}
