package sip2

import (
	"fmt"
	"strings"
	"time"
)

// SIP2 messages are lines: a two-digit command code, fixed-length fields, then
// pipe-delimited variable fields "<2-char code><data>|", terminated by a carriage
// return. With error detection enabled a message also carries a trailing
// "AY<sequence>AZ<checksum>" before the CR.

const (
	fieldDelim = "|"
	msgTerm    = "\r"
)

// checksum returns the four-uppercase-hex-digit SIP2 error-detection checksum of s:
// the 16-bit two's complement of the sum of every byte, computed over the message
// through the literal "AZ" (which the caller must already have appended). Summing
// the message bytes and the checksum value is then zero modulo 0x10000.
func checksum(s string) string {
	var sum uint32
	for i := 0; i < len(s); i++ {
		sum += uint32(s[i])
	}
	return fmt.Sprintf("%04X", (-sum)&0xFFFF)
}

// frame terminates a message for the wire, appending the AY sequence and AZ
// checksum when error detection is on. seq is the caller's rolling 0-9 sequence
// number, echoed by the ACS in its reply.
func frame(msg string, errorDetection bool, seq int) []byte {
	if !errorDetection {
		return []byte(msg + msgTerm)
	}
	body := fmt.Sprintf("%sAY%dAZ", msg, seq%10)
	return []byte(body + checksum(body) + msgTerm)
}

// parseFields splits the variable-field portion of a message into a code->value
// map. The error-detection fields AY and AZ are skipped, and the first occurrence
// of a repeated code wins (SIP2 permits repeats; callers here want the primary).
func parseFields(s string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Split(s, fieldDelim) {
		if len(part) < 2 {
			continue
		}
		code := part[:2]
		if code == "AY" || code == "AZ" {
			continue
		}
		if _, seen := fields[code]; !seen {
			fields[code] = part[2:]
		}
	}
	return fields
}

// sipDate formats a time as an 18-character SIP2 transaction date:
// YYYYMMDD, a four-character time zone (blank here), then HHMMSS.
func sipDate(t time.Time) string {
	return t.Format("20060102") + "    " + t.Format("150405")
}

// circulationStatus maps the SIP2 item circulation-status code (01-13) to its
// meaning. It is exposed through [CirculationStatusLabel] so a caller can fold the
// codes into its own availability rollup (e.g. 03/09 -> available, 04/05/07 ->
// loaned) without this package imposing one.
var circulationStatus = map[string]string{
	"01": "other/unknown",
	"02": "on order",
	"03": "available",
	"04": "charged",
	"05": "charged; not to be recalled until earliest recall date",
	"06": "in process",
	"07": "recalled",
	"08": "waiting on hold shelf",
	"09": "waiting to be re-shelved",
	"10": "in transit between library locations",
	"11": "claimed returned",
	"12": "lost",
	"13": "missing",
}

// CirculationStatusLabel returns the human-readable meaning of a SIP2 circulation
// status code, or "" for an unrecognized code.
func CirculationStatusLabel(code string) string { return circulationStatus[code] }
