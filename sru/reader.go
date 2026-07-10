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
	c       *Client
	ctx     context.Context
	req     Request
	buf     []Record
	i       int
	next    int // startRecord of the next page to fetch (1-based)
	fetched int // records received so far, bounding the pager when nextRecordPosition is omitted
	total   int // numberOfRecords from the last page that reported one, else -1
	done    bool
	err     error
}

// compile-time assertions that Reader satisfies the core interface and reports
// its result-set size.
var (
	_ codex.RecordReader  = (*Reader)(nil)
	_ codex.RecordCounter = (*Reader)(nil)
)

// NewReader returns a Reader over the result set for query, using the client's
// default schema and page size. The context governs every underlying HTTP
// request; cancel it to stop an in-progress stream.
func (c *Client) NewReader(ctx context.Context, query string) *Reader {
	return &Reader{c: c, ctx: ctx, req: Request{Query: query}, next: 1, total: -1}
}

// Total reports the number of records the server said the result set holds, or
// -1 when that is unknown: before the first successful fetch, and on servers
// that omit numberOfRecords, which SRU 2.0 permits. Zero is a real answer,
// meaning the search matched nothing. It satisfies [codex.RecordCounter], so a
// caller holding a [codex.RecordReader] can ask without a type switch over the
// protocols.
func (rd *Reader) Total() int { return rd.total }

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
// set is exhausted. nextRecordPosition is optional in SRU (guaranteed absent only
// when the set is exhausted, but omitted always by some servers), so when a page
// carries records without one, the pager falls back to advancing by the records
// received, bounded by numberOfRecords -- next strictly increases either way, so
// paging always terminates.
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
	if resp.countKnown {
		rd.total = resp.NumberOfRecords
	}
	rd.buf = resp.Records
	rd.i = 0
	rd.fetched += len(resp.Records)
	switch {
	case len(resp.Records) == 0:
		rd.done = true
		return io.EOF
	case resp.NextRecordPosition > rd.next:
		rd.next = resp.NextRecordPosition
	case rd.fetched < resp.NumberOfRecords:
		rd.next += len(resp.Records)
	default:
		rd.done = true
	}
	return nil
}

// All returns an iterator over the remaining records, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}
