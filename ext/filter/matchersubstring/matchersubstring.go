// Package matchersubstring is the filter "matchersubstring" of goda-fi.vim, as
// ddu-filter-matcher_substring: the items that contain every word of the
// input. A word may be "!word" (not contained), "<word" (at the start of a
// word), "^word" (a prefix), "word$" (a suffix); "\ " is a space.
package matchersubstring

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the filter params, as ddu-filter-matcher_substring's.
type Params struct {
	// HighlightMatched: the highlight group of what matched ("": none).
	HighlightMatched string `json:"highlightMatched"`
	// Limit: at most this many items.
	Limit int `json:"limit"`
	// MaxLength: longer items do not match.
	MaxLength int `json:"maxLength"`
}

type filter struct{ p Params }

// New returns the filter with these params (the filter params of the options
// go over them); the zero Params are ddu-filter-matcher_substring's
// defaults: 1000 items, of 500 bytes at most.
func New(p Params) fi.Filter {
	if p.Limit <= 0 {
		p.Limit = 1000
	}
	if p.MaxLength <= 0 {
		p.MaxLength = 500
	}
	return filter{p}
}

func (filter) Name() string { return "matchersubstring" }

type mode int

const (
	contains mode = iota
	negate
	word
	prefix
	suffix
)

type token struct {
	mode  mode
	value string
	re    *regexp.Regexp
}

// tokens splits the input at spaces not escaped with "\".
func tokens(input string, ignoreCase bool) []token {
	var out []token
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		s := cur.String()
		cur.Reset()
		t := token{value: s}
		switch {
		case strings.HasPrefix(s, "!"):
			t.mode, t.value = negate, s[1:]
		case strings.HasPrefix(s, "<"):
			t.mode, t.value = word, s[1:]
			flags := ""
			if ignoreCase {
				flags = "(?i)"
			}
			t.re = regexp.MustCompile(flags + `\b` + regexp.QuoteMeta(t.value))
		case strings.HasPrefix(s, "^"):
			t.mode, t.value = prefix, s[1:]
		case strings.HasSuffix(s, "$"):
			t.mode, t.value = suffix, s[:len(s)-1]
		}
		if ignoreCase {
			t.value = strings.ToLower(t.value)
		}
		out = append(out, t)
	}
	rs := []rune(input)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\\' && i+1 < len(rs) && unicode.IsSpace(rs[i+1]) {
			cur.WriteRune(rs[i+1])
			i++
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

func (t token) match(key, lower string) bool {
	switch t.mode {
	case negate:
		return !strings.Contains(lower, t.value)
	case word:
		return t.re.MatchString(key)
	case prefix:
		return strings.HasPrefix(lower, t.value)
	case suffix:
		return strings.HasSuffix(lower, t.value)
	}
	return strings.Contains(lower, t.value)
}

func (f filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	if a.Input == "" {
		return a.Items, nil
	}
	p := f.p
	if err := fi.Decode(a.FilterParams, &p); err != nil {
		return nil, err
	}
	ignoreCase := a.IgnoreCase()
	ts := tokens(a.Input, ignoreCase)
	out := make([]fi.Item, 0, min(len(a.Items), p.Limit))
	for _, it := range a.Items {
		key := it.MatcherKey()
		if key == "" || len(key) > p.MaxLength {
			continue
		}
		lower := key
		if ignoreCase {
			lower = strings.ToLower(key)
		}
		ok := true
		for _, t := range ts {
			if !t.match(key, lower) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if p.HighlightMatched != "" {
			it = highlight(it, ts, ignoreCase, p.HighlightMatched)
		}
		out = append(out, it)
		if len(out) >= p.Limit {
			break
		}
	}
	return out, nil
}

// highlight adds the highlights of the last match of every token in the text
// shown.
func highlight(it fi.Item, ts []token, ignoreCase bool, group string) fi.Item {
	text := it.Text()
	key := text
	if ignoreCase {
		key = strings.ToLower(text)
	}
	hls := append([]fi.Highlight(nil), it.Highlights...)
	for _, t := range ts {
		if t.mode == negate || t.value == "" {
			continue
		}
		i := strings.LastIndex(key, t.value)
		if i < 0 {
			continue
		}
		hls = append(hls, fi.Highlight{Name: "matched", HlGroup: group, Col: i + 1, Width: len(t.value)})
	}
	it.Highlights = hls
	return it
}
