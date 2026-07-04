package sru

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	codex "github.com/freeeve/libcodex"
)

// The live tests below drive the public, anonymous SRU endpoints libcatalog
// seeds as default copy-cataloging targets (backend/copycat DefaultTargets).
// They touch the open internet, so they are opt-in:
//
//	SRU_LIVE=1 go test ./sru/ -run TestLive -v
//
// Each is a regression guard for a specific real-server quirk this client
// handles: DNB's MARC21-xml schema label and 1.1-only protocol, LOC's
// Bath-profile identifier indexes, and K10plus's PICA index pass-through.

// liveGate skips a test unless SRU_LIVE is set (and never runs it under -short),
// so the default suite stays offline.
func liveGate(t *testing.T) context.Context {
	t.Helper()
	if testing.Short() {
		t.Skip("network test skipped under -short")
	}
	if os.Getenv("SRU_LIVE") == "" {
		t.Skip("set SRU_LIVE=1 to run live SRU endpoint tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// hasUnsupportedIndex reports whether any diagnostic is the CQL "unsupported
// index" error (info:srw/diagnostic/1/16) -- the failure LOC returns for the
// dc.isbn/dc.issn forms the Bath mapping replaced.
func hasUnsupportedIndex(diags []Diagnostic) bool {
	for _, d := range diags {
		if strings.HasSuffix(d.URI, "/1/16") || strings.Contains(strings.ToLower(d.Message), "unsupported index") {
			return true
		}
	}
	return false
}

// TestLiveDNB drives the Deutsche Nationalbibliothek, which answers only SRU
// 1.1 and labels its MARC21 slim payloads "MARC21-xml". It guards that
// normalizeSchema folds that label so the Reader decodes DNB records instead of
// silently skipping every one (the regression behind the schema fix).
func TestLiveDNB(t *testing.T) {
	ctx := liveGate(t)
	c := &Client{BaseURL: "https://services.dnb.de/sru/dnb", Version: "1.1", Schema: "MARC21-xml"}

	// DNB indexes both standard numbers under dnb.num; this ISBN returns hits.
	query := Term("dnb.num", "9783446235755").String()

	resp, err := c.SearchRetrieve(ctx, Request{Query: query, MaxRecords: 3})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	if resp.NumberOfRecords == 0 || len(resp.Records) == 0 {
		t.Fatalf("no DNB hits for %q: %+v", query, resp)
	}
	if got := resp.Records[0].Schema; got != "marcxml" {
		t.Fatalf("record schema = %q, want marcxml (MARC21-xml must normalize)", got)
	}

	recs, err := codex.ReadAll(c.NewReader(ctx, query))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("Reader yielded zero records -- MARC21-xml records were skipped")
	}
	for i, r := range recs {
		if r.SubfieldValue("245", 'a') == "" {
			t.Errorf("record %d has no 245$a", i)
		}
	}
	t.Logf("DNB: %d hits, decoded %d records; first 245$a=%q",
		resp.NumberOfRecords, len(recs), recs[0].SubfieldValue("245", 'a'))
}

// TestLiveLOC drives the Library of Congress, which speaks the Bath-profile
// identifier indexes. It guards that the isbn access point renders bath.isbn
// (not dc.isbn, which LOC rejects with diagnostic 1/16) and that a plain title
// search decodes MARC records.
func TestLiveLOC(t *testing.T) {
	ctx := liveGate(t)
	c := NewClient("http://lx2.loc.gov:210/LCDB")

	isbnQuery := Term("isbn", "9780142437247").String()
	if !strings.HasPrefix(isbnQuery, "bath.isbn") {
		t.Fatalf("isbn access point rendered %q, want bath.isbn form", isbnQuery)
	}
	resp, err := c.SearchRetrieve(ctx, Request{Query: isbnQuery, MaxRecords: 3})
	if err != nil {
		// A no-hit ISBN is fine; an unsupported-index diagnostic is the regression.
		if de, ok := err.(*DiagnosticsError); ok && hasUnsupportedIndex(de.Diagnostics) {
			t.Fatalf("LOC rejected the bath.isbn index: %v", err)
		}
		t.Fatalf("SearchRetrieve(%q): %v", isbnQuery, err)
	}
	if hasUnsupportedIndex(resp.Diagnostics) {
		t.Fatalf("LOC rejected the bath.isbn index: %+v", resp.Diagnostics)
	}
	t.Logf("LOC bath.isbn: %d hits", resp.NumberOfRecords)

	recs, err := codex.ReadAll(c.NewReader(ctx, Term("title", "moby dick").String()))
	if err != nil {
		t.Fatalf("title ReadAll: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no LOC records for a moby dick title search")
	}
	t.Logf("LOC dc.title: decoded %d records; first 245$a=%q", len(recs), recs[0].SubfieldValue("245", 'a'))
}

// TestLiveK10plus drives the K10plus union catalog, which indexes identifiers
// under its own PICA context set. It exercises a dotted index passed through
// verbatim by the builder against a real server returning MARC21 slim XML.
func TestLiveK10plus(t *testing.T) {
	ctx := liveGate(t)
	c := NewClient("https://sru.k10plus.de/opac-de-627")

	query := Term("pica.isb", "9783446235755").String()
	resp, err := c.SearchRetrieve(ctx, Request{Query: query, MaxRecords: 3})
	if err != nil {
		t.Fatalf("SearchRetrieve(%q): %v", query, err)
	}
	if resp.NumberOfRecords == 0 || len(resp.Records) == 0 {
		t.Fatalf("no K10plus hits for %q: %+v", query, resp)
	}
	if got := resp.Records[0].Schema; got != "marcxml" {
		t.Fatalf("record schema = %q, want marcxml", got)
	}
	rec, err := resp.Records[0].Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	t.Logf("K10plus pica.isb: %d hits; first 245$a=%q", resp.NumberOfRecords, rec.SubfieldValue("245", 'a'))
}
