package sru

import "strings"

// Quote wraps a CQL search term in double quotes, escaping embedded backslashes
// and quotes, so a user-supplied term is safe to place in a CQL query:
//
//	client.NewReader(ctx, "dc.title = "+sru.Quote(userInput))
//
// This library passes CQL through verbatim; it does not parse or build queries
// beyond this escaping helper.
func Quote(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(term) + `"`
}
