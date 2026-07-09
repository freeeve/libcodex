package bibframe

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/mrk"
)

// This file is the crosswalk's measured loss gate, mirroring the downstream
// libcat fidelity table (its roundtrip_test.go): a fully populated record
// round-trips through every BIBFRAME serialization and each MARC tag is
// asserted to survive, transform, or stay lost exactly as documented. When new
// crosswalk work makes a "lost" tag survive, the stale guard fails, forcing the
// table (and downstream's) to move it.

// coreTags are the tags the crosswalk round-trips: present in the source record
// implies present after Encode -> Decode.
var coreTags = []string{
	"001", "006", "007", "008",
	"010", "020", "022", "024", "040", "041", "050", "072", "082",
	"100", "130", "240", "245", "246", "250", "260", "300", "306",
	// 264 _4 (copyright) survives via bf:copyrightDate; provision-typed 264s
	// (ind2 0-3) collapse into 260 by documented convention.
	"264",
	"336", "337", "338", "347", "490",
	"500", "504", "505", "511", "520", "521", "533", "538", "546",
	"600", "610", "611", "650", "651", "655",
	"700", "710", "711",
	"773", "776", "780", "785", "856",
}

// transformedTags round-trip into a different tag by documented convention.
var transformedTags = map[string]string{
	"037": "024", // acquisition source -> generic scheme identifier
	"084": "072", // other classification shares 072's source-qualified shape (081 non-goal)
}

// lostTags are knowingly dropped by the crosswalk (the stale guard: if one
// starts surviving, move it to coreTags and update downstream's table).
// 003/005 are carried only as AdminMetadata provenance, deliberately not
// reverse-crosswalked; 310 (frequency) is simply unimplemented.
var lostTags = []string{"003", "005", "310"}

// A control field can survive the round-trip present but hollow: the tag tables
// above compare presence, so an 008 that comes back with every position blank
// passes them. That is exactly the shape of the bug task 103 fixed, and only a
// dedicated test caught it. controlClaims closes the gap by naming, per control
// field, the positions the reverse crosswalk claims to reconstruct.
//
// The claim is checked both ways: a claimed position must come back with the
// source's value, and an unclaimed position must come back blank. The second half
// is the stale guard -- when new crosswalk work populates a position, this table
// must move, the way lostTags must move when a tag starts surviving.
//
// 001 is compared whole and handled separately. 003/005 are lostTags: carried as
// AdminMetadata provenance and deliberately not reverse-crosswalked.
type claim struct {
	name       string
	start, end int // half-open byte range within the control field
}

var controlClaims = map[string][]claim{
	// codedFields rebuilds 006 from an electronic media type: position 00 only.
	"006": {{"00 form of material", 0, 1}},
	// codedFields rebuilds 007 from a carrier: category + specific material designation.
	"007": {{"00 category", 0, 1}, {"01 specific material designation", 1, 2}},
	// control008 renders exactly the positions FromRecord reads back out (task 103).
	"008": {
		{"06 date type", 6, 7},
		{"07-10 date 1", 7, 11},
		{"15-17 place", 15, 18},
		{"35-37 language", 35, 38},
	},
}

// at returns the half-open byte range of s, padded with blanks when s is short, so
// a claim never panics on a truncated control field.
func at(s string, start, end int) string {
	b := []byte(strings.Repeat(" ", end-start))
	for i := start; i < end && i < len(s); i++ {
		b[i-start] = s[i]
	}
	return string(b)
}

// assertControlFields checks every controlClaims entry against a round-tripped
// record: claimed positions carry the source's value, unclaimed positions are
// blank. A control field the source never had, or that the crosswalk drops
// entirely, is the tag tables' business and is skipped here.
func assertControlFields(t *testing.T, src, got *codex.Record) {
	t.Helper()
	if s, g := src.ControlField("001"), got.ControlField("001"); s != g {
		t.Errorf("001 = %q, want %q", g, s)
	}
	for _, tag := range slices.Sorted(maps.Keys(controlClaims)) {
		s, g := src.ControlField(tag), got.ControlField(tag)
		if s == "" || g == "" {
			continue
		}
		claimed := map[int]bool{}
		for _, c := range controlClaims[tag] {
			for i := c.start; i < c.end; i++ {
				claimed[i] = true
			}
			if want, have := at(s, c.start, c.end), at(g, c.start, c.end); have != want {
				t.Errorf("%s/%s = %q, want %q (in %q, out %q)", tag, c.name, have, want, s, g)
			}
		}
		for i := range len(g) {
			if !claimed[i] && g[i] != ' ' {
				t.Errorf("%s position %02d = %q, which controlClaims says the crosswalk cannot reconstruct -- "+
					"if new work populates it, add it to the table (out %q)", tag, i, g[i], g)
			}
		}
	}
}

// kitchenSink builds a record populating every tag the crosswalk knows plus the
// transformed and lost ones, with repeats where fields are repeatable.
func kitchenSink() *codex.Record {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000cam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "sink0001")).
		AddField(codex.NewControlField("003", "DLC")).
		AddField(codex.NewControlField("005", "20260702120000.0")).
		AddField(codex.NewControlField("006", "m                 ")).
		AddField(codex.NewControlField("007", "cr")).
		AddField(codex.NewControlField("008", "920219s1993    nyua                eng  "))
	add := func(tag string, ind1, ind2 byte, subs ...codex.Subfield) {
		rec.AddField(codex.NewDataField(tag, ind1, ind2, subs...))
	}
	sf := codex.NewSubfield
	add("010", ' ', ' ', sf('a', "   92005291 "))
	add("020", ' ', ' ', sf('a', "0786803525"), sf('q', "trade paperback"))
	add("022", ' ', ' ', sf('a', "1234-5678"))
	add("024", '3', ' ', sf('a', "9780786803521"))
	add("037", ' ', ' ', sf('a', "OD-12345"), sf('b', "OverDrive, Inc."))
	add("040", ' ', ' ', sf('a', "DLC"), sf('c', "DLC"), sf('e', "rda"))
	add("041", '1', ' ', sf('a', "eng"), sf('a', "fre"), sf('h', "ger"))
	add("050", '0', '0', sf('a', "PS3556"), sf('b', ".E446 1993"))
	add("072", ' ', '7', sf('a', "FIC"), sf('2', "bisacsh"))
	add("082", '0', '0', sf('a', "813.54"), sf('2', "20"))
	add("084", ' ', ' ', sf('a', "FIC027000"), sf('2', "bisacsh"))
	add("100", '1', ' ', sf('a', "Feinberg, Leslie,"), sf('d', "1949-2014."), sf('4', "aut"))
	add("240", '1', '0', sf('a', "Works."), sf('n', "no. 1,"), sf('p', "Novels"))
	add("245", '1', '0', sf('a', "Stone butch blues :"), sf('b', "a novel /"), sf('c', "Leslie Feinberg."))
	add("246", '1', '1', sf('a', "Blues de pierre"))
	add("246", '1', '4', sf('a', "Cover title"))
	add("250", ' ', ' ', sf('a', "First edition."))
	add("260", ' ', ' ', sf('a', "Ithaca, N.Y. :"), sf('b', "Firebrand Books,"), sf('c', "1993."))
	add("264", ' ', '4', sf('c', "©1993"))
	add("300", ' ', ' ', sf('a', "301 pages ;"), sf('c', "22 cm"))
	add("306", ' ', ' ', sf('a', "013000"))
	add("310", ' ', ' ', sf('a', "Monthly"))
	add("336", ' ', ' ', sf('a', "text"), sf('b', "txt"), sf('2', "rdacontent"))
	add("337", ' ', ' ', sf('a', "unmediated"), sf('b', "n"), sf('2', "rdamedia"))
	add("338", ' ', ' ', sf('a', "volume"), sf('b', "nc"), sf('2', "rdacarrier"))
	add("347", ' ', ' ', sf('a', "text file"), sf('b', "EPUB"), sf('2', "rda"))
	add("490", '0', ' ', sf('a', "Firebrand fiction ;"), sf('v', "bk. 2"))
	add("500", ' ', ' ', sf('a', "A general note."))
	add("504", ' ', ' ', sf('a', "Includes bibliographical references."))
	add("505", '0', ' ', sf('a', "Part one -- Part two."))
	add("511", '0', ' ', sf('a', "Read by the author."))
	add("520", ' ', ' ', sf('a', "A novel about gender and identity."))
	add("521", ' ', ' ', sf('a', "Adult."))
	add("533", ' ', ' ', sf('a', "Electronic reproduction."), sf('b', "Cleveland :"), sf('c', "OverDrive, Inc."))
	add("538", ' ', ' ', sf('a', "Requires OverDrive Read."))
	add("546", ' ', ' ', sf('a', "In English, translated from the German."))
	add("600", '1', '0', sf('a', "Feinberg, Leslie,"), sf('d', "1949-2014."))
	add("610", '2', '0', sf('a', "Firebrand Books."))
	add("611", '2', '0', sf('a', "Stonewall Riots Anniversary."))
	add("650", ' ', '0', sf('a', "Lesbians"), sf('x', "Fiction."))
	add("650", ' ', '0', sf('a', "Gender identity"), sf('x', "Fiction."))
	add("651", ' ', '0', sf('a', "New York (State)"), sf('x', "Fiction."))
	add("655", ' ', '7', sf('a', "Bildungsromans."), sf('2', "lcgft"))
	add("700", '1', ' ', sf('a', "Editor, An,"), sf('4', "edt"))
	add("700", '1', ' ', sf('a', "Homer,"), sf('t', "Odyssey."))
	add("710", '2', ' ', sf('a', "Some Body,"), sf('t', "Annual report.")) // second name-title: repeated bf:relatedTo must survive JSON-LD
	add("710", '2', ' ', sf('a', "Firebrand Books."))
	add("711", '2', ' ', sf('a', "Some Conference."))
	add("773", '0', ' ', sf('t', "Host anthology"))
	add("776", '0', '8', sf('c', "Original"), sf('z', "9780786803521"))
	add("780", '0', '0', sf('t', "Preceding title"), sf('x', "0000-1111"))
	add("785", '0', '0', sf('t', "Succeeding title"))
	add("856", '4', '0', sf('u', "https://example.org/item"))
	return rec
}

// bfFormats are the BIBFRAME serializations the matrix crosses.
var bfFormats = map[string]func(*codex.Record) ([]byte, error){
	"rdfxml":   Encode,
	"jsonld":   EncodeJSONLD,
	"turtle":   EncodeTurtle,
	"ntriples": EncodeNTriples,
}

// tagSet collects the distinct tags of a record.
func tagSet(r *codex.Record) map[string]bool {
	set := map[string]bool{}
	for _, f := range r.Fields() {
		set[f.Tag] = true
	}
	return set
}

// gate round-trips one record through one serialization and returns the decoded
// record.
func gate(t *testing.T, rec *codex.Record, encode func(*codex.Record) ([]byte, error)) *codex.Record {
	t.Helper()
	b, err := encode(rec)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	recs, err := Decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("decoded %d records, want 1", len(recs))
	}
	return recs[0]
}

// TestLossGateKitchenSink asserts, across every BIBFRAME serialization, that
// each coreTag survives the round-trip, each transformed tag lands on its
// documented target, each lost tag stays lost (the stale guard), and each
// control field comes back with the positions controlClaims says it does -- a
// surviving tag is not the same as a surviving value.
func TestLossGateKitchenSink(t *testing.T) {
	sink := kitchenSink()
	src := tagSet(sink)
	for _, tag := range coreTags {
		if tag != "130" && !src[tag] { // 130 is exercised by real data (a record cannot carry both 100 and 130)
			t.Fatalf("kitchen sink is missing core tag %s", tag)
		}
	}
	for name, encode := range bfFormats {
		t.Run(name, func(t *testing.T) {
			decoded := gate(t, sink, encode)
			assertControlFields(t, sink, decoded)
			got := tagSet(decoded)
			for _, tag := range coreTags {
				if src[tag] && !got[tag] {
					t.Errorf("core tag %s lost", tag)
				}
			}
			for from, to := range transformedTags {
				if !got[to] {
					t.Errorf("transformed tag %s -> %s: target missing", from, to)
				}
				if got[from] {
					t.Errorf("transformed tag %s still present, expected conversion to %s", from, to)
				}
			}
			for _, tag := range lostTags {
				if got[tag] {
					t.Errorf("stale loss table: tag %s now survives -- move it to coreTags and update the downstream fidelity table", tag)
				}
			}
		})
	}
}

// TestLossGateRealData sweeps the real LC sample records through every BIBFRAME
// serialization: any core tag present in a source record must survive.
func TestLossGateRealData(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "realdata", "*.marcxml"))
	if err != nil || len(files) == 0 {
		t.Skipf("no realdata samples (%v)", err)
	}
	core := map[string]bool{}
	for _, tag := range coreTags {
		core[tag] = true
	}
	for _, path := range files {
		recs, err := marcxml.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, rec := range recs {
			src := tagSet(rec)
			for name, encode := range bfFormats {
				got := tagSet(gate(t, rec, encode))
				for tag := range src {
					if !core[tag] || got[tag] {
						continue
					}
					if tag == "264" && got["260"] {
						continue // provision-typed 264s collapse into 260 by convention
					}
					if tag == "007" && !mappable007(rec) {
						continue // only the sound/computer/video categories reconstruct (tasks/082)
					}
					t.Errorf("%s [%s]: core tag %s lost", filepath.Base(path), name, tag)
				}
			}
		}
	}
}

// mappable007 reports whether any of a record's 007 fields is in a category the
// crosswalk reconstructs (sound/computer/video via the carrier correlation).
func mappable007(r *codex.Record) bool {
	for _, f := range r.Fields() {
		if f.Tag == "007" && len(f.Value) >= 2 {
			if _, ok := carrierFor007(f.Value[:2]); ok {
				return true
			}
		}
	}
	return false
}

// TestKitchenSinkLosslessCodecs runs the sink through the lossless codecs:
// canonicalized once through iso2709 (which recomputes the leader's computed
// bytes), every codec must reproduce the record exactly.
func TestKitchenSinkLosslessCodecs(t *testing.T) {
	b, err := iso2709.Encode(kitchenSink())
	if err != nil {
		t.Fatal(err)
	}
	canonical, _, err := iso2709.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	codecs := map[string]struct {
		enc func(*codex.Record) ([]byte, error)
		dec func([]byte) (*codex.Record, error)
	}{
		"iso2709":  {iso2709.Encode, func(b []byte) (*codex.Record, error) { r, _, err := iso2709.Decode(b); return r, err }},
		"marcxml":  {marcxml.Encode, marcxml.Decode},
		"marcjson": {marcjson.Encode, marcjson.Decode},
		"mrk":      {mrk.Encode, mrk.Decode},
	}
	for name, c := range codecs {
		t.Run(name, func(t *testing.T) {
			b, err := c.enc(canonical)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := c.dec(b)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(got, canonical) {
				t.Errorf("lossless round-trip differs:\n got %+v\nwant %+v", got, canonical)
			}
		})
	}
}
