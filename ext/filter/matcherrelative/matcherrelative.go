// Package matcherrelative is the filter "matcherrelative" of goda-fi.vim, as
// ddu-filter-matcher_relative: only the files under the directory of the
// source (SourceOptions.Path, or the current one).
package matcherrelative

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

func (filter) Name() string { return "matcherrelative" }

func (filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	base := a.SourcePath()
	out := make([]fi.Item, 0, len(a.Items))
	for i := range a.Items {
		path := file.PathOf(&a.Items[i])
		if path == "" {
			continue
		}
		rel, err := filepath.Rel(base, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		out = append(out, a.Items[i])
	}
	return out, nil
}
