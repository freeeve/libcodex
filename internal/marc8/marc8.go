// Package marc8 decodes MARC-8 byte sequences to UTF-8 for the common Western
// subset: Basic Latin (ASCII, the default G0 set) and ANSEL Extended Latin (the
// default G1 set), including combining diacritics. It is shared by the MARC
// serialization codecs that may carry MARC-8 data (e.g. iso2709, mrk).
//
// In MARC-8 a combining diacritic is stored BEFORE its base character, the
// reverse of Unicode, where the combining mark follows the base. This decoder
// buffers pending marks and emits them after the base character, composing the
// base and the first mark to a precomposed (NFC) code point when one exists.
//
// Out of scope: EACC/CJK, Cyrillic, Greek, Hebrew, Arabic, and the
// subscript/superscript/Greek-symbol sets. Their escape designations are
// recognized only well enough to skip the escape bytes and pass subsequent
// bytes through best-effort (as Latin-1) without crashing.
package marc8

import (
	"fmt"
	"strings"
)

// escape (0x1b) introduces a MARC-8 character-set designation sequence.
const escape = 0x1b

// anselGraphic maps ANSEL (Extended Latin, G1) spacing graphic bytes to their
// Unicode code points.
var anselGraphic = map[byte]rune{
	0xA1: 0x0141, // Ł
	0xA2: 0x00D8, // Ø
	0xA3: 0x0110, // Đ
	0xA4: 0x00DE, // Þ
	0xA5: 0x00C6, // Æ
	0xA6: 0x0152, // Œ
	0xA7: 0x02B9, // ʹ modifier prime
	0xA8: 0x00B7, // · middle dot
	0xA9: 0x266D, // ♭ music flat
	0xAA: 0x00AE, // ®
	0xAB: 0x00B1, // ±
	0xAC: 0x01A0, // Ơ
	0xAD: 0x01AF, // Ư
	0xAE: 0x02BC, // ʼ alif (modifier letter apostrophe; LoC remapped from 02BE in 2005)
	0xB0: 0x02BB, // ʻ ayn (modifier letter turned comma; LoC remapped from 02BF in 1999)
	0xB1: 0x0142, // ł
	0xB2: 0x00F8, // ø
	0xB3: 0x0111, // đ
	0xB4: 0x00FE, // þ
	0xB5: 0x00E6, // æ
	0xB6: 0x0153, // œ
	0xB7: 0x02BA, // ʺ modifier double prime
	0xB8: 0x0131, // ı dotless i
	0xB9: 0x00A3, // £
	0xBA: 0x00F0, // ð
	0xBC: 0x01A1, // ơ small o with horn (lowercase of 0xAC)
	0xBD: 0x01B0, // ư small u with horn (lowercase of 0xAD)
	0xC0: 0x00B0, // °
	0xC1: 0x2113, // ℓ
	0xC2: 0x2117, // ℗
	0xC3: 0x00A9, // ©
	0xC4: 0x266F, // ♯ music sharp
	0xC5: 0x00BF, // ¿
	0xC6: 0x00A1, // ¡
	0xC7: 0x00DF, // ß
	0xC8: 0x20AC, // €
}

// anselCombining maps ANSEL combining diacritic bytes to their Unicode
// combining code points. The double-diacritic halves (0xEB/0xEC ligature and
// 0xFA/0xFB double tilde) use the precise half-mark code points U+FE20-FE23
// rather than the single spanning marks U+0361/U+0360 the LoC table also lists,
// so the left/right halves remain distinct and round-trippable.
var anselCombining = map[byte]rune{
	0xE0: 0x0309, // hook above
	0xE1: 0x0300, // grave
	0xE2: 0x0301, // acute
	0xE3: 0x0302, // circumflex
	0xE4: 0x0303, // tilde
	0xE5: 0x0304, // macron
	0xE6: 0x0306, // breve
	0xE7: 0x0307, // dot above
	0xE8: 0x0308, // diaeresis
	0xE9: 0x030C, // caron (hacek)
	0xEA: 0x030A, // ring above
	0xEB: 0xFE20, // ligature left half
	0xEC: 0xFE21, // ligature right half
	0xED: 0x0315, // comma above right
	0xEE: 0x030B, // double acute
	0xEF: 0x0310, // candrabindu
	0xF0: 0x0327, // cedilla
	0xF1: 0x0328, // ogonek
	0xF2: 0x0323, // dot below
	0xF3: 0x0324, // double dot below
	0xF4: 0x0325, // ring below
	0xF5: 0x0333, // double low line
	0xF6: 0x0332, // low line
	0xF7: 0x0326, // comma below
	0xF8: 0x031C, // left half ring below
	0xF9: 0x032E, // breve below
	0xFA: 0xFE22, // double tilde left half
	0xFB: 0xFE23, // double tilde right half
	0xFE: 0x0313, // comma above
}

// marc8Compose maps a base character and a combining mark to their precomposed
// (NFC) code point, covering the common Latin combinations. Pairs not listed are
// emitted in decomposed form (base followed by the combining mark).
var marc8Compose = buildCompose()

// buildCompose builds the precomposition table from compact mark-indexed lists,
// keeping the source readable.
func buildCompose() map[[2]rune]rune {
	type group struct {
		mark  rune
		pairs string // space-separated "base:composed" entries
	}
	groups := []group{
		{0x0300, "A:À a:à E:È e:è I:Ì i:ì O:Ò o:ò U:Ù u:ù N:Ǹ n:ǹ Y:Ỳ y:ỳ W:Ẁ w:ẁ"},
		{0x0301, "A:Á a:á E:É e:é I:Í i:í O:Ó o:ó U:Ú u:ú Y:Ý y:ý C:Ć c:ć G:Ǵ g:ǵ K:Ḱ k:ḱ L:Ĺ l:ĺ M:Ḿ m:ḿ N:Ń n:ń P:Ṕ p:ṕ R:Ŕ r:ŕ S:Ś s:ś Z:Ź z:ź"},
		{0x0302, "A:Â a:â E:Ê e:ê I:Î i:î O:Ô o:ô U:Û u:û W:Ŵ w:ŵ Y:Ŷ y:ŷ C:Ĉ c:ĉ G:Ĝ g:ĝ H:Ĥ h:ĥ J:Ĵ j:ĵ S:Ŝ s:ŝ"},
		{0x0303, "A:Ã a:ã E:Ẽ e:ẽ I:Ĩ i:ĩ O:Õ o:õ U:Ũ u:ũ N:Ñ n:ñ V:Ṽ v:ṽ Y:Ỹ y:ỹ"},
		{0x0304, "A:Ā a:ā E:Ē e:ē I:Ī i:ī O:Ō o:ō U:Ū u:ū"},
		{0x0306, "A:Ă a:ă E:Ĕ e:ĕ G:Ğ g:ğ I:Ĭ i:ĭ O:Ŏ o:ŏ U:Ŭ u:ŭ"},
		{0x0307, "C:Ċ c:ċ E:Ė e:ė G:Ġ g:ġ I:İ Z:Ż z:ż"},
		{0x0308, "A:Ä a:ä E:Ë e:ë I:Ï i:ï O:Ö o:ö U:Ü u:ü Y:Ÿ y:ÿ"},
		{0x030A, "A:Å a:å U:Ů u:ů"},
		{0x030B, "O:Ő o:ő U:Ű u:ű"},
		{0x030C, "C:Č c:č D:Ď d:ď E:Ě e:ě G:Ǧ g:ǧ L:Ľ l:ľ N:Ň n:ň R:Ř r:ř S:Š s:š T:Ť t:ť Z:Ž z:ž"},
		{0x0327, "C:Ç c:ç G:Ģ g:ģ K:Ķ k:ķ L:Ļ l:ļ N:Ņ n:ņ R:Ŗ r:ŗ S:Ş s:ş T:Ţ t:ţ"},
		{0x0328, "A:Ą a:ą E:Ę e:ę I:Į i:į O:Ǫ o:ǫ U:Ų u:ų"},
	}

	m := make(map[[2]rune]rune)
	for _, g := range groups {
		for pair := range strings.FieldsSeq(g.pairs) {
			base, composed, _ := strings.Cut(pair, ":")
			m[[2]rune{[]rune(base)[0], g.mark}] = []rune(composed)[0]
		}
	}
	return m
}

// charset identifies the working character set designated to G1, the set that
// governs high (0x80-0xFF) bytes. G0 is always decoded as Basic Latin within
// this package's scope.
type charset int

const (
	csASCII charset = iota // Basic Latin
	csANSEL                // Extended Latin (ANSEL)
	csOther                // unsupported; bytes passed through best-effort
)

// Decoder decodes MARC-8 with persistent G1 designation state. MARC-8 reinstates
// the default working sets at the start of each field, not each subfield, so a
// field is decoded with a single Decoder (create a new one per field) and its
// subfields share the designation state.
type Decoder struct {
	g1    charset
	lossy bool
}

// NewDecoder returns a Decoder initialized to the MARC-8 default working sets
// (Basic Latin in G0, ANSEL Extended Latin in G1).
func NewDecoder() *Decoder {
	return &Decoder{g1: csANSEL}
}

// Lossy reports whether any Decode call on this Decoder fell back to best-effort
// passthrough of an out-of-scope MARC-8 character set, meaning the decoded text
// may contain mojibake. Callers re-serializing MARC-8 as UTF-8 can use this to
// avoid silently emitting corrupted data labeled as clean Unicode.
func (d *Decoder) Lossy() bool { return d.lossy }

// Decode decodes a MARC-8 byte sequence to a UTF-8 string for the supported
// Western subset, passing unsupported sets through best-effort and carrying the
// G1 designation state forward for the next call.
func (d *Decoder) Decode(data []byte) string {
	var b strings.Builder
	var pending []rune

	flush := func(base rune) {
		if len(pending) == 0 {
			b.WriteRune(base)
			return
		}
		composed, rest := base, pending
		if c, ok := marc8Compose[[2]rune{base, pending[0]}]; ok {
			composed, rest = c, pending[1:]
		}
		b.WriteRune(composed)
		for _, m := range rest {
			b.WriteRune(m)
		}
		pending = pending[:0]
	}

	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == escape:
			i += d.interpretEscape(data[i:])
		case c < 0x80:
			flush(rune(c))
			i++
		default:
			if d.g1 == csANSEL {
				if m, ok := anselCombining[c]; ok {
					pending = append(pending, m)
					i++
					continue
				}
				if r, ok := anselGraphic[c]; ok {
					flush(r)
					i++
					continue
				}
			}
			d.lossy = true // best-effort pass-through (Latin-1) of an out-of-scope byte
			flush(rune(c))
			i++
		}
	}
	for _, m := range pending {
		b.WriteRune(m)
	}
	return b.String()
}

// Decode decodes a MARC-8 byte sequence to UTF-8 using a fresh Decoder. It is a
// convenience for one-shot decoding where designation state need not persist.
func Decode(data []byte) string {
	return NewDecoder().Decode(data)
}

// interpretEscape consumes one escape sequence starting at data[0] (the escape
// byte) and updates the G1 designation. It returns the number of bytes consumed.
// A Latin designation (ASCII or ANSEL) switches G1; any multibyte or other
// designation marks the stream lossy because those bytes are out of scope and
// pass through best-effort. G0 designations are parsed and skipped (GL bytes are
// always decoded as Basic Latin), but a non-ASCII G0 designation is still
// flagged lossy.
func (d *Decoder) interpretEscape(data []byte) int {
	n := 1
	for n < len(data) && data[n] >= 0x20 && data[n] <= 0x2F {
		n++
	}
	var final byte
	if n < len(data) && data[n] >= 0x30 && data[n] <= 0x7E {
		final = data[n]
		n++
	}
	if n == 1 {
		return 2 // malformed: skip the escape byte and one more
	}

	targetG1, multibyte := false, false
	for _, ib := range data[1 : n-1] {
		switch ib {
		case ')', '-':
			targetG1 = true
		case '(', ',':
			targetG1 = false
		case '$':
			multibyte = true
		}
	}

	set := csOther
	switch {
	case multibyte:
		set = csOther
	case final == 'B':
		set = csASCII
	case final == 'E':
		set = csANSEL
	}
	if targetG1 {
		d.g1 = set
	}
	if set == csOther {
		d.lossy = true
	}
	return n
}

// Inverse tables for encoding, derived from the decode tables so the two stay in
// sync. encGraphic maps a Unicode code point to its ANSEL G1 byte; encCombining
// maps a Unicode combining mark to its ANSEL byte; encCompose maps a precomposed
// (NFC) code point to its base and combining mark.
var (
	encGraphic   = invertGraphic()
	encCombining = invertCombining()
	encCompose   = invertCompose()
)

func invertGraphic() map[rune]byte {
	m := make(map[rune]byte, len(anselGraphic))
	for b, r := range anselGraphic {
		m[r] = b
	}
	return m
}

func invertCombining() map[rune]byte {
	m := make(map[rune]byte, len(anselCombining))
	for b, r := range anselCombining {
		m[r] = b
	}
	return m
}

func invertCompose() map[rune][2]rune {
	m := make(map[rune][2]rune, len(marc8Compose))
	for pair, composed := range marc8Compose {
		m[composed] = pair // pair is [2]rune{base, mark}
	}
	return m
}

// Encode encodes a UTF-8 string to MARC-8 for the supported Western subset: ASCII
// (G0) and ANSEL Extended Latin (G1) including combining diacritics. It is the
// inverse of Decode: a precomposed Latin character is decomposed to its base and
// combining mark, and combining marks are emitted BEFORE their base character (as
// MARC-8 requires, the reverse of Unicode order). It returns an error on the
// first code point outside the supported subset (e.g. Greek, Cyrillic, CJK), so
// callers learn the value is not representable rather than producing mojibake.
func Encode(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// A bare/leading combining mark (no preceding base) is emitted as-is.
		if b, ok := encCombining[r]; ok {
			out = append(out, b)
			continue
		}

		var marks []byte
		var baseByte byte
		switch {
		case r < 0x80:
			// The escape introducer and the ISO 2709 separators are structural in
			// a MARC-8 stream and cannot appear as data.
			if r == escape || r == 0x1d || r == 0x1e || r == 0x1f {
				return nil, fmt.Errorf("marc8: cannot encode reserved control byte 0x%02X", r)
			}
			baseByte = byte(r)
		default:
			if b, ok := encGraphic[r]; ok {
				baseByte = b
			} else if pair, ok := encCompose[r]; ok {
				marks = append(marks, encCombining[pair[1]])
				baseByte = byte(pair[0]) // the base is always an ASCII letter
			} else {
				return nil, fmt.Errorf("marc8: cannot encode %q (U+%04X)", r, r)
			}
		}

		// Gather any following NFD combining marks so they precede the base too.
		for i+1 < len(runes) {
			if b, ok := encCombining[runes[i+1]]; ok {
				marks = append(marks, b)
				i++
			} else {
				break
			}
		}
		out = append(out, marks...)
		out = append(out, baseByte)
	}
	return out, nil
}
