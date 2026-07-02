package sru

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	codex "github.com/freeeve/libcodex"
)

// startYazZtest spawns YAZ's test server (whose generic frontend listens for both
// Z39.50 and HTTP/SRU on one port) on a free local port and returns the SRU base
// URL. Skipped when YAZ is not installed, like the pymarc interop tests.
func startYazZtest(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("yaz-ztest")
	if err != nil {
		t.Skipf("yaz-ztest unavailable (%v); brew install yaz to run the interop test", err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	cmd := exec.Command(bin, fmt.Sprintf("tcp:@:%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start yaz-ztest: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for deadline := time.Now().Add(5 * time.Second); ; {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("yaz-ztest did not start listening")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Sprintf("http://%s/Default", addr)
}

// TestYazZtestInterop runs the client against YAZ's test server -- the reference
// implementation -- over real HTTP. yaz-ztest omits nextRecordPosition, so this
// also exercises the record-count paging fallback against a real peer.
func TestYazZtestInterop(t *testing.T) {
	c := NewClient(startYazZtest(t))
	c.MaxRecords = 5 // force paging
	ctx := context.Background()

	resp, err := c.SearchRetrieve(ctx, Request{Query: "computer"})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	if resp.NumberOfRecords == 0 || len(resp.Records) == 0 {
		t.Fatalf("no hits from yaz-ztest: %+v", resp)
	}
	if resp.Records[0].Schema != "marcxml" {
		t.Errorf("schema = %q, want marcxml", resp.Records[0].Schema)
	}

	recs, err := codex.ReadAll(c.NewReader(ctx, "computer"))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) != resp.NumberOfRecords {
		t.Errorf("paged %d records, server reports %d hits", len(recs), resp.NumberOfRecords)
	}
	for i, r := range recs {
		if r.SubfieldValue("245", 'a') == "" {
			t.Errorf("record %d has no 245$a", i)
		}
	}
}

// TestLiveEndpoint is an opt-in smoke test against a real SRU endpoint, e.g.
//
//	SRU_LIVE_URL=http://lx2.loc.gov:210/lcdb go test ./sru/ -run TestLiveEndpoint -v
//
// Skipped unless SRU_LIVE_URL is set, so the suite never touches the network by
// default.
func TestLiveEndpoint(t *testing.T) {
	base := os.Getenv("SRU_LIVE_URL")
	if base == "" {
		t.Skip("set SRU_LIVE_URL to run the live SRU smoke test")
	}
	c := NewClient(base)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.SearchRetrieve(ctx, Request{Query: `dc.title = ` + Quote("moby dick"), MaxRecords: 3})
	if err != nil {
		t.Fatalf("SearchRetrieve: %v", err)
	}
	t.Logf("hits: %d, page: %d records", resp.NumberOfRecords, len(resp.Records))
	for _, rec := range resp.Records {
		dec, err := rec.Decode()
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		t.Logf("  245$a=%q", dec.SubfieldValue("245", 'a'))
	}
}
