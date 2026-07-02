package z3950

import (
	"fmt"
	"strconv"
)

// This file encodes and decodes the Z39.50 APDUs the client speaks: Initialize,
// Search, Present and Close, written from the ASN.1 in ANSI/NISO Z39.50-1995.
// Every structure is BER with implicit context tags unless the field wraps a
// CHOICE, which takes an explicit (constructed) tag.

// PDU choice tags.
const (
	pduInitRequest     = 20
	pduInitResponse    = 21
	pduSearchRequest   = 22
	pduSearchResponse  = 23
	pduPresentRequest  = 24
	pduPresentResponse = 25
	pduClose           = 48
)

// Object identifiers.
var (
	oidBib1Attributes = []uint32{1, 2, 840, 10003, 3, 1}
	oidDiagBib1       = []uint32{1, 2, 840, 10003, 4, 1}
	oidMARC21         = []uint32{1, 2, 840, 10003, 5, 10}
	oidUNIMARC        = []uint32{1, 2, 840, 10003, 5, 1}
	oidSUTRS          = []uint32{1, 2, 840, 10003, 5, 101}
	oidOPAC           = []uint32{1, 2, 840, 10003, 5, 102}
	oidXML            = []uint32{1, 2, 840, 10003, 5, 109, 10}
)

// syntaxName folds a record-syntax OID to a short token; an unknown OID renders
// as its dotted form.
func syntaxName(oid []uint32) string {
	switch {
	case oidEqual(oid, oidMARC21):
		return "marc21"
	case oidEqual(oid, oidUNIMARC):
		return "unimarc"
	case oidEqual(oid, oidSUTRS):
		return "sutrs"
	case oidEqual(oid, oidXML):
		return "xml"
	case oidEqual(oid, oidOPAC):
		return "opac"
	}
	var b []byte
	for i, arc := range oid {
		if i > 0 {
			b = append(b, '.')
		}
		b = strconv.AppendUint(b, uint64(arc), 10)
	}
	return string(b)
}

// syntaxOID inverts syntaxName for the syntaxes the client can request.
func syntaxOID(name string) ([]uint32, error) {
	switch name {
	case "", "marc21", "usmarc":
		return oidMARC21, nil
	case "unimarc":
		return oidUNIMARC, nil
	case "xml", "marcxml":
		return oidXML, nil
	case "sutrs":
		return oidSUTRS, nil
	}
	return nil, fmt.Errorf("z3950: unknown record syntax %q", name)
}

// ---- requests ----

// encodeInitRequest renders an InitializeRequest offering protocol versions 1-3
// and the search + present options, with generous message sizes.
func encodeInitRequest() []byte {
	var body []byte
	body = appendBits(body, classContext, 3, 0, 1, 2) // protocolVersion: 1, 2, 3
	body = appendBits(body, classContext, 4, 0, 1)    // options: search, present
	body = appendInt(body, classContext, 5, 1<<20)    // preferredMessageSize
	body = appendInt(body, classContext, 6, 1<<24)    // exceptionalRecordSize
	body = appendString(body, classContext, 110, "libcodex")
	body = appendString(body, classContext, 111, "libcodex z3950")
	return appendElem(nil, classContext, true, pduInitRequest, body)
}

// encodeSearchRequest renders a SearchRequest for the given databases and RPN
// query. Piggybacked records are declined (small/medium set bounds force a
// separate Present), keeping retrieval on one code path.
func encodeSearchRequest(databases []string, syntax []uint32, rpn []byte) []byte {
	var body []byte
	body = appendInt(body, classContext, 13, 0)            // smallSetUpperBound
	body = appendInt(body, classContext, 14, 1)            // largeSetLowerBound
	body = appendInt(body, classContext, 15, 0)            // mediumSetPresentNumber
	body = appendBool(body, classContext, 16, true)        // replaceIndicator
	body = appendString(body, classContext, 17, "default") // resultSetName
	var dbs []byte
	for _, db := range databases {
		dbs = appendString(dbs, classContext, 105, db) // DatabaseName
	}
	body = appendElem(body, classContext, true, 18, dbs) // databaseNames
	body = appendOID(body, classContext, 104, syntax)    // preferredRecordSyntax
	// query [21] is an explicit tag around the Query CHOICE; type-1 [1] wraps the
	// RPNQuery: attributeSet OID then the RPN structure.
	var rpnQuery []byte
	rpnQuery = appendOID(rpnQuery, classUniversal, tagOID, oidBib1Attributes)
	rpnQuery = append(rpnQuery, rpn...)
	typed := appendElem(nil, classContext, true, 1, rpnQuery)
	body = appendElem(body, classContext, true, 21, typed)
	return appendElem(nil, classContext, true, pduSearchRequest, body)
}

// encodePresentRequest renders a PresentRequest for count records of the default
// result set starting at start (1-based), asking for full records ("F").
func encodePresentRequest(start, count int, syntax []uint32) []byte {
	var body []byte
	body = appendString(body, classContext, 31, "default") // resultSetId
	body = appendInt(body, classContext, 30, int64(start)) // resultSetStartPoint
	body = appendInt(body, classContext, 29, int64(count)) // numberOfRecordsRequested
	// recordComposition precedes preferredRecordSyntax in the SEQUENCE. simple
	// [19] is explicit around the ElementSetNames CHOICE; genericElementSetName
	// [0] carries the element-set name.
	esn := appendString(nil, classContext, 0, "F")
	body = appendElem(body, classContext, true, 19, esn)
	body = appendOID(body, classContext, 104, syntax) // preferredRecordSyntax
	return appendElem(nil, classContext, true, pduPresentRequest, body)
}

// encodeClose renders a Close with reason "finished".
func encodeClose() []byte {
	body := appendInt(nil, classContext, 211, 0)
	return appendElem(nil, classContext, true, pduClose, body)
}

// ---- responses ----

// initResponse is the subset of an InitializeResponse the client acts on.
type initResponse struct {
	result         bool
	implementation string
}

// searchResponse is the subset of a SearchResponse the client acts on.
type searchResponse struct {
	resultCount  int
	searchStatus bool
	records      []Record
	diagnostics  []Diagnostic
}

// presentResponse is the subset of a PresentResponse the client acts on.
type presentResponse struct {
	returned      int
	nextPosition  int
	presentStatus int64
	records       []Record
	diagnostics   []Diagnostic
}

// decodePDU parses one APDU and returns its tag plus the decoded body for the
// response types the client handles.
func decodePDU(b []byte) (uint32, any, error) {
	v, _, err := berParse(b)
	if err != nil {
		return 0, nil, err
	}
	if v.class != classContext {
		return 0, nil, fmt.Errorf("z3950: unexpected PDU class %#x", v.class)
	}
	switch v.tag {
	case pduInitResponse:
		r, err := decodeInitResponse(v)
		return v.tag, r, err
	case pduSearchResponse:
		r, err := decodeSearchResponse(v)
		return v.tag, r, err
	case pduPresentResponse:
		r, err := decodePresentResponse(v)
		return v.tag, r, err
	case pduClose:
		return v.tag, nil, nil
	}
	return v.tag, nil, fmt.Errorf("z3950: unexpected PDU tag %d", v.tag)
}

func decodeInitResponse(v berValue) (*initResponse, error) {
	fields, err := v.children()
	if err != nil {
		return nil, err
	}
	out := &initResponse{}
	for _, f := range fields {
		if f.class != classContext {
			continue
		}
		switch f.tag {
		case 12:
			out.result = f.boolVal()
		case 111:
			out.implementation = f.stringVal()
		}
	}
	return out, nil
}

func decodeSearchResponse(v berValue) (*searchResponse, error) {
	fields, err := v.children()
	if err != nil {
		return nil, err
	}
	out := &searchResponse{}
	for _, f := range fields {
		if f.class != classContext {
			continue
		}
		switch f.tag {
		case 23:
			n, err := f.intVal()
			if err != nil {
				return nil, err
			}
			out.resultCount = int(n)
		case 22:
			out.searchStatus = f.boolVal()
		case 28, 130, 205:
			out.records, out.diagnostics, err = parseRecords(f)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func decodePresentResponse(v berValue) (*presentResponse, error) {
	fields, err := v.children()
	if err != nil {
		return nil, err
	}
	out := &presentResponse{}
	for _, f := range fields {
		if f.class != classContext {
			continue
		}
		switch f.tag {
		case 24:
			n, err := f.intVal()
			if err != nil {
				return nil, err
			}
			out.returned = int(n)
		case 25:
			n, err := f.intVal()
			if err != nil {
				return nil, err
			}
			out.nextPosition = int(n)
		case 27:
			out.presentStatus, _ = f.intVal()
		case 28, 130, 205:
			out.records, out.diagnostics, err = parseRecords(f)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// parseRecords handles the Records CHOICE: responseRecords [28] (a list of
// NamePlusRecord), nonSurrogateDiagnostic [130] (one DefaultDiagFormat), or
// multipleNonSurDiagnostics [205].
func parseRecords(v berValue) ([]Record, []Diagnostic, error) {
	switch v.tag {
	case 130:
		d, err := parseDefaultDiag(v)
		if err != nil {
			return nil, nil, err
		}
		return nil, []Diagnostic{d}, nil
	case 205:
		items, err := v.children()
		if err != nil {
			return nil, nil, err
		}
		var diags []Diagnostic
		for _, it := range items {
			d, err := parseDiagRec(it)
			if err != nil {
				return nil, nil, err
			}
			diags = append(diags, d)
		}
		return nil, diags, nil
	}
	items, err := v.children()
	if err != nil {
		return nil, nil, err
	}
	var recs []Record
	for _, it := range items {
		rec, err := parseNamePlusRecord(it)
		if err != nil {
			return nil, nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil, nil
}

// parseNamePlusRecord unwraps NamePlusRecord: an optional database name [0],
// then record [1] holding either retrievalRecord [1] (an EXTERNAL) or
// surrogateDiagnostic [2].
func parseNamePlusRecord(v berValue) (Record, error) {
	fields, err := v.children()
	if err != nil {
		return Record{}, err
	}
	for _, f := range fields {
		if f.class != classContext || f.tag != 1 {
			continue
		}
		inner, err := f.children()
		if err != nil {
			return Record{}, err
		}
		if len(inner) == 0 {
			break
		}
		switch c := inner[0]; c.tag {
		case 1: // retrievalRecord: explicit tag around an EXTERNAL
			ext, err := c.children()
			if err != nil {
				return Record{}, err
			}
			if len(ext) == 0 {
				return Record{}, fmt.Errorf("z3950: empty retrievalRecord")
			}
			return parseExternal(ext[0])
		case 2: // surrogateDiagnostic
			diagChildren, err := c.children()
			if err != nil {
				return Record{}, err
			}
			if len(diagChildren) == 0 {
				return Record{}, fmt.Errorf("z3950: empty surrogateDiagnostic")
			}
			d, err := parseDiagRec(diagChildren[0])
			if err != nil {
				return Record{}, err
			}
			return Record{Diag: &d}, nil
		}
	}
	return Record{}, fmt.Errorf("z3950: NamePlusRecord without record")
}

// parseExternal unwraps an EXTERNAL: the direct-reference OID names the record
// syntax and the encoding choice carries the payload (octet-aligned [1] for MARC,
// single-ASN1-type [0] for SUTRS text).
func parseExternal(v berValue) (Record, error) {
	if v.class != classUniversal || v.tag != tagExternal {
		return Record{}, fmt.Errorf("z3950: expected EXTERNAL, got class %#x tag %d", v.class, v.tag)
	}
	fields, err := v.children()
	if err != nil {
		return Record{}, err
	}
	rec := Record{}
	for _, f := range fields {
		switch {
		case f.class == classUniversal && f.tag == tagOID:
			oid, err := f.oidVal()
			if err != nil {
				return Record{}, err
			}
			rec.Syntax = syntaxName(oid)
		case f.class == classContext && f.tag == 1: // octet-aligned
			rec.Data = f.content
		case f.class == classContext && f.tag == 0: // single-ASN1-type
			if f.constructed {
				if inner, err := f.children(); err == nil && len(inner) == 1 {
					rec.Data = inner[0].content
					continue
				}
			}
			rec.Data = f.content
		}
	}
	if rec.Syntax == "" && rec.Data == nil {
		return Record{}, fmt.Errorf("z3950: EXTERNAL without payload")
	}
	return rec, nil
}

// parseDiagRec handles the DiagRec CHOICE, of which only defaultFormat (a plain
// SEQUENCE) is produced by the servers this client targets.
func parseDiagRec(v berValue) (Diagnostic, error) {
	if v.class == classUniversal && v.tag == tagSequence {
		return parseDefaultDiag(v)
	}
	return Diagnostic{Message: "externally defined diagnostic"}, nil
}

// parseDefaultDiag decodes DefaultDiagFormat: diagnostic set OID, condition
// code, and the human-readable addinfo string.
func parseDefaultDiag(v berValue) (Diagnostic, error) {
	fields, err := v.children()
	if err != nil {
		return Diagnostic{}, err
	}
	d := Diagnostic{}
	for _, f := range fields {
		switch {
		case f.class == classUniversal && f.tag == tagOID:
			if oid, err := f.oidVal(); err == nil && oidEqual(oid, oidDiagBib1) {
				d.Set = "bib-1"
			}
		case f.class == classUniversal && f.tag == tagInteger:
			n, err := f.intVal()
			if err != nil {
				return Diagnostic{}, err
			}
			d.Condition = int(n)
		case f.class == classUniversal && (f.tag == tagVisibleString || f.tag == tagGeneralString):
			d.Message = f.stringVal()
		case f.class == classContext: // v2Addinfo/v3Addinfo under an explicit tag
			d.Message = f.stringVal()
		}
	}
	return d, nil
}
