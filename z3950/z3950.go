// Package z3950 is a client for the Z39.50 information-retrieval protocol
// (ANSI/NISO Z39.50 / ISO 23950), the classic library search protocol that SRU
// succeeded. It speaks BER-encoded APDUs over TCP -- Initialize, Search (Type-1
// RPN queries over the bib-1 attribute set), Present and Close -- implemented
// from the published standard using only the standard library.
//
// Retrieved records decode through libcodex's readers by record syntax: MARC21
// via iso2709, UNIMARC via unimarc, MARCXML via marcxml; SUTRS text is exposed
// raw. The [Reader] implements codex.RecordReader, so a Z39.50 search is a
// drop-in source for codex.Convert, mirroring the sru package:
//
//	c := z3950.NewClient("lx2.loc.gov:210/LCDB")
//	rd := c.NewReader(ctx, z3950.Term("title", "moby dick"))
//	codex.Convert(rd, marcjson.NewWriter(os.Stdout))
package z3950

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	codex "github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/unimarc"
)

// Client holds the target address and session defaults. The zero value is not
// usable; construct one with [NewClient].
type Client struct {
	Address   string   // host:port of the Z39.50 server
	Databases []string // databases to search
	Syntax    string   // preferred record syntax: "marc21" (default), "unimarc", "xml", "sutrs"
	PageSize  int      // records per Present; <=0 uses 10

	// Authentication, sent as idAuthentication in the Initialize request. User,
	// Password and Group select the structured idPass form; AuthOpen sends the
	// single-string open form instead, for the rare server that only accepts it.
	// All empty means anonymous (the field is omitted).
	User     string
	Password string
	Group    string
	AuthOpen string
}

// NewClient returns a Client for a target in host:port/database form (the
// conventional Z39.50 target notation, e.g. "lx2.loc.gov:210/LCDB").
func NewClient(target string) *Client {
	c := &Client{Address: target}
	if addr, db, ok := strings.Cut(target, "/"); ok {
		c.Address = addr
		if db != "" {
			c.Databases = []string{db}
		}
	}
	return c
}

func (c *Client) pageSize() int {
	if c.PageSize > 0 {
		return c.PageSize
	}
	return 10
}

// Record is one retrieved record: its record syntax, the raw payload, or a
// surrogate diagnostic when the server could not deliver this record. An OPAC
// record is unwrapped on arrival: Syntax/Data carry its embedded bibliographic
// record (so Decode works transparently) and Holdings carries its holdings.
type Record struct {
	Syntax   string      // "marc21", "unimarc", "xml", "sutrs", "opac", or a dotted OID
	Data     []byte      // raw record payload in its syntax
	Diag     *Diagnostic // set instead of Data for a surrogate diagnostic
	Holdings []Holding   // holdings data from an OPAC record; nil otherwise
}

// Holding is one holdings statement from an OPAC record: where a copy lives and
// how it circulates. Members the server omits are empty; unknown members are
// skipped, never a parse failure.
type Holding struct {
	NUCCode          string // holding institution (MARC organization code)
	LocalLocation    string
	ShelvingLocation string
	CallNumber       string
	CopyNumber       string
	PublicNote       string
	EnumAndChron     string // enumeration/chronology (volume, year)
	Circulation      []Circulation
}

// Circulation is one circulation record of a holding.
type Circulation struct {
	AvailableNow     bool
	AvailabilityDate string
	ItemID           string
	Renewable        bool
	OnHold           bool
}

// Decode parses the record into a *codex.Record for the MARC syntaxes: MARC21
// (ISO 2709), UNIMARC and MARCXML. Other syntaxes return an error; their payload
// remains available in Data.
func (r Record) Decode() (*codex.Record, error) {
	if r.Diag != nil {
		return nil, fmt.Errorf("z3950: surrogate diagnostic: %s", r.Diag)
	}
	switch r.Syntax {
	case "marc21":
		rec, _, err := iso2709.Decode(r.Data)
		return rec, err
	case "unimarc":
		return unimarc.Decode(r.Data)
	case "xml":
		return marcxml.Decode(r.Data)
	}
	return nil, fmt.Errorf("z3950: cannot decode record syntax %q into codex.Record", r.Syntax)
}

// Diagnostic is one bib-1 diagnostic from the server.
type Diagnostic struct {
	Set       string // diagnostic set ("bib-1")
	Condition int    // bib-1 condition code
	Message   string // human-readable addinfo
}

func (d Diagnostic) String() string {
	if d.Message != "" {
		return fmt.Sprintf("condition %d: %s", d.Condition, d.Message)
	}
	return fmt.Sprintf("condition %d", d.Condition)
}

// DiagnosticsError reports that a search or present failed with diagnostics.
type DiagnosticsError struct {
	Diagnostics []Diagnostic
}

func (e *DiagnosticsError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "z3950: diagnostic"
	}
	msg := e.Diagnostics[0].String()
	if len(e.Diagnostics) > 1 {
		return fmt.Sprintf("z3950: %s (+%d more)", msg, len(e.Diagnostics)-1)
	}
	return "z3950: " + msg
}

// Conn is one Z39.50 session: a TCP connection that has completed Initialize.
// It is not safe for concurrent use; the protocol is strictly request/response.
type Conn struct {
	c      *Client
	nc     net.Conn
	buf    []byte
	syntax []uint32
}

// Connect dials the target and negotiates the session with an Initialize
// exchange. Close the Conn when done.
func (c *Client) Connect(ctx context.Context) (*Conn, error) {
	syntax, err := syntaxOID(c.Syntax)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, err
	}
	conn := &Conn{c: c, nc: nc, syntax: syntax}
	resp, err := conn.roundTrip(ctx, encodeInitRequest(c))
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("z3950: initialize: %w", err)
	}
	init, ok := resp.(*initResponse)
	if !ok {
		nc.Close()
		return nil, fmt.Errorf("z3950: unexpected response to initialize")
	}
	if !init.result {
		nc.Close()
		return nil, fmt.Errorf("z3950: server %q rejected initialization", init.implementation)
	}
	return conn, nil
}

// Result reports the outcome of a Search: the total hit count on the server's
// result set, retrievable with [Conn.Present].
type Result struct {
	Count int // total hits in the result set
}

// Search runs a Type-1 RPN query against the client's databases, replacing the
// session's default result set. Records are then fetched with [Conn.Present].
func (co *Conn) Search(ctx context.Context, q Query) (*Result, error) {
	if len(co.c.Databases) == 0 {
		return nil, fmt.Errorf("z3950: no database configured (use host:port/database)")
	}
	rpn, err := q.rpn()
	if err != nil {
		return nil, err
	}
	resp, err := co.roundTrip(ctx, encodeSearchRequest(co.c.Databases, co.syntax, rpn))
	if err != nil {
		return nil, fmt.Errorf("z3950: search: %w", err)
	}
	sr, ok := resp.(*searchResponse)
	if !ok {
		return nil, fmt.Errorf("z3950: unexpected response to search")
	}
	if len(sr.diagnostics) > 0 {
		return nil, &DiagnosticsError{Diagnostics: sr.diagnostics}
	}
	if !sr.searchStatus {
		return nil, fmt.Errorf("z3950: search failed")
	}
	return &Result{Count: sr.resultCount}, nil
}

// Present fetches count records from the current result set starting at the
// 1-based position, in the session's preferred record syntax.
func (co *Conn) Present(ctx context.Context, start, count int) ([]Record, error) {
	resp, err := co.roundTrip(ctx, encodePresentRequest(start, count, co.syntax))
	if err != nil {
		return nil, fmt.Errorf("z3950: present: %w", err)
	}
	pr, ok := resp.(*presentResponse)
	if !ok {
		return nil, fmt.Errorf("z3950: unexpected response to present")
	}
	if len(pr.diagnostics) > 0 {
		return nil, &DiagnosticsError{Diagnostics: pr.diagnostics}
	}
	return pr.records, nil
}

// Close sends a Close APDU (best effort) and closes the connection.
func (co *Conn) Close() error {
	if co.nc == nil {
		return nil
	}
	co.nc.SetDeadline(time.Now().Add(2 * time.Second))
	co.nc.Write(encodeClose())
	err := co.nc.Close()
	co.nc = nil
	return err
}

// roundTrip writes one APDU and reads the next one from the server, honoring the
// context's deadline and cancelation by acting on the socket.
func (co *Conn) roundTrip(ctx context.Context, req []byte) (any, error) {
	if deadline, ok := ctx.Deadline(); ok {
		co.nc.SetDeadline(deadline)
	} else {
		co.nc.SetDeadline(time.Time{})
	}
	stop := context.AfterFunc(ctx, func() { co.nc.SetDeadline(time.Now()) })
	defer stop()

	if _, err := co.nc.Write(req); err != nil {
		return nil, err
	}
	for {
		if len(co.buf) > 0 {
			if _, err := berSize(co.buf); err == nil {
				break
			} else if err != errIncomplete {
				return nil, err
			}
		}
		chunk := make([]byte, 8192)
		n, err := co.nc.Read(chunk)
		if n > 0 {
			co.buf = append(co.buf, chunk[:n]...)
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
	}
	total, err := berSize(co.buf)
	if err != nil {
		return nil, err
	}
	pdu := co.buf[:total]
	co.buf = co.buf[total:]
	tag, resp, err := decodePDU(pdu)
	if err != nil {
		return nil, err
	}
	if tag == pduClose {
		return nil, fmt.Errorf("z3950: server closed the session")
	}
	return resp, nil
}
