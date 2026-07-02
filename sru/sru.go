// Package sru is a client for the SRU (Search/Retrieve via URL) protocol, the
// modern HTTP successor to Z39.50 used by library catalogs for search and
// retrieval. It performs a searchRetrieve operation over net/http and hands the
// bibliographic records embedded in the XML response to libcodex's decoders,
// using only the standard library.
//
// SRU responses embed each record in a recordData element in a negotiated
// schema. This client decodes MARCXML records into *codex.Record (via the marcxml
// package) and exposes any other schema's payload (MODS, Dublin Core, ...) as raw
// XML bytes, since those crosswalks are encode-only in this library. The [Reader]
// implements codex.RecordReader, so a catalog search is a drop-in source for
// codex.Convert:
//
//	c := sru.NewClient("http://lx2.loc.gov:210/lcdb")
//	rd := c.NewReader(ctx, `dc.title = "moby dick"`)
//	codex.Convert(rd, marcjson.NewWriter(os.Stdout))
//
// It targets SRU 1.1/1.2 (the widely deployed versions); the query is CQL, passed
// through verbatim -- use [Quote] to escape a user-supplied term.
package sru

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	codex "github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/marcxml"
)

// defaultVersion is the SRU protocol version sent when a Client sets none. 1.2 is
// the most widely deployed; 1.1 shares the same request parameters.
const defaultVersion = "1.2"

// defaultMaxRecords is the page size requested when a Client and Request both
// leave it unset.
const defaultMaxRecords = 10

// defaultMaxResponseBytes bounds how much of a response body the client will
// buffer when the Client sets no limit: far above any real SRU page (which is
// bounded by maximumRecords) while still capping memory against a misbehaving
// server.
const defaultMaxResponseBytes = 64 << 20

// Client is an SRU endpoint. The zero value is not usable; construct one with
// [NewClient]. HTTPClient, Version, Schema and MaxRecords are optional overrides.
type Client struct {
	BaseURL    string       // SRU endpoint URL (scheme://host/path)
	HTTPClient *http.Client // nil uses http.DefaultClient
	Version    string       // SRU version; "" uses defaultVersion ("1.2")
	Schema     string       // default recordSchema; "" uses "marcxml"
	MaxRecords int          // records requested per page; <=0 uses defaultMaxRecords

	// MaxResponseBytes bounds how much of a response body is buffered: 0 uses a
	// generous 64 MiB default, negative means unlimited. A response over the
	// limit fails with a distinct error rather than being truncated.
	MaxResponseBytes int64
}

// NewClient returns a Client for the SRU endpoint at baseURL with default
// settings (SRU 1.2, marcxml schema, http.DefaultClient).
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) version() string {
	if c.Version != "" {
		return c.Version
	}
	return defaultVersion
}

func (c *Client) schema() string {
	if c.Schema != "" {
		return c.Schema
	}
	return "marcxml"
}

func (c *Client) maxRecords() int {
	if c.MaxRecords > 0 {
		return c.MaxRecords
	}
	return defaultMaxRecords
}

// Request is one searchRetrieve page. Only Query is required; the rest fall back
// to the Client's defaults.
type Request struct {
	Query       string // CQL query, passed through verbatim
	StartRecord int    // 1-based position of the first record; <1 means 1
	MaxRecords  int    // records to return; <=0 uses the Client default
	Schema      string // recordSchema; "" uses the Client default
}

// Response is one parsed searchRetrieve response.
type Response struct {
	Version            string       // the server's reported protocol version
	NumberOfRecords    int          // total hits in the result set
	NextRecordPosition int          // start of the next page, or 0 when exhausted
	Records            []Record     // the records on this page
	Diagnostics        []Diagnostic // non-fatal or fatal diagnostics the server returned
}

// Err returns a *DiagnosticsError when the response carries diagnostics but no
// records (a failed search), or nil otherwise. A response with both records and
// diagnostics is treated as a successful partial result.
func (r *Response) Err() error {
	if len(r.Records) == 0 && len(r.Diagnostics) > 0 {
		return &DiagnosticsError{Diagnostics: r.Diagnostics}
	}
	return nil
}

// Record is one record carried in a searchRetrieve response.
type Record struct {
	Schema   string // normalized record schema: "marcxml", "mods", "dc", or the raw id
	Packing  string // "xml" or "string"
	Position int    // 1-based position in the result set
	Data     []byte // the record payload as XML bytes, in its schema
}

// Decode parses a MARCXML record payload into a *codex.Record. It returns an
// error for any other schema, whose payload remains available in Data.
func (r Record) Decode() (*codex.Record, error) {
	if r.Schema != "marcxml" {
		return nil, fmt.Errorf("sru: cannot decode record schema %q into codex.Record (only marcxml)", r.Schema)
	}
	return marcxml.Decode(r.Data)
}

// Diagnostic is one SRU diagnostic (an error or warning) from the server.
type Diagnostic struct {
	URI     string // diagnostic identifier, e.g. info:srw/diagnostic/1/7
	Message string // human-readable message
	Details string // extra context, e.g. the offending value
}

// DiagnosticsError reports that a searchRetrieve returned diagnostics and no
// records. It carries every diagnostic the server sent.
type DiagnosticsError struct {
	Diagnostics []Diagnostic
}

// Error summarizes the first diagnostic and the count of any others.
func (e *DiagnosticsError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "sru: diagnostic"
	}
	d := e.Diagnostics[0]
	msg := d.Message
	if msg == "" {
		msg = d.URI
	}
	if d.Details != "" {
		msg += ": " + d.Details
	}
	if len(e.Diagnostics) > 1 {
		return fmt.Sprintf("sru: %s (+%d more)", msg, len(e.Diagnostics)-1)
	}
	return fmt.Sprintf("sru: %s", msg)
}

// SearchRetrieve runs one searchRetrieve request and parses the response. It
// returns a transport or parse error with a nil Response; on a well-formed
// response it returns the Response together with its [Response.Err] (a
// *DiagnosticsError when the search failed with diagnostics, else nil), so the
// records and counts remain available for inspection.
func (c *Client) SearchRetrieve(ctx context.Context, r Request) (*Response, error) {
	target, err := c.buildURL(r)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sru: unexpected HTTP status %s", resp.Status)
	}
	body, err := c.readBody(resp.Body)
	if err != nil {
		return nil, err
	}
	out, err := parseResponse(body)
	if err != nil {
		return nil, err
	}
	return out, out.Err()
}

// readBody buffers a response body up to the client's size limit, failing with
// a limit-naming error (not a truncated parse error) when the server sends more.
func (c *Client) readBody(r io.Reader) ([]byte, error) {
	limit := c.MaxResponseBytes
	if limit == 0 {
		limit = defaultMaxResponseBytes
	}
	if limit < 0 {
		body, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("sru: read response: %w", err)
		}
		return body, nil
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("sru: read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("sru: response exceeds %d bytes; raise Client.MaxResponseBytes to allow larger responses", limit)
	}
	return body, nil
}

// buildURL renders the searchRetrieve request URL, merging the SRU parameters
// with any already present on the base endpoint.
func (c *Client) buildURL(r Request) (string, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", fmt.Errorf("sru: invalid base URL: %w", err)
	}
	start := max(r.StartRecord, 1)
	count := r.MaxRecords
	if count <= 0 {
		count = c.maxRecords()
	}
	schema := r.Schema
	if schema == "" {
		schema = c.schema()
	}
	q := base.Query()
	q.Set("operation", "searchRetrieve")
	q.Set("version", c.version())
	q.Set("query", r.Query)
	q.Set("startRecord", strconv.Itoa(start))
	q.Set("maximumRecords", strconv.Itoa(count))
	q.Set("recordSchema", schema)
	q.Set("recordPacking", "xml")
	base.RawQuery = q.Encode()
	return base.String(), nil
}

// xmlResponse mirrors a searchRetrieveResponse. All tags are local names so the
// parse is namespace-agnostic (SRU servers use zs:/srw: prefixes inconsistently).
type xmlResponse struct {
	Version            string      `xml:"version"`
	NumberOfRecords    int         `xml:"numberOfRecords"`
	NextRecordPosition int         `xml:"nextRecordPosition"`
	Records            []xmlRecord `xml:"records>record"`
	Diagnostics        []xmlDiag   `xml:"diagnostics>diagnostic"`
}

type xmlRecord struct {
	Schema   string        `xml:"recordSchema"`
	Packing  string        `xml:"recordPacking"`
	Position int           `xml:"recordPosition"`
	Data     xmlRecordData `xml:"recordData"`
}

type xmlRecordData struct {
	Inner []byte `xml:",innerxml"`
}

type xmlDiag struct {
	URI     string `xml:"uri"`
	Message string `xml:"message"`
	Details string `xml:"details"`
}

// parseResponse unmarshals a searchRetrieveResponse and extracts each record's
// payload and schema, plus any diagnostics.
func parseResponse(body []byte) (*Response, error) {
	var xr xmlResponse
	if err := xml.Unmarshal(body, &xr); err != nil {
		return nil, fmt.Errorf("sru: parse response: %w", err)
	}
	out := &Response{
		Version:            strings.TrimSpace(xr.Version),
		NumberOfRecords:    xr.NumberOfRecords,
		NextRecordPosition: xr.NextRecordPosition,
	}
	for _, d := range xr.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{
			URI:     strings.TrimSpace(d.URI),
			Message: strings.TrimSpace(d.Message),
			Details: strings.TrimSpace(d.Details),
		})
	}
	for _, rec := range xr.Records {
		out.Records = append(out.Records, Record{
			Schema:   normalizeSchema(strings.TrimSpace(rec.Schema)),
			Packing:  strings.TrimSpace(rec.Packing),
			Position: rec.Position,
			Data:     payloadBytes(rec),
		})
	}
	return out, nil
}

// payloadBytes returns a record's payload as XML bytes. For recordPacking="xml"
// the inner XML is the record markup; for "string" it is XML-escaped text that is
// unescaped back into markup.
func payloadBytes(rec xmlRecord) []byte {
	inner := bytes.TrimSpace(rec.Data.Inner)
	if strings.EqualFold(strings.TrimSpace(rec.Packing), "string") {
		return []byte(xmlUnescape(string(inner)))
	}
	return inner
}

// xmlUnescape resolves XML entity and character references in s, turning
// recordPacking="string" escaped text (e.g. "&lt;record&gt;") back into markup.
func xmlUnescape(s string) string {
	dec := xml.NewDecoder(strings.NewReader(s))
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	return b.String()
}

// schema URIs and short names that normalize to a canonical schema token.
const (
	marcxmlSlimNS = "http://www.loc.gov/MARC21/slim"
	modsV3NS      = "http://www.loc.gov/mods/v3"
	dcElementsNS  = "http://purl.org/dc/elements/1.1/"
)

// normalizeSchema folds the many recordSchema identifiers a server may send (short
// names, info: URIs, namespace URIs) to a canonical token: "marcxml", "mods" or
// "dc". An unrecognized identifier is returned unchanged.
func normalizeSchema(s string) string {
	switch {
	case s == "marcxml" || s == marcxmlSlimNS || strings.HasPrefix(s, "info:srw/schema/1/marcxml"):
		return "marcxml"
	case s == "mods" || s == modsV3NS || strings.HasPrefix(s, "info:srw/schema/1/mods"):
		return "mods"
	case s == "dc" || s == "oai_dc" || s == dcElementsNS || strings.HasPrefix(s, "info:srw/schema/1/dc"):
		return "dc"
	}
	return s
}
