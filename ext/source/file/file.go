// Package file is the source "file" of goda-fi.vim, as ddu-source-file: the
// files of a directory (SourceOptions.Path, or the current one). The
// directories are trees: expanded, they list their files under them.
//
// The input picks the directory when the source is volatile: "sub/x" lists
// sub (filtered by the matchers with the whole input).
package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the source params.
type Params struct {
	// IgnoreDirectories: files only.
	IgnoreDirectories bool `json:"ignoreDirectories"`
	// New: an item of a new file named by the input, instead.
	New bool `json:"new"`
}

type source struct{ p Params }

// New returns the source with these params (the source params of the
// options go over them).
func New(p Params) fi.Source { return source{p} }

func (source) Name() string { return "file" }
func (source) Kind() string { return "file" }

const chunk = 20000

func (s source) Gather(ctx context.Context, _ sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	p := s.p
	if err := fi.Decode(a.SourceParams, &p); err != nil {
		return err
	}
	base := a.SourcePath()
	input := a.Input
	if a.Parent != nil {
		// The children of a directory expanded: all its files.
		p.New, input = false, ""
	}
	if p.New {
		if input == "" {
			return nil
		}
		return emit([]fi.Item{{
			Word:    input,
			Display: "[new] " + input,
			Action:  file.ActionData{Path: filepath.Join(base, input)},
		}})
	}

	st, err := os.Stat(base)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return emit([]fi.Item{Item(base, base, false)})
	}
	dir := base
	abs := filepath.IsAbs(input)
	if i := strings.LastIndexAny(input, `/`+string(filepath.Separator)); i >= 0 {
		dir = input[:i+1]
		if !abs {
			dir = filepath.Join(base, dir)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if dir != base {
			// The input names a directory that is not there (yet: a new
			// file is typed, say), as ddu-source-file: nothing.
			return nil
		}
		return err
	}
	items := make([]fi.Item, 0, min(len(entries), chunk))
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		it := Item(base, path, abs)
		if p.IgnoreDirectories && it.Action.(file.ActionData).IsDirectory {
			continue
		}
		items = append(items, it)
		if len(items) >= chunk {
			if err := emit(items); err != nil {
				return err
			}
			items = items[:0:0]
		}
	}
	return emit(items)
}

// Item is the item of path: its word relative to base (absolute with abs),
// with "/" after a directory; a link to a directory is one. Its TreePath is
// path; a directory is a tree.
func Item(base, path string, abs bool) fi.Item {
	ad := file.ActionData{Path: path}
	var status *fi.Status
	if lst, err := os.Lstat(path); err == nil {
		st := lst
		if lst.Mode()&os.ModeSymlink != 0 {
			ad.IsLink = true
			if s, err := os.Stat(path); err == nil {
				st = s
			}
		}
		ad.IsDirectory = st.IsDir()
		status = &fi.Status{Size: st.Size(), Time: st.ModTime().UnixMilli()}
	}
	word := path
	if !abs {
		if rel, err := filepath.Rel(base, path); err == nil {
			word = rel
		}
	}
	if ad.IsDirectory {
		word += "/"
	}
	return fi.Item{Word: filepath.ToSlash(word), Action: ad, Status: status, TreePath: path, IsTree: ad.IsDirectory}
}
