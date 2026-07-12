package sip2

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeACS runs a scripted SIP2 server on one end of a net.Pipe: for each inbound
// request line it writes the next canned reply. It returns the client end as the
// Client.Dial seam and records the requests it saw.
func fakeACS(t *testing.T, replies ...string) (dial func(context.Context, string) (net.Conn, error), seen *[]string) {
	t.Helper()
	client, server := net.Pipe()
	got := &[]string{}
	go func() {
		defer server.Close()
		r := bufio.NewReader(server)
		for _, reply := range replies {
			req, err := r.ReadString('\r')
			if err != nil {
				return
			}
			*got = append(*got, strings.TrimRight(req, "\r\n"))
			if _, err := server.Write([]byte(reply + "\r")); err != nil {
				return
			}
		}
	}()
	return func(context.Context, string) (net.Conn, error) { return client, nil }, got
}

// TestItemInformationParsesResponse drives a full 17 -> 18 exchange through the
// dial seam and checks every field a discovery caller reads is parsed, including
// the 3M CS call number and the CF hold queue.
func TestItemInformationParsesResponse(t *testing.T) {
	dial, seen := fakeACS(t,
		"18"+"03"+"02"+"01"+"20240101    120000"+"AB30000123|AJStone butch blues|AHdue soon|APmain stacks|AQcentral|CSPS3556 .E446|CF2|")
	c := &Client{Address: "acs:6001", InstitutionID: "MAIN", Dial: dial}
	co, err := c.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer co.Close()
	info, err := co.ItemInformation(context.Background(), "30000123")
	if err != nil {
		t.Fatal(err)
	}
	if info.CirculationStatus != "03" || info.StatusLabel != "available" {
		t.Errorf("status = %q/%q, want 03/available", info.CirculationStatus, info.StatusLabel)
	}
	if info.SecurityMarker != "02" || info.FeeType != "01" || info.TransactionDate != "20240101    120000" {
		t.Errorf("fixed header = %+v", info)
	}
	for _, tc := range []struct{ got, want, name string }{
		{info.ItemID, "30000123", "AB"}, {info.Title, "Stone butch blues", "AJ"},
		{info.DueDate, "due soon", "AH"}, {info.CurrentLocation, "main stacks", "AP"},
		{info.PermanentLocation, "central", "AQ"}, {info.CallNumber, "PS3556 .E446", "CS"},
		{info.HoldQueueLength, "2", "CF"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	// The request is a 17 carrying the item barcode in AB and the institution AO.
	if len(*seen) != 1 || !strings.HasPrefix((*seen)[0], "17") ||
		!strings.Contains((*seen)[0], "AB30000123|") || !strings.Contains((*seen)[0], "AOMAIN|") {
		t.Errorf("request = %q", *seen)
	}
}

// TestLoginExchange checks that a configured User triggers the 93 login and that a
// 941 reply is accepted while a 940 reply is rejected.
func TestLoginExchange(t *testing.T) {
	dial, seen := fakeACS(t, "941")
	c := &Client{Address: "x", User: "term", Password: "pw", Location: "loc", Dial: dial}
	if _, err := c.Connect(context.Background()); err != nil {
		t.Fatalf("login should succeed on 941: %v", err)
	}
	if len(*seen) != 1 || !strings.HasPrefix((*seen)[0], "9300") ||
		!strings.Contains((*seen)[0], "CNterm|") || !strings.Contains((*seen)[0], "COpw|") ||
		!strings.Contains((*seen)[0], "CPloc|") {
		t.Errorf("login request = %q", *seen)
	}

	dialFail, _ := fakeACS(t, "940")
	cf := &Client{Address: "x", User: "term", Password: "pw", Dial: dialFail}
	if _, err := cf.Connect(context.Background()); err == nil {
		t.Error("login should be rejected on 940")
	}
}

// TestErrorDetectionChecksum checks the AY/AZ trailer is appended when enabled and
// that the checksum is a valid two's complement (byte sum + checksum ≡ 0 mod 2^16).
func TestErrorDetectionChecksum(t *testing.T) {
	for _, s := range []string{"", "9300CNa|CO|AY0AZ", "1720240101    120000AB1|AY3AZ"} {
		cs := checksum(s)
		if len(cs) != 4 {
			t.Fatalf("checksum(%q) = %q, want 4 hex digits", s, cs)
		}
		v, err := strconv.ParseUint(cs, 16, 32)
		if err != nil {
			t.Fatalf("checksum(%q) = %q, not hex", s, cs)
		}
		var sum uint32
		for i := 0; i < len(s); i++ {
			sum += uint32(s[i])
		}
		if (sum+uint32(v))&0xFFFF != 0 {
			t.Errorf("checksum(%q)=%s: (sum+cs) mod 0x10000 = %#x, want 0", s, cs, (sum+uint32(v))&0xFFFF)
		}
	}
	// A framed message with error detection ends in the AY/AZ trailer and a CR.
	out := string(frame("17"+sipDate(time.Unix(0, 0).UTC()), true, 5))
	if !strings.HasSuffix(out, "\r") || !strings.Contains(out, "AY5AZ") {
		t.Errorf("framed = %q, want AY5AZ...CR", out)
	}
}

// TestParseFieldsSkipsErrorDetectionAndKeepsFirst checks the variable-field parser
// drops AY/AZ and keeps the first occurrence of a repeated code.
func TestParseFieldsSkipsErrorDetectionAndKeepsFirst(t *testing.T) {
	f := parseFields("AB111|AJTitle|AB222|AY0AZFDEC")
	if f["AB"] != "111" {
		t.Errorf("AB = %q, want 111 (first wins)", f["AB"])
	}
	if _, ok := f["AY"]; ok {
		t.Error("AY should be skipped")
	}
	if _, ok := f["AZ"]; ok {
		t.Error("AZ should be skipped")
	}
}

// TestParseItemInfoRejectsShort checks a malformed 18 (or wrong code) errors rather
// than slicing out of range.
func TestParseItemInfoRejectsShort(t *testing.T) {
	if _, err := parseItemInfo("1803"); err == nil {
		t.Error("short 18 should error")
	}
	if _, err := parseItemInfo("64bad"); err == nil {
		t.Error("non-18 response should error")
	}
}

// FuzzParseItemInfo asserts the 18 parser never panics on arbitrary bytes and, when
// it succeeds, always fills the fixed header from a body of at least 24 chars.
func FuzzParseItemInfo(f *testing.F) {
	f.Add("18" + "03" + "02" + "01" + "20240101    120000" + "AB1|AJt|")
	f.Add("18")
	f.Add("")
	f.Add("18\x00\xff|AB|AY0AZ")
	f.Fuzz(func(t *testing.T, s string) {
		info, err := parseItemInfo(s)
		if err != nil {
			return
		}
		if len(info.CirculationStatus) != 2 || len(info.TransactionDate) != 18 {
			t.Errorf("parsed %q but header malformed: %+v", s, info)
		}
	})
}

// TestCirculationStatusRollup documents that the exposed table supports libcat's
// available/loaned/unavailable/unknown fold.
func TestCirculationStatusRollup(t *testing.T) {
	rollup := func(code string) string {
		switch code {
		case "03", "09":
			return "available"
		case "04", "05", "07":
			return "loaned"
		case "01":
			return "unknown"
		default:
			return "unavailable"
		}
	}
	for code := range circulationStatus {
		if CirculationStatusLabel(code) == "" {
			t.Errorf("code %s has no label", code)
		}
		if rollup(code) == "" {
			t.Errorf("code %s has no rollup", code)
		}
	}
	if rollup("03") != "available" || rollup("04") != "loaned" || rollup("12") != "unavailable" || rollup("01") != "unknown" {
		t.Error("rollup mapping wrong")
	}
}
