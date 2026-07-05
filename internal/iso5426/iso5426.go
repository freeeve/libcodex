// Package iso5426 transcodes between the ISO 5426 character set — the extended
// Latin character set used by UNIMARC records — and UTF-8.
//
// Like MARC-8, ISO 5426 stores a combining diacritic BEFORE the base character it
// modifies (the reverse of Unicode). The decoder reorders the mark after the base
// and composes the pair to a precomposed (NFC) code point when ISO 5426 defines
// one; otherwise it emits the base followed by the standalone combining mark.
//
// The graphic and precomposition tables are generated from the ISO 5426 ICU
// character map (see gen); the standalone combining marks are hand-maintained
// here, verified against the marc4j ISO 5426 converter.
package iso5426

//go:generate go run ./gen

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// iso5426Graphic maps an ISO 5426 single graphic byte to its rune: the symbol
// block (0xA1-0xBF) and the letter block (0xE1-0xFF). Hand-maintained from the
// standard, cross-checked with the marc4j ISO 5426 converter. Byte 0xF9 is both
// the letter ø and the combining breve-below mark; the decoder disambiguates it.
var iso5426Graphic = map[byte]rune{
	0xA1: 0x00A1, 0xA2: 0x201E, 0xA3: 0x00A3, 0xA4: 0x0024, 0xA5: 0x00A5,
	0xA6: 0x2020, 0xA7: 0x00A7, 0xA8: 0x2032, 0xA9: 0x2018, 0xAA: 0x201C,
	0xAB: 0x00AB, 0xAC: 0x266D, 0xAD: 0x00A9, 0xAE: 0x2117, 0xAF: 0x00AE,
	0xB0: 0x02BB, 0xB1: 0x02BC, 0xB2: 0x201A, 0xB6: 0x2021, 0xB7: 0x00B7,
	0xB8: 0x2033, 0xB9: 0x2019, 0xBA: 0x201D, 0xBB: 0x00BB, 0xBC: 0x266F,
	0xBD: 0x02B9, 0xBE: 0x02BA, 0xBF: 0x00BF,
	0xE1: 0x00C6, 0xE2: 0x0110, 0xE6: 0x0132, 0xE8: 0x0141, 0xE9: 0x00D8,
	0xEA: 0x0152, 0xEC: 0x00DE,
	0xF1: 0x00E6, 0xF2: 0x0111, 0xF3: 0x00F0, 0xF5: 0x0131, 0xF6: 0x0133,
	0xF8: 0x0142, 0xF9: 0x00F8, 0xFA: 0x0153, 0xFB: 0x00DF, 0xFC: 0x00FE,
}

// iso5426Combining maps an ISO 5426 combining-mark byte to its Unicode combining
// code point, used when a mark+base pair has no precomposed form.
var iso5426Combining = map[byte]rune{
	0xC0: 0x0309, // hook above
	0xC1: 0x0300, // grave
	0xC2: 0x0301, // acute
	0xC3: 0x0302, // circumflex
	0xC4: 0x0303, // tilde
	0xC5: 0x0304, // macron
	0xC6: 0x0306, // breve
	0xC7: 0x0307, // dot above
	0xC8: 0x0308, // diaeresis
	0xC9: 0x0308, // umlaut (same combining point as diaeresis)
	0xCA: 0x030A, // ring above
	0xCD: 0x030B, // double acute
	0xCE: 0x031B, // horn
	0xCF: 0x030C, // caron
	0xD0: 0x0327, // cedilla
	0xD3: 0x0328, // ogonek
	0xD4: 0x0325, // ring below
	0xD6: 0x0323, // dot below
	0xD7: 0x0324, // double dot below
	0xD8: 0x0332, // low line
	0xD9: 0x0333, // double low line
	0xDA: 0x0329, // vertical line below
	0xF9: 0x032E, // breve below
}

// decNFC composes a base rune and a (preceding) combining mark to the precomposed
// rune ISO 5426 stores as a mark+base byte pair, for the decoder.
var decNFC = func() map[[2]rune]rune {
	m := make(map[[2]rune]rune, len(iso5426Compose))
	for k, composed := range iso5426Compose {
		m[[2]rune{decodeRune(k[1]), iso5426Combining[k[0]]}] = composed
	}
	return m
}()

// Decode decodes an ISO 5426 byte sequence to a UTF-8 string. Combining marks
// precede their base in ISO 5426; the decoder buffers them and emits them after
// the base, composing the base with the innermost (first-buffered) mark to a
// precomposed (NFC) code point when one exists. Stacked marks are preserved in
// Unicode order.
func Decode(data []byte) string {
	s, _ := DecodeLossy(data)
	return s
}

// DecodeLossy decodes like Decode and additionally reports whether any byte fell
// through the best-effort Latin-1 passthrough (an undefined high byte with no
// ISO 5426 mapping). A caller that must not silently corrupt data can check the
// flag, mirroring the marc8 decoder's Lossy signal.
func DecodeLossy(data []byte) (string, bool) {
	var b strings.Builder
	b.Grow(len(data))
	var pending []rune // combining marks awaiting their base
	lossy := false

	flush := func(base rune) {
		if len(pending) == 0 {
			b.WriteRune(base)
			return
		}
		// The innermost mark (first buffered, nearest the base in Unicode order)
		// composes with the base; any remaining stacked marks follow in order.
		if c, ok := decNFC[[2]rune{base, pending[0]}]; ok {
			b.WriteRune(c)
			for _, m := range pending[1:] {
				b.WriteRune(m)
			}
		} else {
			b.WriteRune(base)
			for _, m := range pending {
				b.WriteRune(m)
			}
		}
		pending = pending[:0]
	}

	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c < 0x80:
			flush(rune(c))
			i++
		case isCombining(c):
			// 0xF9 is both the breve-below mark and the letter ø; treat it as a mark
			// only when it composes with the following byte, otherwise as the graphic.
			// With marks already pending it is always the graphic: marks precede their
			// base, and Encode places an inner decomposition mark only at the start of
			// a run — so a mark-then-0xF9 sequence is a diacritic on ø, not two marks.
			if g, isGraphic := iso5426Graphic[c]; isGraphic {
				if len(pending) > 0 || i+1 >= len(data) || !composes(c, data[i+1]) {
					flush(g)
					i++
					continue
				}
			}
			pending = append(pending, iso5426Combining[c])
			i++
		default:
			r, ok := graphicRune(c)
			if !ok {
				lossy = true
			}
			flush(r)
			i++
		}
	}
	for _, m := range pending {
		b.WriteRune(m)
	}
	return b.String(), lossy
}

func isCombining(c byte) bool { _, ok := iso5426Combining[c]; return ok }

func composes(mark, base byte) bool { _, ok := iso5426Compose[[2]byte{mark, base}]; return ok }

// decodeRune decodes a single non-combining byte (a base or a graphic) to a rune.
func decodeRune(c byte) rune {
	if c < 0x80 {
		return rune(c)
	}
	r, _ := graphicRune(c)
	return r
}

// graphicRune decodes a high graphic byte, returning its rune and whether the
// byte was defined; an undefined byte passes through best-effort as Latin-1 with
// ok=false so the caller can flag the decode lossy.
func graphicRune(c byte) (rune, bool) {
	if r, ok := iso5426Graphic[c]; ok {
		return r, true
	}
	return rune(c), false // best-effort Latin-1 passthrough
}

// ---- encoding ----

var (
	encOnce      sync.Once
	encGraphic   map[rune]byte    // rune -> graphic byte (runes >= 0x80)
	encCompose   map[rune][2]byte // precomposed rune -> {mark byte, base byte}
	encCombining map[rune]byte    // combining mark rune -> mark byte
	encNFC       map[[2]rune]rune // {base rune, mark rune} -> precomposed (decNFC inverse)
)

func buildEncode() {
	encGraphic = make(map[rune]byte, len(iso5426Graphic))
	for b, r := range iso5426Graphic {
		if r >= 0x80 { // ASCII always encodes to its own byte
			if _, ok := encGraphic[r]; !ok {
				encGraphic[r] = b
			}
		}
	}
	// Build the compose inverse deterministically (lowest byte pair wins).
	keys := make([][2]byte, 0, len(iso5426Compose))
	for k := range iso5426Compose {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	encCompose = make(map[rune][2]byte, len(iso5426Compose))
	encNFC = make(map[[2]rune]rune, len(decNFC))
	maps.Copy(encNFC, decNFC)
	for _, k := range keys {
		composed := iso5426Compose[k]
		if _, ok := encCompose[composed]; !ok {
			encCompose[composed] = k
		}
	}
	// Build the standalone combining-mark inverse deterministically (lowest byte
	// wins). Skip bytes that are also graphics (0xF9 = ø): a standalone mark there
	// is ambiguous and only representable as part of a composition.
	encCombining = make(map[rune]byte, len(iso5426Combining))
	cbytes := make([]int, 0, len(iso5426Combining))
	for b := range iso5426Combining {
		cbytes = append(cbytes, int(b))
	}
	sort.Ints(cbytes)
	for _, bi := range cbytes {
		b := byte(bi)
		if _, isGraphic := iso5426Graphic[b]; isGraphic {
			continue
		}
		if r := iso5426Combining[b]; encCombining[r] == 0 {
			encCombining[r] = b
		}
	}
}

// Encode encodes a UTF-8 string to ISO 5426. It is the inverse of Decode: a base
// followed by a combining mark is first composed to its precomposed form, then
// emitted either as a single graphic byte or as ISO 5426's combining-mark-then-
// base byte pair. It returns an error on the first code point ISO 5426 cannot
// represent.
func Encode(s string) ([]byte, error) {
	encOnce.Do(buildEncode)
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size // i now points just past r, where any following runes begin
		// A combining mark with no preceding base. In ISO 5426 a mark applies to the
		// FOLLOWING character, so if the next rune is a base they compose; otherwise
		// the mark is emitted standalone (it must not gather, which would reorder
		// marks among themselves and not round-trip).
		if b, ok := encCombining[r]; ok {
			if i < len(s) {
				next, nsize := utf8.DecodeRuneInString(s[i:])
				if c, ok := encNFC[[2]rune{next, r}]; ok {
					var err error
					if out, err = encodeBase(out, c, nil); err != nil {
						return nil, err
					}
					i += nsize
					continue
				}
			}
			out = append(out, b)
			continue
		}
		// A base gathers the combining marks that follow it. ISO 5426 stores marks
		// before the base, innermost first; encodeBase places the base's own
		// decomposition mark (when it is a precomposed graphic) ahead of these
		// gathered outer marks so the sequence decodes back in Unicode order.
		var marks []byte
		for i < len(s) {
			next, nsize := utf8.DecodeRuneInString(s[i:])
			b, ok := encCombining[next]
			if !ok {
				break
			}
			marks = append(marks, b)
			i += nsize
		}
		var err error
		if out, err = encodeBase(out, r, marks); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// encodeBase appends the ISO 5426 encoding of base rune r to out. The gathered
// outer marks (already ISO 5426 mark bytes, innermost first) are inserted after
// r's own decomposition mark when r is a precomposed graphic, and otherwise
// immediately before r, so the byte order round-trips through Decode.
func encodeBase(out []byte, r rune, outerMarks []byte) ([]byte, error) {
	if r < 0x80 {
		if r == 0x1b || r == 0x1d || r == 0x1e || r == 0x1f {
			return nil, fmt.Errorf("iso5426: cannot encode reserved control byte 0x%02X", r)
		}
		out = append(out, outerMarks...)
		return append(out, byte(r)), nil
	}
	if b, ok := encGraphic[r]; ok { // a single graphic byte, no own mark
		out = append(out, outerMarks...)
		return append(out, b), nil
	}
	if pair, ok := encCompose[r]; ok { // inner mark byte, then outer marks, then base byte
		out = append(out, pair[0])
		out = append(out, outerMarks...)
		return append(out, pair[1]), nil
	}
	return nil, fmt.Errorf("iso5426: cannot encode %q (U+%04X)", r, r)
}
