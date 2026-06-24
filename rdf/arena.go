package rdf

import (
	"unicode/utf8"
	"unsafe"
)

// arena packs many short strings — expanded prefixed-name IRIs, unescaped
// literals — into shared backing buffers and hands out substring views, so
// materializing N distinct strings costs about total_bytes/chunkSize allocations
// instead of one per string.
//
// It grows by allocating a fresh chunk when the current one is full; a live chunk
// is never reallocated, so views returned earlier stay valid. Each chunk is kept
// alive by the strings that point into it (the runtime tracks a string's data
// pointer), so the arena itself need not outlive parsing.
type arena struct {
	buf []byte
}

const arenaChunkSize = 1 << 16 // 64 KiB

// reserve ensures the current chunk holds at least n more bytes, starting a fresh
// chunk (never reallocating the live one) when it does not, and returns the offset
// at which writing begins.
func (a *arena) reserve(n int) int {
	if cap(a.buf)-len(a.buf) < n {
		a.buf = make([]byte, 0, max(n, arenaChunkSize)) // old chunk stays alive via its views
	}
	return len(a.buf)
}

// view returns the bytes appended since start as an arena-backed string.
func (a *arena) view(start int) string {
	if len(a.buf) == start {
		return ""
	}
	b := a.buf[start:len(a.buf):len(a.buf)]
	return unsafe.String(&b[0], len(b))
}

// concat returns x+y as an arena-backed string, allocating only when a new chunk
// is needed.
func (a *arena) concat(x, y string) string {
	if len(x)+len(y) == 0 {
		return ""
	}
	start := a.reserve(len(x) + len(y))
	a.buf = append(a.buf, x...)
	a.buf = append(a.buf, y...)
	return a.view(start)
}

// unescape decodes the RDF string escapes in s into the arena. The unescaped form
// is never longer than s, so the single reservation never overflows the chunk.
func (a *arena) unescape(s string) string {
	start := a.reserve(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			r, n := unescapeRune(s[i:])
			a.buf = utf8.AppendRune(a.buf, r)
			i += n
			continue
		}
		a.buf = append(a.buf, s[i])
		i++
	}
	return a.view(start)
}
