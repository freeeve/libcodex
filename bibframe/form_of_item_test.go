package bibframe

import (
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
)

// f008 builds a 40-byte 008 with the given form-of-item byte at pos and the given
// 3-char language at 35-37, everything else blank.
func f008(pos int, form byte, lang string) string {
	b := []byte(strings.Repeat(" ", 40))
	b[pos] = form
	copy(b[35:38], lang)
	return string(b)
}

// TestFormOfItemSynthesizesCarrierMediaLanguage covers task 123: a book carrying
// format and language only in the 008 (no 337/338/041, the common Koha OAI shape)
// gets an Instance carrier/media from 008/23 form-of-item and a Work language from
// 008/35-37, with a 2-letter 639-1 code normalized to its MARC 639-2/B code.
func TestFormOfItemSynthesizesCarrierMediaLanguage(t *testing.T) {
	g := FromRecord(codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")). // /06=a book
		AddField(codex.NewControlField("008", f008(23, 'o', "en "))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))))
	if len(g.Instance.Carrier) != 1 || g.Instance.Carrier[0].Code != "cr" {
		t.Errorf("carrier = %+v, want one cr (online resource)", g.Instance.Carrier)
	}
	if len(g.Instance.Media) != 1 || g.Instance.Media[0].Code != "c" {
		t.Errorf("media = %+v, want one c (computer)", g.Instance.Media)
	}
	if len(g.Work.Languages) != 1 || g.Work.Languages[0] != "eng" {
		t.Errorf("languages = %+v, want [eng] (en normalized)", g.Work.Languages)
	}
}

// TestFormOfItemYieldsToRDA checks the gate: an explicit 338/337 wins, so the 008
// form-of-item adds nothing when the RDA fields are present.
func TestFormOfItemYieldsToRDA(t *testing.T) {
	g := FromRecord(codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("008", f008(23, 'o', "   "))).
		AddField(codex.NewDataField("337", ' ', ' ', codex.NewSubfield('b', "n"), codex.NewSubfield('2', "rdamedia"))).
		AddField(codex.NewDataField("338", ' ', ' ', codex.NewSubfield('b', "nc"), codex.NewSubfield('2', "rdacarrier"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))))
	if len(g.Instance.Carrier) != 1 || g.Instance.Carrier[0].Code != "nc" {
		t.Errorf("carrier = %+v, want only the 338 nc (008 must not override)", g.Instance.Carrier)
	}
	if len(g.Instance.Media) != 1 || g.Instance.Media[0].Code != "n" {
		t.Errorf("media = %+v, want only the 337 n", g.Instance.Media)
	}
}

// TestFormOfItemBlankIsUnmediatedVolume checks the print-book default: a blank
// form-of-item yields unmediated media and no carrier, matching m2b.
func TestFormOfItemBlankIsUnmediatedVolume(t *testing.T) {
	g := FromRecord(codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("008", f008(23, ' ', "fre"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))))
	if len(g.Instance.Media) != 1 || g.Instance.Media[0].Code != "n" {
		t.Errorf("media = %+v, want one n (unmediated)", g.Instance.Media)
	}
	if len(g.Instance.Carrier) != 0 {
		t.Errorf("carrier = %+v, want none for a blank form-of-item", g.Instance.Carrier)
	}
	if len(g.Work.Languages) != 1 || g.Work.Languages[0] != "fre" {
		t.Errorf("languages = %+v, want [fre] (3-letter passes through)", g.Work.Languages)
	}
}

// TestFormOfItemMapsReadPosition29 checks that maps/visual materials read the
// form-of-item at 008/29, not 008/23.
func TestFormOfItemMapsReadPosition29(t *testing.T) {
	f := []byte(strings.Repeat(" ", 40))
	f[23] = 'a' // would be a microform carrier if 23 were (wrongly) read
	f[29] = 'o' // the real form-of-item for a map -> online resource
	g := FromRecord(codex.NewRecord().
		SetLeader(codex.Leader("00000nem a2200000 a 4500")). // /06=e cartographic
		AddField(codex.NewControlField("008", string(f))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Map"))))
	if len(g.Instance.Carrier) != 1 || g.Instance.Carrier[0].Code != "cr" {
		t.Errorf("carrier = %+v, want cr from 008/29 (not hd from /23)", g.Instance.Carrier)
	}
}

// TestNormalizeLang pins the language normalization: 3-letter codes pass through,
// 639-1 codes map to their MARC 639-2/B code (the bibliographic variant, so sq->alb
// not sqi), and anything else is dropped.
func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"eng":  "eng", // already 639-2
		"en":   "eng",
		"fr":   "fre", // B (not fra)
		"sq":   "alb", // B (not sqi)
		"zh":   "chi", // B (not zho)
		"xx":   "",    // unknown 2-letter
		"e":    "",    // too short
		"engl": "",    // too long
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}
