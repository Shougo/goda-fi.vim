// Package fileold is the source "fileold" of goda-fi.vim, as
// ddu-source-file_old: the files of v:oldfiles that are there, the viminfo
// (or ShaDa) read again first.
package fileold

import (
	"context"
	_ "embed"
	"path/filepath"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/internal/vimscript"
	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// The Vim side of the source (fileold.vim): goda_fi#source#fileold#*().
//
//go:embed fileold.vim
var vimSrc []byte

var script = vimscript.New("autoload/goda_fi/source/fileold.vim", vimSrc)

type source struct{}

// New returns the source.
func New() fi.Source { return source{} }

func (source) Name() string { return "fileold" }
func (source) Kind() string { return "file" }

func (source) Gather(ctx context.Context, h sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	raw, err := script.Call(ctx, h, "goda_fi#source#fileold#_list")
	if err != nil {
		return err
	}
	var paths []string
	if err := sdk.Decode(raw, &paths); err != nil {
		return err
	}
	items := make([]fi.Item, 0, len(paths))
	for _, p := range paths {
		path := a.Abs(p)
		word := path
		if rel, err := filepath.Rel(a.Context.Cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
			word = rel
		}
		items = append(items, fi.Item{Word: filepath.ToSlash(word), Action: file.ActionData{Path: path}})
	}
	return emit(items)
}
