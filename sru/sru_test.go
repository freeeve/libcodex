package sru

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	codex "github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/marcjson"
)

// fixture reads a testdata SRU response document.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// serve returns a test server that replies to a searchRetrieve request with the
// body chosen by pick from the request's query values.
func serve(t *testing.T, pick func(url.Values) []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("operation") != "searchRetrieve" {
			http.Error(w, "bad operation", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write(pick(q))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSearchRetrieve checks that one page is parsed into records with the right
// metadata, that the request carries the expected SRU parameters, and that a
// MARCXML record decodes to a *codex.Record.
func TestSearchRetrieve(t *testing.T) {
	var gotQuery url.Values
	srv := serve(t, func(q url.Values) []byte {
		gotQuery = q
		return fixture(t, "search_page1.xml")
	})
	c := NewClient(srv.URL)

	resp, err := c.SearchRetrieve(context.Background(), Request{Query: `dc.title = "whale"`})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	if gotQuery.Get("version") != "1.2" || gotQuery.Get("recordSchema") != "marcxml" ||
		gotQuery.Get("query") != `dc.title = "whale"` || gotQuery.Get("startRecord") != "1" {
		t.Errorf("request params = %v", gotQuery)
	}
	if resp.NumberOfRecords != 3 || resp.NextRecordPosition != 3 || len(resp.Records) != 2 {
		t.Fatalf("counts: n=%d next=%d records=%d", resp.NumberOfRecords, resp.NextRecordPosition, len(resp.Records))
	}
	r0 := resp.Records[0]
	if r0.Schema != "marcxml" || r0.Packing != "xml" || r0.Position != 1 {
		t.Errorf("record[0] = %+v", r0)
	}
	rec, err := r0.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := rec.ControlField("001"); got != "92005291" {
		t.Errorf("001 = %q", got)
	}
	if got := rec.SubfieldValue("245", 'a'); got != "Stone butch blues :" {
		t.Errorf("245$a = %q", got)
	}
	if got := rec.SubfieldValue("100", 'a'); got != "Feinberg, Leslie." {
		t.Errorf("100$a = %q", got)
	}
	if got := rec.SubfieldValue("020", 'a'); got != "0786803525" {
		t.Errorf("020$a = %q", got)
	}
}

// TestReaderPaging checks that the Reader follows nextRecordPosition across two
// pages, yields every record in order, then io.EOF, and satisfies ReadAll.
func TestReaderPaging(t *testing.T) {
	srv := serve(t, func(q url.Values) []byte {
		if q.Get("startRecord") == "3" {
			return fixture(t, "search_page2.xml")
		}
		return fixture(t, "search_page1.xml")
	})
	c := NewClient(srv.URL)

	recs, err := codex.ReadAll(c.NewReader(context.Background(), "melville"))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("read %d records, want 3", len(recs))
	}
	want := []string{"Stone butch blues :", "Moby-Dick, or, The whale /", "Pride and prejudice /"}
	for i, w := range want {
		if got := recs[i].SubfieldValue("245", 'a'); got != w {
			t.Errorf("record %d 245$a = %q, want %q", i, got, w)
		}
	}
}

// TestReaderPagingWithoutNextRecordPosition checks the pager against a server
// that omits nextRecordPosition entirely (the element is optional): the Reader
// must fall back to advancing by the records received, bounded by
// numberOfRecords, instead of truncating the result set to the first page.
func TestReaderPagingWithoutNextRecordPosition(t *testing.T) {
	page1 := bytes.ReplaceAll(fixture(t, "search_page1.xml"),
		[]byte("<zs:nextRecordPosition>3</zs:nextRecordPosition>"), nil)
	srv := serve(t, func(q url.Values) []byte {
		if q.Get("startRecord") == "3" {
			return fixture(t, "search_page2.xml")
		}
		return page1
	})
	c := NewClient(srv.URL)

	recs, err := codex.ReadAll(c.NewReader(context.Background(), "q"))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("read %d records, want 3 (pager must not stop when nextRecordPosition is omitted)", len(recs))
	}
}

// TestHTTPError checks that a non-200 response is a transport error from both
// SearchRetrieve and the Reader.
func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)

	if _, err := c.SearchRetrieve(context.Background(), Request{Query: "x"}); err == nil {
		t.Error("SearchRetrieve on HTTP 500 should error")
	}
	if _, err := c.NewReader(context.Background(), "x").Read(); err == nil {
		t.Error("Reader.Read on HTTP 500 should error")
	}
}

// TestDiagnostics checks that a zero-record diagnostics response surfaces as a
// *DiagnosticsError from both SearchRetrieve and the Reader.
func TestDiagnostics(t *testing.T) {
	srv := serve(t, func(url.Values) []byte { return fixture(t, "diagnostic.xml") })
	c := NewClient(srv.URL)

	resp, err := c.SearchRetrieve(context.Background(), Request{Query: "?"})
	var de *DiagnosticsError
	if !errors.As(err, &de) {
		t.Fatalf("SearchRetrieve err = %v, want *DiagnosticsError", err)
	}
	if resp == nil || resp.NumberOfRecords != 0 {
		t.Errorf("resp = %+v", resp)
	}
	if len(de.Diagnostics) != 1 || de.Diagnostics[0].Message != "Query syntax error" {
		t.Errorf("diagnostics = %+v", de.Diagnostics)
	}

	_, rerr := c.NewReader(context.Background(), "?").Read()
	if !errors.As(rerr, &de) {
		t.Errorf("Reader.Read err = %v, want *DiagnosticsError", rerr)
	}
}

// TestStringPacking checks that a recordPacking="string" payload is unescaped and
// decodes like inline XML.
func TestStringPacking(t *testing.T) {
	srv := serve(t, func(url.Values) []byte { return fixture(t, "string_packing.xml") })
	c := NewClient(srv.URL)

	resp, err := c.SearchRetrieve(context.Background(), Request{Query: "x"})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	if len(resp.Records) != 1 || resp.Records[0].Packing != "string" {
		t.Fatalf("records = %+v", resp.Records)
	}
	rec, err := resp.Records[0].Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := rec.SubfieldValue("245", 'a'); got != "Escaped title /" {
		t.Errorf("245$a = %q", got)
	}
}

// TestConvertEndToEnd streams an SRU search straight into a MARCJSON writer,
// proving the Reader is a drop-in codex.Convert source.
func TestConvertEndToEnd(t *testing.T) {
	srv := serve(t, func(q url.Values) []byte {
		if q.Get("startRecord") == "3" {
			return fixture(t, "search_page2.xml")
		}
		return fixture(t, "search_page1.xml")
	})
	c := NewClient(srv.URL)

	var buf bytes.Buffer
	w := marcjson.NewWriter(&buf)
	if err := codex.Convert(c.NewReader(context.Background(), "q"), w); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if err := codex.Close(w); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Stone butch blues :", "Moby-Dick, or, The whale /", "Pride and prejudice /"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("MARCJSON output missing %q; got %s", want, out)
		}
	}
}

// TestNonMarcxmlRecord checks that a non-MARCXML record exposes its raw payload
// but refuses to decode, and that the Reader skips it.
func TestNonMarcxmlRecord(t *testing.T) {
	const body = `<zs:searchRetrieveResponse xmlns:zs="http://www.loc.gov/zing/srw/">
  <zs:numberOfRecords>1</zs:numberOfRecords>
  <zs:records><zs:record>
    <zs:recordSchema>info:srw/schema/1/mods-v3.7</zs:recordSchema>
    <zs:recordPacking>xml</zs:recordPacking>
    <zs:recordData><mods xmlns="http://www.loc.gov/mods/v3"><titleInfo><title>A MODS record</title></titleInfo></mods></zs:recordData>
    <zs:recordPosition>1</zs:recordPosition>
  </zs:record></zs:records>
</zs:searchRetrieveResponse>`
	srv := serve(t, func(url.Values) []byte { return []byte(body) })
	c := NewClient(srv.URL)

	resp, err := c.SearchRetrieve(context.Background(), Request{Query: "x", Schema: "mods"})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	if resp.Records[0].Schema != "mods" {
		t.Errorf("schema = %q, want mods", resp.Records[0].Schema)
	}
	if !bytes.Contains(resp.Records[0].Data, []byte("A MODS record")) {
		t.Errorf("raw payload not preserved: %s", resp.Records[0].Data)
	}
	if _, err := resp.Records[0].Decode(); err == nil {
		t.Error("Decode of a MODS record should error")
	}
	// The Reader skips the non-MARCXML record and reaches EOF cleanly.
	if recs, err := codex.ReadAll(c.NewReader(context.Background(), "x")); err != nil || len(recs) != 0 {
		t.Errorf("ReadAll = %d records, %v; want 0, nil", len(recs), err)
	}
}

// TestNormalizeSchema covers the short-name, info-URI and namespace-URI forms.
func TestNormalizeSchema(t *testing.T) {
	cases := map[string]string{
		"marcxml":                          "marcxml",
		"info:srw/schema/1/marcxml-v1.1":   "marcxml",
		"http://www.loc.gov/MARC21/slim":   "marcxml",
		"mods":                             "mods",
		"info:srw/schema/1/mods-v3.7":      "mods",
		"http://www.loc.gov/mods/v3":       "mods",
		"dc":                               "dc",
		"oai_dc":                           "dc",
		"info:srw/schema/1/dc-v1.1":        "dc",
		"http://purl.org/dc/elements/1.1/": "dc",
		"something/else":                   "something/else",
	}
	for in, want := range cases {
		if got := normalizeSchema(in); got != want {
			t.Errorf("normalizeSchema(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestQuote covers CQL term escaping.
func TestQuote(t *testing.T) {
	cases := map[string]string{
		`moby dick`:    `"moby dick"`,
		`say "hi"`:     `"say \"hi\""`,
		`back\slash`:   `"back\\slash"`,
		`both " and \`: `"both \" and \\"`,
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

// FuzzParseResponse asserts the envelope parser never panics on arbitrary input.
func FuzzParseResponse(f *testing.F) {
	for _, name := range []string{"search_page1.xml", "search_page2.xml", "diagnostic.xml", "string_packing.xml"} {
		if b, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(_ *testing.T, data []byte) {
		if resp, err := parseResponse(data); err == nil {
			for _, r := range resp.Records {
				_, _ = r.Decode()
			}
		}
	})
}
