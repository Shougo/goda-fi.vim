// Package sorteralpha is the filter "sorteralpha" of goda-fi.vim, as
// ddu-filter-sorter_alpha: the items by their word.
package sorteralpha

import (
	"context"
	"slices"
	"strings"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type filter struct{}

// New returns the filter.
func New() fi.Filter { return filter{} }

func (filter) Name() string { return "sorteralpha" }

func (filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	out := slices.Clone(a.Items)
	slices.SortStableFunc(out, func(x, y fi.Item) int { return strings.Compare(x.Word, y.Word) })
	return out, nil
}
