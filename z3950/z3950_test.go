package z3950

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	codex "github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

// sampleMARC builds one ISO 2709 record with the given control number and title.
func sampleMARC(t *testing.T, id, title string) []byte {
	t.Helper()
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", id)).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', title)))
	b, err := iso2709.Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ---- server-side PDU encoders (test-only, built on the same BER primitives) ----

func encodeInitResponseT(result bool) []byte {
	var body []byte
	body = appendBits(body, classContext, 3, 0, 1, 2)
	body = appendBits(body, classContext, 4, 0, 1)
	body = appendInt(body, classContext, 5, 1<<20)
	body = appendInt(body, classContext, 6, 1<<24)
	body = appendBool(body, classContext, 12, result)
	body = appendString(body, classContext, 111, "fake server")
	return appendElem(nil, classContext, true, pduInitResponse, body)
}

func encodeSearchResponseT(count int, diag *Diagnostic) []byte {
	var body []byte
	body = appendInt(body, classContext, 23, int64(count))
	body = appendInt(body, classContext, 24, 0)
	body = appendInt(body, classContext, 25, 1)
	body = appendBool(body, classContext, 22, diag == nil)
	if diag != nil {
		body = appendElem(body, classContext, true, 130, defaultDiagT(*diag))
	}
	return appendElem(nil, classContext, true, pduSearchResponse, body)
}

// defaultDiagT renders DefaultDiagFormat content (OID, condition, addinfo).
func defaultDiagT(d Diagnostic) []byte {
	var b []byte
	b = appendOID(b, classUniversal, tagOID, oidDiagBib1)
	b = appendInt(b, classUniversal, tagInteger, int64(d.Condition))
	b = appendString(b, classUniversal, tagVisibleString, d.Message)
	return b
}

// marcNamePlusRecordT wraps an ISO 2709 payload as NamePlusRecord -> record [1]
// -> retrievalRecord [1] -> EXTERNAL(marc21, octet-aligned).
func marcNamePlusRecordT(marc []byte) []byte {
	var ext []byte
	ext = appendOID(ext, classUniversal, tagOID, oidMARC21)
	ext = appendElem(ext, classContext, false, 1, marc)
	extEl := appendElem(nil, classUniversal, true, tagExternal, ext)
	retr := appendElem(nil, classContext, true, 1, extEl)
	recCh := appendElem(nil, classContext, true, 1, retr)
	return appendElem(nil, classUniversal, true, tagSequence, recCh)
}

// diagNamePlusRecordT wraps a surrogate diagnostic as NamePlusRecord.
func diagNamePlusRecordT(d Diagnostic) []byte {
	diagSeq := appendElem(nil, classUniversal, true, tagSequence, defaultDiagT(d))
	surr := appendElem(nil, classContext, true, 2, diagSeq)
	recCh := appendElem(nil, classContext, true, 1, surr)
	return appendElem(nil, classUniversal, true, tagSequence, recCh)
}

func encodePresentResponseT(records [][]byte, next int) []byte {
	var list []byte
	for _, r := range records {
		list = append(list, r...)
	}
	var body []byte
	body = appendInt(body, classContext, 24, int64(len(records)))
	body = appendInt(body, classContext, 25, int64(next))
	body = appendInt(body, classContext, 27, 0)
	body = appendElem(body, classContext, true, 28, list)
	return appendElem(nil, classContext, true, pduPresentResponse, body)
}

// fakeServer answers Initialize/Search/Present from canned data: hits is the
// reported result count, pages the NamePlusRecord payloads by 1-based position.
type fakeServer struct {
	initOK     bool
	hits       int
	searchDiag *Diagnostic
	records    [][]byte // one encoded NamePlusRecord per result-set position
}

// start listens on a loopback port and serves one connection per accept.
func (s *fakeServer) start(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return l.Addr().String()
}

func (s *fakeServer) serve(conn net.Conn) {
	defer conn.Close()
	var buf []byte
	chunk := make([]byte, 4096)
	for {
		total, err := berSize(buf)
		if err == errIncomplete || len(buf) == 0 {
			n, rerr := conn.Read(chunk)
			if n > 0 {
				buf = append(buf, chunk[:n]...)
				continue
			}
			if rerr != nil {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		pdu := buf[:total]
		buf = buf[total:]
		v, _, err := berParse(pdu)
		if err != nil {
			return
		}
		switch v.tag {
		case pduInitRequest:
			conn.Write(encodeInitResponseT(s.initOK))
		case pduSearchRequest:
			conn.Write(encodeSearchResponseT(s.hits, s.searchDiag))
		case pduPresentRequest:
			start, count := presentRange(v)
			end := min(start-1+count, len(s.records))
			var page [][]byte
			if start >= 1 && start <= end {
				page = s.records[start-1 : end]
			}
			conn.Write(encodePresentResponseT(page, end+1))
		case pduClose:
			return
		}
	}
}

// presentRange extracts resultSetStartPoint and numberOfRecordsRequested from a
// PresentRequest.
func presentRange(v berValue) (start, count int) {
	fields, _ := v.children()
	for _, f := range fields {
		if f.class != classContext {
			continue
		}
		switch f.tag {
		case 30:
			n, _ := f.intVal()
			start = int(n)
		case 29:
			n, _ := f.intVal()
			count = int(n)
		}
	}
	return start, count
}

// ---- tests ----

// TestConnectSearchPresent runs the whole session against the fake server and
// decodes the presented MARC21 record.
func TestConnectSearchPresent(t *testing.T) {
	srv := &fakeServer{initOK: true, hits: 1,
		records: [][]byte{marcNamePlusRecordT(sampleMARC(t, "0001", "Moby Dick"))}}
	c := NewClient(srv.start(t) + "/biblios")
	ctx := context.Background()

	conn, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()
	res, err := conn.Search(ctx, Term("title", "moby"))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Count != 1 {
		t.Fatalf("count = %d, want 1", res.Count)
	}
	recs, err := conn.Present(ctx, 1, 10)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if len(recs) != 1 || recs[0].Syntax != "marc21" {
		t.Fatalf("records = %+v", recs)
	}
	dec, err := recs[0].Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := dec.SubfieldValue("245", 'a'); got != "Moby Dick" {
		t.Errorf("245$a = %q", got)
	}
	if got := dec.ControlField("001"); got != "0001" {
		t.Errorf("001 = %q", got)
	}
}

// TestReaderPaging checks the Reader presents in pages, yields every record in
// order, then a sticky io.EOF, and closes its connection.
func TestReaderPaging(t *testing.T) {
	var records [][]byte
	for i := 1; i <= 5; i++ {
		records = append(records, marcNamePlusRecordT(sampleMARC(t, fmt.Sprintf("%04d", i), fmt.Sprintf("Title %d", i))))
	}
	srv := &fakeServer{initOK: true, hits: 5, records: records}
	c := NewClient(srv.start(t) + "/biblios")
	c.PageSize = 2 // force three Present round trips

	rd := c.NewReader(context.Background(), Term("any", "x"))
	recs, err := codex.ReadAll(rd)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) != 5 {
		t.Fatalf("read %d records, want 5", len(recs))
	}
	for i, rec := range recs {
		if got, want := rec.SubfieldValue("245", 'a'), fmt.Sprintf("Title %d", i+1); got != want {
			t.Errorf("record %d: 245$a = %q, want %q", i, got, want)
		}
	}
	if _, err := rd.Read(); err != io.EOF {
		t.Errorf("Read after end = %v, want io.EOF", err)
	}
}

// TestSearchDiagnostic checks a failed search surfaces as *DiagnosticsError from
// both Search and the Reader.
func TestSearchDiagnostic(t *testing.T) {
	srv := &fakeServer{initOK: true,
		searchDiag: &Diagnostic{Condition: 114, Message: "Unsupported use attribute"}}
	c := NewClient(srv.start(t) + "/biblios")
	ctx := context.Background()

	conn, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()
	_, err = conn.Search(ctx, Term("title", "x"))
	var de *DiagnosticsError
	if !errors.As(err, &de) || de.Diagnostics[0].Condition != 114 {
		t.Fatalf("Search err = %v, want DiagnosticsError condition 114", err)
	}

	_, rerr := c.NewReader(ctx, Term("title", "x")).Read()
	if !errors.As(rerr, &de) {
		t.Errorf("Reader err = %v, want DiagnosticsError", rerr)
	}
}

// TestSurrogateDiagnosticSkipped checks that an undeliverable record (surrogate
// diagnostic) is skipped by the Reader while the deliverable one comes through.
func TestSurrogateDiagnosticSkipped(t *testing.T) {
	srv := &fakeServer{initOK: true, hits: 2, records: [][]byte{
		diagNamePlusRecordT(Diagnostic{Condition: 25, Message: "record unavailable"}),
		marcNamePlusRecordT(sampleMARC(t, "0002", "Survivor")),
	}}
	c := NewClient(srv.start(t) + "/biblios")

	recs, err := codex.ReadAll(c.NewReader(context.Background(), Term("any", "x")))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(recs) != 1 || recs[0].SubfieldValue("245", 'a') != "Survivor" {
		t.Fatalf("records = %+v, want just Survivor", recs)
	}
}

// TestInitRejected checks that a refused Initialize fails Connect.
func TestInitRejected(t *testing.T) {
	srv := &fakeServer{initOK: false}
	c := NewClient(srv.start(t) + "/biblios")
	if _, err := c.Connect(context.Background()); err == nil {
		t.Fatal("Connect should fail when the server rejects initialization")
	}
}

// TestQueryErrors covers builder validation.
func TestQueryErrors(t *testing.T) {
	if _, err := Term("nonsense", "x").rpn(); err == nil {
		t.Error("unknown access point should error")
	}
	if _, err := Term("title", "").rpn(); err == nil {
		t.Error("empty term should error")
	}
	if _, err := And(Term("title", "a"), Term("author", "b")).rpn(); err != nil {
		t.Errorf("boolean query: %v", err)
	}
}

// TestSyntaxNames covers the OID <-> name mapping both ways.
func TestSyntaxNames(t *testing.T) {
	if syntaxName(oidMARC21) != "marc21" || syntaxName(oidXML) != "xml" || syntaxName(oidSUTRS) != "sutrs" {
		t.Error("known OIDs should map to short names")
	}
	if got := syntaxName([]uint32{1, 2, 3}); got != "1.2.3" {
		t.Errorf("unknown OID = %q, want dotted form", got)
	}
	if _, err := syntaxOID("nonsense"); err == nil {
		t.Error("unknown syntax name should error")
	}
}

// FuzzDecodePDU asserts the APDU decoder never panics on arbitrary bytes.
func FuzzDecodePDU(f *testing.F) {
	f.Add(encodeInitResponseT(true))
	f.Add(encodeSearchResponseT(23, nil))
	f.Add(encodePresentResponseT(nil, 1))
	f.Fuzz(func(_ *testing.T, data []byte) {
		decodePDU(data)
	})
}
