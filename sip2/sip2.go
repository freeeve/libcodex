// Package sip2 is a client for the 3M Standard Interchange Protocol version 2
// (SIP2), the request/response protocol integrated library systems speak for
// real-time circulation. It implements the discovery slice: an optional Login
// (93/94) and Item Information (17/18) over a TCP session, for asking an ILS
// "is this item on the shelf?" It does not cover checkout, holds, fees or patron
// messages.
//
// A session is strictly request/response and a [Conn] is not safe for concurrent
// use. The wire format is line-oriented: a two-digit command code, fixed-length
// fields, then pipe-delimited variable fields, terminated by a carriage return,
// with an optional trailing AY/AZ error-detection checksum.
package sip2

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// Client holds the SIP2 target and session defaults. The zero value needs at
// least Address set; construct one with [NewClient].
type Client struct {
	Address string // host:port of the SIP2 server (the ACS)

	// Login (93) is sent only when User is non-empty: CN=User, CO=Password, and
	// CP=Location when set. Both algorithm bytes are the plain "0".
	User     string
	Password string
	Location string // login location code (CP); optional

	// InstitutionID (AO) and TerminalPass (AC) ride on each Item Information
	// request; both optional, sent as empty fields when unset.
	InstitutionID string
	TerminalPass  string

	// ErrorDetection appends the AY sequence and AZ checksum to each outbound
	// message. Inbound AY/AZ are always tolerated and skipped regardless.
	ErrorDetection bool

	// Dial is the connection seam, overridable in tests. When nil a plain TCP
	// dialer bound to the request context is used.
	Dial func(ctx context.Context, addr string) (net.Conn, error)
}

// NewClient returns a Client for a host:port SIP2 target.
func NewClient(addr string) *Client { return &Client{Address: addr} }

// Conn is one SIP2 session over a live connection. Not safe for concurrent use.
type Conn struct {
	c   *Client
	nc  net.Conn
	r   *bufio.Reader
	seq int // rolling AY sequence number, 0-9
}

// Connect dials the target and, when a User is configured, performs the Login
// (93/94) exchange. Close the Conn when done.
func (c *Client) Connect(ctx context.Context) (*Conn, error) {
	dial := c.Dial
	if dial == nil {
		dial = func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}
	}
	nc, err := dial(ctx, c.Address)
	if err != nil {
		return nil, fmt.Errorf("sip2: dial %s: %w", c.Address, err)
	}
	co := &Conn{c: c, nc: nc, r: bufio.NewReader(nc)}
	if c.User != "" {
		if err := co.login(ctx); err != nil {
			nc.Close()
			return nil, err
		}
	}
	return co, nil
}

// Close ends the session by closing the underlying connection. SIP2 has no
// mandatory logout for the item-information slice.
func (co *Conn) Close() error { return co.nc.Close() }

// login performs the 93 -> 94 exchange; a reply beginning "941" is success.
func (co *Conn) login(ctx context.Context) error {
	var b strings.Builder
	b.WriteString("9300") // 93 + UID algorithm 0 + PWD algorithm 0
	writeField(&b, "CN", co.c.User)
	writeField(&b, "CO", co.c.Password)
	if co.c.Location != "" {
		writeField(&b, "CP", co.c.Location)
	}
	resp, err := co.roundTrip(ctx, b.String())
	if err != nil {
		return fmt.Errorf("sip2: login: %w", err)
	}
	if !strings.HasPrefix(resp, "941") {
		return fmt.Errorf("sip2: login rejected by %s", co.c.Address)
	}
	return nil
}

// ItemInfo is a parsed Item Information Response (18): the fixed status header and
// the variable fields a discovery caller reads. Fields holds every variable field
// (first occurrence) for anything not surfaced as a named member.
type ItemInfo struct {
	CirculationStatus string // 2-digit code, 01-13 (see CirculationStatusLabel)
	StatusLabel       string // human meaning of CirculationStatus
	SecurityMarker    string // 2-digit security marker
	FeeType           string // 2-digit fee type
	TransactionDate   string // 18-char SIP2 date

	ItemID            string // AB
	Title             string // AJ
	DueDate           string // AH
	CurrentLocation   string // AP
	PermanentLocation string // AQ
	CallNumber        string // CS (3M extension)
	HoldQueueLength   string // CF

	Fields map[string]string // all variable fields by 2-char code
}

// ItemInformation requests circulation status for one item barcode (17) and parses
// the response (18).
func (co *Conn) ItemInformation(ctx context.Context, itemID string) (*ItemInfo, error) {
	var b strings.Builder
	b.WriteString("17")
	b.WriteString(sipDate(time.Now()))
	writeField(&b, "AO", co.c.InstitutionID)
	writeField(&b, "AB", itemID)
	if co.c.TerminalPass != "" {
		writeField(&b, "AC", co.c.TerminalPass)
	}
	resp, err := co.roundTrip(ctx, b.String())
	if err != nil {
		return nil, fmt.Errorf("sip2: item information: %w", err)
	}
	return parseItemInfo(resp)
}

// parseItemInfo parses an 18 response: the 24-character fixed header (status,
// security marker, fee type, transaction date) then the variable fields.
func parseItemInfo(msg string) (*ItemInfo, error) {
	if !strings.HasPrefix(msg, "18") {
		return nil, fmt.Errorf("sip2: expected an 18 response, got %q", truncate(msg))
	}
	body := msg[2:]
	if len(body) < 24 {
		return nil, fmt.Errorf("sip2: 18 response too short for its fixed header")
	}
	info := &ItemInfo{
		CirculationStatus: body[0:2],
		SecurityMarker:    body[2:4],
		FeeType:           body[4:6],
		TransactionDate:   body[6:24],
		Fields:            parseFields(body[24:]),
	}
	info.StatusLabel = CirculationStatusLabel(info.CirculationStatus)
	info.ItemID = info.Fields["AB"]
	info.Title = info.Fields["AJ"]
	info.DueDate = info.Fields["AH"]
	info.CurrentLocation = info.Fields["AP"]
	info.PermanentLocation = info.Fields["AQ"]
	info.CallNumber = info.Fields["CS"]
	info.HoldQueueLength = info.Fields["CF"]
	return info, nil
}

// roundTrip frames and sends a message, then reads one CR-terminated reply. The
// context's deadline, when set, bounds the whole exchange.
func (co *Conn) roundTrip(ctx context.Context, msg string) (string, error) {
	dl, _ := ctx.Deadline()
	_ = co.nc.SetDeadline(dl)
	if _, err := co.nc.Write(frame(msg, co.c.ErrorDetection, co.seq)); err != nil {
		return "", err
	}
	co.seq = (co.seq + 1) % 10
	line, err := co.r.ReadString(msgTerm[0])
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// writeField appends a SIP2 variable field: the 2-char code, the value, and the
// terminating pipe.
func writeField(b *strings.Builder, code, value string) {
	b.WriteString(code)
	b.WriteString(value)
	b.WriteString(fieldDelim)
}

// truncate shortens a message for an error string.
func truncate(s string) string {
	if len(s) > 16 {
		return s[:16] + "..."
	}
	return s
}
