// Package pathhistory is the source "pathhistory" of goda-fi.vim, as
// ddu-source-path_history: the paths a session left (the action narrow of
// the filer), that of the buffer the session is started from
// (b:goda_fi_name), the last first.
package pathhistory

import (
	"context"
	"slices"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type source struct{}

// New returns the source.
func New() fi.Source { return source{} }

func (source) Name() string { return "pathhistory" }
func (source) Kind() string { return "file" }

func (source) Gather(ctx context.Context, h sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	r, err := h.Call(ctx, "getbufvar", a.Context.BufNr, "goda_fi_name", "")
	name, _ := r.(string)
	if err != nil || name == "" {
		return err
	}
	c, ok := a.ContextOf(name)
	if !ok {
		return nil
	}
	var items []fi.Item
	seen := map[string]bool{}
	for _, p := range slices.Backward(c.PathHistories) {
		if seen[p] {
			continue
		}
		seen[p] = true
		items = append(items, fi.Item{Word: p, Action: file.ActionData{Path: p, IsDirectory: true}, TreePath: p, IsTree: true})
	}
	return emit(items)
}
