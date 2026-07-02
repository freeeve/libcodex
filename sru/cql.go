package sru

import "strings"

// cqlEscaper escapes the characters CQL requires escaping inside a quoted term.
var cqlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// Quote wraps a CQL search term in double quotes, escaping embedded backslashes
// and quotes, so a user-supplied term is safe to place in a CQL query:
//
//	client.NewReader(ctx, "dc.title = "+sru.Quote(userInput))
//
// This library passes CQL through verbatim; it does not parse or build queries
// beyond this escaping helper.
func Quote(term string) string {
	return `"` + cqlEscaper.Replace(term) + `"`
}
