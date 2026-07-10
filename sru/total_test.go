package sru

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	codex "github.com/freeeve/libcodex"
)

// emptyResponse is a well-formed searchRetrieveResponse for a search that
// matched nothing: numberOfRecords is present and zero, and there are no
// records. It must not read as "the count is unknown".
const emptyResponse = `<?xml version="1.0" encoding="UTF-8"?>
<zs:searchRetrieveResponse xmlns:zs="http://www.loc.gov/zing/srw/">
  <zs:version>1.2</zs:version>
  <zs:numberOfRecords>0</zs:numberOfRecords>
</zs:searchRetrieveResponse>`

// TestReaderTotalBeforeFetch checks that Total reports -1 until a fetch has
// succeeded, so a caller cannot mistake "not asked yet" for an empty result set.
func TestReaderTotalBeforeFetch(t *testing.T) {
	srv := serve(t, func(url.Values) []byte { return fixture(t, "search_page1.xml") })
	rd := NewClient(srv.URL).NewReader(context.Background(), "melville")
	if got := rd.Total(); got != -1 {
		t.Fatalf("Total before first Read = %d, want -1", got)
	}
	if _, err := rd.Read(); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := rd.Total(); got != 3 {
		t.Errorf("Total after first Read = %d, want 3", got)
	}
}

// TestReaderTotalEmptyResultSet checks that a server reporting zero hits gives
// Total 0, not the -1 that means unknown.
func TestReaderTotalEmptyResultSet(t *testing.T) {
	srv := serve(t, func(url.Values) []byte { return []byte(emptyResponse) })
	rd := NewClient(srv.URL).NewReader(context.Background(), "nothing")
	if _, err := rd.Read(); err != io.EOF {
		t.Fatalf("Read on empty result set = %v, want io.EOF", err)
	}
	if got := rd.Total(); got != 0 {
		t.Errorf("Total = %d, want 0 (the server said zero hits)", got)
	}
}

// TestReaderTotalOmittedCount checks that a server omitting numberOfRecords --
// which SRU 2.0 permits -- leaves Total at -1 rather than reporting 0 hits for a
// result set that plainly has records in it.
func TestReaderTotalOmittedCount(t *testing.T) {
	page1 := bytes.ReplaceAll(fixture(t, "search_page1.xml"),
		[]byte("<zs:numberOfRecords>3</zs:numberOfRecords>"), nil)
	srv := serve(t, func(q url.Values) []byte {
		if q.Get("startRecord") == "3" {
			return fixture(t, "search_page2.xml")
		}
		return page1
	})
	rd := NewClient(srv.URL).NewReader(context.Background(), "q")
	if _, err := rd.Read(); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := rd.Total(); got != -1 {
		t.Errorf("Total with numberOfRecords omitted = %d, want -1 (unknown, not zero hits)", got)
	}
}

// TestReaderTotalStaysUnknownAfterError checks that a fetch that fails leaves
// Total unknown rather than reporting a count the server never delivered.
func TestReaderTotalStaysUnknownAfterError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	rd := NewClient(srv.URL).NewReader(context.Background(), "q")
	if _, err := rd.Read(); err == nil {
		t.Fatal("Read succeeded against a failing server")
	}
	if got := rd.Total(); got != -1 {
		t.Errorf("Total after a failed fetch = %d, want -1", got)
	}
}

// TestReaderIsRecordCounter checks the Reader answers Total through the optional
// codex.RecordCounter interface, so a caller holding a codex.RecordReader needs
// no type switch over the protocols.
func TestReaderIsRecordCounter(t *testing.T) {
	srv := serve(t, func(url.Values) []byte { return fixture(t, "search_page1.xml") })
	var r codex.RecordReader = NewClient(srv.URL).NewReader(context.Background(), "melville")
	rc, ok := r.(codex.RecordCounter)
	if !ok {
		t.Fatal("sru.Reader does not satisfy codex.RecordCounter")
	}
	if _, err := r.Read(); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := rc.Total(); got != 3 {
		t.Errorf("Total via codex.RecordCounter = %d, want 3", got)
	}
}

// TestResponseCountKnown checks the parser distinguishes an absent
// numberOfRecords from a zero one, which the exported int alone cannot express.
func TestResponseCountKnown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  []byte
		want  int
		known bool
	}{
		{"present", []byte(emptyResponse), 0, true},
		{"absent", bytes.ReplaceAll([]byte(emptyResponse),
			[]byte("<zs:numberOfRecords>0</zs:numberOfRecords>"), nil), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := parseResponse(tc.body)
			if err != nil {
				t.Fatalf("parseResponse: %v", err)
			}
			if resp.NumberOfRecords != tc.want || resp.countKnown != tc.known {
				t.Errorf("NumberOfRecords=%d countKnown=%v, want %d/%v",
					resp.NumberOfRecords, resp.countKnown, tc.want, tc.known)
			}
		})
	}
}
