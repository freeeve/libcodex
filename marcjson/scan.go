package marcjson

import (
	"bufio"
	"fmt"
	"io"
	"unicode/utf16"
	"unicode/utf8"
)

// scanner is a minimal streaming JSON tokenizer for the MARC-in-JSON grammar. It
// replaces encoding/json.Decoder.Token on the read path: that API boxes every
// token in an interface and allocates a syntax-error object at value boundaries,
// which profiling showed to be ~95% of the decoder's allocations. This scanner
// walks the byte stream directly, allocating a Go string only for each retained
// value.
//
// It treats ',' and ':' as insignificant separators (skipped with whitespace),
// like Decoder.Token does, so the higher-level reader parses the object/array
// structure without threading separators through every step. It is deliberately
// lenient on separator placement; what it accepts still round-trips (the fuzz
// contract), and it never loops or panics on malformed input -- every read either
// advances or returns an error.
type scanner struct {
	br  *bufio.Reader
	buf []byte // reused scratch for decoding string bodies
}

func newScanner(r io.Reader) *scanner { return &scanner{br: bufio.NewReader(r)} }

// isInsignificant reports whether c is JSON whitespace or a structural separator
// the scanner skips between tokens.
func isInsignificant(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == ':'
}

// peek returns the next significant byte without consuming it, skipping leading
// whitespace and separators.
func (s *scanner) peek() (byte, error) {
	for {
		bs, err := s.br.Peek(1)
		if err != nil {
			return 0, err
		}
		if isInsignificant(bs[0]) {
			_, _ = s.br.Discard(1)
			continue
		}
		return bs[0], nil
	}
}

// consume returns the next significant byte, skipping leading whitespace and
// separators.
func (s *scanner) consume() (byte, error) {
	for {
		c, err := s.br.ReadByte()
		if err != nil {
			return 0, err
		}
		if !isInsignificant(c) {
			return c, nil
		}
	}
}

// expect consumes the next significant byte and checks it equals d.
func (s *scanner) expect(d byte) error {
	c, err := s.consume()
	if err != nil {
		return err
	}
	if c != d {
		return fmt.Errorf("marcjson: expected %q, got %q", d, c)
	}
	return nil
}

// more reports whether another element or member precedes the container's closing
// byte, so a caller can loop `for more { … }` then expect(close).
func (s *scanner) more(closeByte byte) (bool, error) {
	c, err := s.peek()
	if err != nil {
		return false, err
	}
	return c != closeByte, nil
}

// readString consumes a JSON string token and returns its decoded value.
func (s *scanner) readString() (string, error) {
	if err := s.expect('"'); err != nil {
		return "", err
	}
	return s.stringBody()
}

// stringBody decodes a string's contents after the opening quote, up to and
// including the closing quote, resolving escapes and \uXXXX (with surrogate
// pairs). It returns a freshly allocated string, since the value is retained by
// the record.
func (s *scanner) stringBody() (string, error) {
	s.buf = s.buf[:0]
	for {
		c, err := s.br.ReadByte()
		if err != nil {
			return "", err // unterminated string
		}
		switch c {
		case '"':
			return string(s.buf), nil
		case '\\':
			if s.buf, err = s.escape(); err != nil {
				return "", err
			}
		default:
			s.buf = append(s.buf, c)
		}
	}
}

// escape decodes one backslash escape (the '\' already consumed) onto s.buf and
// returns the extended buffer.
func (s *scanner) escape() ([]byte, error) {
	c, err := s.br.ReadByte()
	if err != nil {
		return s.buf, err
	}
	switch c {
	case '"', '\\', '/':
		return append(s.buf, c), nil
	case 'b':
		return append(s.buf, '\b'), nil
	case 'f':
		return append(s.buf, '\f'), nil
	case 'n':
		return append(s.buf, '\n'), nil
	case 'r':
		return append(s.buf, '\r'), nil
	case 't':
		return append(s.buf, '\t'), nil
	case 'u':
		r, err := s.hex4()
		if err != nil {
			return s.buf, err
		}
		// Combine a UTF-16 surrogate pair; a lone or unpaired surrogate becomes the
		// replacement character, matching encoding/json.
		if utf16.IsSurrogate(rune(r)) {
			if lo, ok := s.lowSurrogate(); ok {
				return utf8.AppendRune(s.buf, utf16.DecodeRune(rune(r), rune(lo))), nil
			}
			return utf8.AppendRune(s.buf, utf8.RuneError), nil
		}
		return utf8.AppendRune(s.buf, rune(r)), nil
	default:
		return s.buf, fmt.Errorf("marcjson: invalid escape \\%c", c)
	}
}

// lowSurrogate reads a following \uXXXX low surrogate if present, returning it and
// true; otherwise it consumes nothing and returns false.
func (s *scanner) lowSurrogate() (uint16, bool) {
	bs, err := s.br.Peek(6)
	if err != nil || bs[0] != '\\' || bs[1] != 'u' {
		return 0, false
	}
	lo := uint16(0)
	for i := 2; i < 6; i++ {
		d, ok := hexVal(bs[i])
		if !ok {
			return 0, false
		}
		lo = lo<<4 | uint16(d)
	}
	if !utf16.IsSurrogate(rune(lo)) {
		return 0, false
	}
	_, _ = s.br.Discard(6)
	return lo, true
}

// hex4 reads four hex digits into a code unit.
func (s *scanner) hex4() (uint16, error) {
	var v uint16
	for range 4 {
		c, err := s.br.ReadByte()
		if err != nil {
			return 0, err
		}
		d, ok := hexVal(c)
		if !ok {
			return 0, fmt.Errorf("marcjson: invalid \\u hex digit %q", c)
		}
		v = v<<4 | uint16(d)
	}
	return v, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// skipValue consumes and discards the next JSON value, including nested objects
// and arrays and scalar literals (string, number, true, false, null).
func (s *scanner) skipValue() error {
	c, err := s.peek()
	if err != nil {
		return err
	}
	switch c {
	case '{':
		return s.skipContainer('{')
	case '[':
		return s.skipContainer('[')
	case '"':
		_, err := s.readString()
		return err
	default:
		return s.skipLiteral()
	}
}

// skipContainer consumes a balanced {…} or […], including any nested containers
// and strings (so a '}' inside a string is not miscounted).
func (s *scanner) skipContainer(open byte) error {
	if err := s.expect(open); err != nil {
		return err
	}
	for depth := 1; depth > 0; {
		c, err := s.br.ReadByte()
		if err != nil {
			return err
		}
		switch c {
		case '"':
			if _, err := s.stringBody(); err != nil {
				return err
			}
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return nil
}

// skipLiteral consumes a scalar literal (number, true, false, null) up to the
// next separator or closing delimiter.
func (s *scanner) skipLiteral() error {
	for {
		bs, err := s.br.Peek(1)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		c := bs[0]
		if isInsignificant(c) || c == '}' || c == ']' {
			return nil
		}
		_, _ = s.br.Discard(1)
	}
}
