// Package filerec is the source "filerec" of goda-fi.vim, as
// ddu-source-file_rec: the files under a directory (SourceOptions.Path, or the
// current one), recursively, as they are found.
package filerec

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the source params, as ddu-source-file_rec's.
type Params struct {
	// ChunkSize: the items are handed over by this many.
	ChunkSize int `json:"chunkSize"`
	// IgnoredDirectories are not entered (by name).
	IgnoredDirectories []string `json:"ignoredDirectories"`
	// ExpandSymbolicLink: links to directories are entered (once each).
	ExpandSymbolicLink bool `json:"expandSymbolicLink"`
}

type source struct{ p Params }

// New returns the source with these params (the source params of the
// options go over them); the zero Params are ddu-source-file_rec's
// defaults: chunks of 1000, ".git" ignored.
func New(p Params) fi.Source {
	if p.ChunkSize <= 0 {
		p.ChunkSize = 1000
	}
	if p.IgnoredDirectories == nil {
		p.IgnoredDirectories = []string{".git"}
	}
	return source{p}
}

func (source) Name() string { return "filerec" }
func (source) Kind() string { return "file" }

func (s source) Gather(ctx context.Context, _ sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	p := s.p
	if err := fi.Decode(a.SourceParams, &p); err != nil {
		return err
	}
	p.ChunkSize = max(p.ChunkSize, 1)
	root := a.SourcePath()
	visited := map[string]bool{}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		visited[real] = true
	}
	stack := []string{root}
	items := make([]fi.Item, 0, p.ChunkSize)
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrPermission) || dir != root {
				continue
			}
			return err
		}
		// Popped in order: the first entries first.
		var dirs []string
		for _, e := range entries {
			path := filepath.Join(dir, e.Name())
			isDir := e.IsDir()
			isLink := e.Type()&fs.ModeSymlink != 0
			if isLink && p.ExpandSymbolicLink {
				if st, err := os.Stat(path); err == nil && st.IsDir() {
					real, err := filepath.EvalSymlinks(path)
					if err != nil || visited[real] {
						continue
					}
					visited[real] = true
					isDir = true
				}
			}
			if isDir {
				if slices.Contains(p.IgnoredDirectories, e.Name()) {
					continue
				}
				if p.ExpandSymbolicLink && !isLink {
					// A directory a link entered already.
					real, err := filepath.EvalSymlinks(path)
					if err != nil || visited[real] {
						continue
					}
					visited[real] = true
				}
				dirs = append(dirs, path)
				continue
			}
			it := fi.Item{Action: file.ActionData{Path: path, IsLink: isLink}}
			if rel, err := filepath.Rel(root, path); err == nil {
				it.Word = filepath.ToSlash(rel)
			} else {
				it.Word = path
			}
			if info, err := e.Info(); err == nil {
				it.Status = &fi.Status{Size: info.Size(), Time: info.ModTime().UnixMilli()}
			}
			items = append(items, it)
			if len(items) >= p.ChunkSize {
				if err := emit(items); err != nil {
					return err
				}
				items = make([]fi.Item, 0, p.ChunkSize)
			}
		}
		slices.Reverse(dirs)
		stack = append(stack, dirs...)
	}
	return emit(items)
}
