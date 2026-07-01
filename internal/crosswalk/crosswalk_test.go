package crosswalk

import (
	"testing"

	"github.com/freeeve/libcodex"
)

func TestTrimISBD(t *testing.T) {
	cases := map[string]string{
		"Title /":   "Title",
		"Title :":   "Title",
		"Author ,":  "Author",
		"Series ;":  "Series",
		"Plain":     "Plain",
		"Trailing ": "Trailing",
		"":          "",
		"/":         "",
	}
	for in, want := range cases {
		if got := TrimISBD(in); got != want {
			t.Errorf("TrimISBD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinSub(t *testing.T) {
	f := codex.NewDataField("245", '1', '0',
		codex.NewSubfield('a', "Title :"),
		codex.NewSubfield('b', "subtitle /"),
		codex.NewSubfield('c', "author"))
	if got := JoinSub(f, "ab", " "); got != "Title subtitle" {
		t.Errorf("JoinSub = %q, want %q", got, "Title subtitle")
	}
	if got := JoinSub(f, "z", " "); got != "" {
		t.Errorf("JoinSub of absent codes = %q, want empty", got)
	}
}

func TestSubject(t *testing.T) {
	f := codex.NewDataField("650", ' ', '0',
		codex.NewSubfield('a', "Topic"),
		codex.NewSubfield('x', "General"),
		codex.NewSubfield('z', "Region "),
		codex.NewSubfield('b', "ignored"))
	if got := Subject(f); got != "Topic--General--Region" {
		t.Errorf("Subject = %q, want %q", got, "Topic--General--Region")
	}
}

func TestYear(t *testing.T) {
	cases := map[string]string{
		"c1993, printed 1995": "1993",
		"2021":                "2021",
		"n.d.":                "",
		"19":                  "",
		"pub. in 0800 AD":     "0800",
	}
	for in, want := range cases {
		if got := Year(in); got != want {
			t.Errorf("Year(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppendJSONString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", `"plain"`},
		{"a\"b", `"a\"b"`},
		{"a\\b", `"a\\b"`},
		{"tab\there", `"tab\there"`},
		{"nl\nhere", `"nl\nhere"`},
		{"cr\rhere", `"cr\rhere"`},
		{"\x01", `"\u0001"`},
		{"éend", "\"éend\""},       // multibyte UTF-8 passes through unescaped
		{"bad\xffend", `"badend"`}, // invalid UTF-8 byte dropped
	}
	for _, c := range cases {
		if got := string(AppendJSONString(nil, c.in)); got != c.want {
			t.Errorf("AppendJSONString(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
