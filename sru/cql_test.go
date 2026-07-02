package sru

import (
	"net/url"
	"testing"
)

// TestQueryString pins the CQL each builder form renders.
func TestQueryString(t *testing.T) {
	cases := []struct {
		name string
		q    Query
		want string
	}{
		{"any is bare", Term("any", "moby dick"), `"moby dick"`},
		{"title", Term("title", "moby dick"), `dc.title = "moby dick"`},
		{"author", Term("author", "melville"), `dc.author = "melville"`},
		{"isbn", Term("isbn", "9780142437247"), `dc.isbn = "9780142437247"`},
		{"dotted passthrough", Term("bath.isbn", "9780142437247"), `bath.isbn = "9780142437247"`},
		{"unknown name passes through", Term("auther", "x"), `auther = "x"`},
		{"quoting", Term("title", `say "hi" \ bye`), `dc.title = "say \"hi\" \\ bye"`},
		{"and", And(Term("author", "melville"), Term("title", "moby dick")),
			`(dc.author = "melville") and (dc.title = "moby dick")`},
		{"nested", Or(And(Term("title", "a"), Term("title", "b")), Term("any", "c")),
			`((dc.title = "a") and (dc.title = "b")) or ("c")`},
		{"not", Not(Term("subject", "whales"), Term("subject", "fiction")),
			`(dc.subject = "whales") not (dc.subject = "fiction")`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.q.String(); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// TestQueryAgainstServer runs a built query through the full client path.
func TestQueryAgainstServer(t *testing.T) {
	srv := serve(t, func(q url.Values) []byte {
		if got := q.Get("query"); got != `dc.title = "moby dick"` {
			t.Errorf("query = %q", got)
		}
		return fixture(t, "search_page2.xml")
	})
	c := NewClient(srv.URL)
	resp, err := c.SearchRetrieve(t.Context(), Request{Query: Term("title", "moby dick").String()})
	if err != nil || len(resp.Records) != 1 {
		t.Fatalf("SearchRetrieve: %v (%d records)", err, len(resp.Records))
	}
}
