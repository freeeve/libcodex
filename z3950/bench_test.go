package z3950

import "testing"

// BenchmarkDecodePresentResponse measures decoding a five-record Present
// response, the hot path of every page the Reader fetches.
func BenchmarkDecodePresentResponse(b *testing.B) {
	rec := codexSampleMARC()
	var records [][]byte
	for range 5 {
		records = append(records, marcNamePlusRecordBench(rec))
	}
	pdu := encodePresentResponseBench(records)
	b.ReportAllocs()
	b.SetBytes(int64(len(pdu)))
	for b.Loop() {
		if _, _, err := decodePDU(pdu); err != nil {
			b.Fatal(err)
		}
	}
}

// Bench-local encoders mirror the test helpers without a *testing.T.

func codexSampleMARC() []byte {
	// A minimal ISO 2709 record body is irrelevant to APDU decode speed; use a
	// fixed payload.
	return []byte("00074nam a2200049 a 4500001000500000245001000005\x1e0001\x1e10\x1faTitle\x1e\x1d")
}

func marcNamePlusRecordBench(marc []byte) []byte {
	var ext []byte
	ext = appendOID(ext, classUniversal, tagOID, oidMARC21)
	ext = appendElem(ext, classContext, false, 1, marc)
	extEl := appendElem(nil, classUniversal, true, tagExternal, ext)
	retr := appendElem(nil, classContext, true, 1, extEl)
	recCh := appendElem(nil, classContext, true, 1, retr)
	return appendElem(nil, classUniversal, true, tagSequence, recCh)
}

func encodePresentResponseBench(records [][]byte) []byte {
	var list []byte
	for _, r := range records {
		list = append(list, r...)
	}
	var body []byte
	body = appendInt(body, classContext, 24, int64(len(records)))
	body = appendInt(body, classContext, 25, int64(len(records)+1))
	body = appendInt(body, classContext, 27, 0)
	body = appendElem(body, classContext, true, 28, list)
	return appendElem(nil, classContext, true, pduPresentResponse, body)
}
