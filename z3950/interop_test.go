package z3950

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

// startYazZtest spawns YAZ's test server on a free local port and returns its
// address. Skipped when YAZ is not installed, like the pymarc interop tests.
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
	return addr
}

// TestYazZtestInterop runs Initialize/Search/Present against YAZ's test server --
// the reference implementation -- over real TCP+BER, then pages the whole result
// set through the Reader.
func TestYazZtestInterop(t *testing.T) {
	c := NewClient(startYazZtest(t) + "/Default")
	c.PageSize = 5 // force paging
	ctx := context.Background()

	conn, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()
	res, err := conn.Search(ctx, Term("any", "computer"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Count == 0 {
		t.Fatal("no hits from yaz-ztest")
	}
	recs, err := conn.Present(ctx, 1, 2)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if len(recs) == 0 || recs[0].Syntax != "marc21" {
		t.Fatalf("records = %+v", recs)
	}
	if dec, err := recs[0].Decode(); err != nil || dec.SubfieldValue("245", 'a') == "" {
		t.Errorf("decode: %v", err)
	}

	all, err := codex.ReadAll(c.NewReader(ctx, Term("any", "computer")))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(all) != res.Count {
		t.Errorf("paged %d records, server reports %d hits", len(all), res.Count)
	}
}

// TestLiveTarget is an opt-in smoke test against a real Z39.50 target, e.g.
//
//	Z3950_LIVE_TARGET=lx2.loc.gov:210/LCDB go test ./z3950/ -run TestLiveTarget -v
//
// Skipped unless Z3950_LIVE_TARGET is set, so the suite never touches the
// network by default.
func TestLiveTarget(t *testing.T) {
	target := os.Getenv("Z3950_LIVE_TARGET")
	if target == "" {
		t.Skip("set Z3950_LIVE_TARGET to run the live Z39.50 smoke test")
	}
	c := NewClient(target)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()
	res, err := conn.Search(ctx, Term("title", "moby dick"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	t.Logf("hits: %d", res.Count)
	if res.Count > 0 {
		recs, err := conn.Present(ctx, 1, min(3, res.Count))
		if err != nil {
			t.Fatalf("Present: %v", err)
		}
		for _, r := range recs {
			if dec, err := r.Decode(); err == nil {
				t.Logf("  %s: 245$a=%q", r.Syntax, dec.SubfieldValue("245", 'a'))
			}
		}
	}
}
