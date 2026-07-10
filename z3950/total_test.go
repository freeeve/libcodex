package z3950

import (
	"context"
	"fmt"
	"io"
	"testing"

	codex "github.com/freeeve/libcodex"
)

// TestReaderTotalBeforeSearch checks that Total reports -1 until the first Read
// has run the search, so a caller cannot mistake "not asked yet" for no hits.
func TestReaderTotalBeforeSearch(t *testing.T) {
	var records [][]byte
	for i := 1; i <= 3; i++ {
		records = append(records, marcNamePlusRecordT(sampleMARC(t, fmt.Sprintf("%04d", i), fmt.Sprintf("Title %d", i))))
	}
	srv := &fakeServer{initOK: true, hits: 3, records: records}
	rd := NewClient(srv.start(t)+"/biblios").NewReader(context.Background(), Term("any", "x"))

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

// TestReaderTotalEmptyResultSet checks that a search matching nothing gives
// Total 0, not the -1 that means unknown.
func TestReaderTotalEmptyResultSet(t *testing.T) {
	srv := &fakeServer{initOK: true, hits: 0}
	rd := NewClient(srv.start(t)+"/biblios").NewReader(context.Background(), Term("any", "nothing"))

	if _, err := rd.Read(); err != io.EOF {
		t.Fatalf("Read on empty result set = %v, want io.EOF", err)
	}
	if got := rd.Total(); got != 0 {
		t.Errorf("Total = %d, want 0 (the server said zero hits)", got)
	}
}

// TestReaderTotalStaysUnknownAfterError checks that a rejected Init leaves Total
// unknown rather than reporting a count no search ever produced.
func TestReaderTotalStaysUnknownAfterError(t *testing.T) {
	srv := &fakeServer{initOK: false}
	rd := NewClient(srv.start(t)+"/biblios").NewReader(context.Background(), Term("any", "x"))

	if _, err := rd.Read(); err == nil {
		t.Fatal("Read succeeded against a server that rejected Init")
	}
	if got := rd.Total(); got != -1 {
		t.Errorf("Total after a failed fetch = %d, want -1", got)
	}
}

// TestReaderIsRecordCounter checks the Reader answers Total through the optional
// codex.RecordCounter interface, matching sru.Reader, so a caller holding a
// codex.RecordReader needs no type switch over the protocols.
func TestReaderIsRecordCounter(t *testing.T) {
	srv := &fakeServer{initOK: true, hits: 1,
		records: [][]byte{marcNamePlusRecordT(sampleMARC(t, "0001", "Moby Dick"))}}
	var r codex.RecordReader = NewClient(srv.start(t)+"/biblios").NewReader(context.Background(), Term("any", "x"))

	rc, ok := r.(codex.RecordCounter)
	if !ok {
		t.Fatal("z3950.Reader does not satisfy codex.RecordCounter")
	}
	if _, err := r.Read(); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := rc.Total(); got != 1 {
		t.Errorf("Total via codex.RecordCounter = %d, want 1", got)
	}
}
