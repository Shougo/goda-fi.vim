// Package matcherfiles is the filter "matcherfiles" of goda-fi.vim, as
// ddu-filter-matcher_files: only the items whose path matches one of the
// globs ("*.go", "**/test/*") or patterns (regular expressions). Without
// either, every item.
package matcherfiles

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the filter params, as ddu-filter-matcher_files's.
type Params struct {
	Globs    []string `json:"globs"`
	Patterns []string `json:"patterns"`
}

type filter struct{ p Params }

// New returns the filter with these params (the filter params of the options
// go over them).
func New(p Params) fi.Filter { return filter{p} }

func (filter) Name() string { return "matcherfiles" }

func (f filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	p := f.p
	if err := fi.Decode(a.FilterParams, &p); err != nil {
		return nil, err
	}
	if len(p.Globs) == 0 && len(p.Patterns) == 0 {
		return a.Items, nil
	}
	var res []*regexp.Regexp
	for _, pat := range p.Patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, err
		}
		res = append(res, re)
	}
	for _, g := range p.Globs {
		re, err := regexp.Compile(Glob("**/" + g))
		if err != nil {
			return nil, err
		}
		res = append(res, re)
	}
	out := make([]fi.Item, 0, len(a.Items))
	for i := range a.Items {
		path := filepath.ToSlash(file.PathOf(&a.Items[i]))
		if path == "" {
			continue
		}
		for _, re := range res {
			if re.MatchString(path) {
				out = append(out, a.Items[i])
				break
			}
		}
	}
	return out, nil
}

// Glob is the regular expression of a glob of paths: "**" is any
// directories, "*" and "?" are within a name, "{a,b}" either.
func Glob(g string) string {
	var b strings.Builder
	b.WriteString("^")
	depth := 0
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch {
		case strings.HasPrefix(g[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(g[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			b.WriteString("(?:")
			depth++
		case c == '}' && depth > 0:
			b.WriteString(")")
			depth--
		case c == ',' && depth > 0:
			b.WriteString("|")
		case c == '[':
			if j := strings.IndexByte(g[i:], ']'); j > 0 {
				class := g[i+1 : i+j]
				if strings.HasPrefix(class, "!") {
					class = "^" + class[1:]
				}
				b.WriteString("[" + class + "]")
				i += j
				continue
			}
			b.WriteString(`\[`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return b.String()
}
