// Package buffer is the source "buffer" of goda-fi.vim, as ddu-source-buffer:
// the listed buffers of files, the alternate one first.
package buffer

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
	"github.com/Shougo/goda.vim/sdk/vimscript"
)

// The Vim side of the source (buffer.vim): goda_fi#source#buffer#*().
//
//go:embed buffer.vim
var vimSrc []byte

var script = vimscript.New("autoload/goda_fi/source/buffer.vim", vimSrc)

type source struct{}

// New returns the source.
func New() fi.Source { return source{} }

func (source) Name() string { return "buffer" }
func (source) Kind() string { return "file" }

type buf struct {
	BufNr     int    `json:"bufnr"`
	Path      string `json:"path"`
	Current   bool   `json:"current"`
	Alternate bool   `json:"alternate"`
	Modified  bool   `json:"modified"`
}

func (source) Gather(ctx context.Context, h sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	raw, err := script.Call(ctx, h, "goda_fi#source#buffer#_list", a.Context.WinID, a.Context.BufNr)
	if err != nil {
		return err
	}
	var bufs []buf
	if err := sdk.Decode(raw, &bufs); err != nil {
		return err
	}
	cwd := a.Context.Cwd
	items := make([]fi.Item, 0, len(bufs))
	for _, b := range bufs {
		mark := func(on bool, s string) string {
			if on {
				return s
			}
			return ""
		}
		name := b.Path
		if rel, err := filepath.Rel(cwd, b.Path); err == nil {
			name = filepath.ToSlash(rel)
		}
		items = append(items, fi.Item{
			Word: fmt.Sprintf("%d %s%s %s %s", b.BufNr,
				mark(b.Current, "%"), mark(b.Alternate, "#"), mark(b.Modified, "+"), name),
			Action: file.ActionData{BufNr: b.BufNr, Path: b.Path},
		})
	}
	return emit(items)
}
