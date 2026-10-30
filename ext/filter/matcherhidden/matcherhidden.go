// Package matcherhidden is the filter "matcherhidden" of goda-fi.vim, as
// ddu-filter-matcher_hidden: no hidden files (a name or a directory
// starting with "."), unless the input has a ".".
package matcherhidden

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type filter struct{}

// New returns the filter.
func New() fi.Filter { return filter{} }

func (filter) Name() string { return "matcherhidden" }

func (filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	if strings.Contains(a.Input, ".") {
		return a.Items, nil
	}
	out := make([]fi.Item, 0, len(a.Items))
	for i := range a.Items {
		it := &a.Items[i]
		path := file.PathOf(it)
		if path == "" {
			continue
		}
		if strings.HasPrefix(filepath.Base(path), ".") ||
			strings.HasPrefix(filepath.Base(filepath.Dir(path)), ".") ||
			strings.HasPrefix(it.MatcherKey(), ".") {
			continue
		}
		out = append(out, *it)
	}
	return out, nil
}
