// Package fileexternal is the source "fileexternal" of goda-fi.vim, as
// ddu-source-file_external: the files a command lists (fd, git ls-files, ...),
// one per line, relative to the directory it runs in (SourceOptions.Path, or
// the current one).
package fileexternal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	srcfile "github.com/Shougo/goda-fi.vim/ext/source/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the source params, as ddu-source-file_external's.
type Params struct {
	// Cmd is the command and its arguments: ["fd", ".", "-H", "-t", "f"].
	Cmd []string `json:"cmd"`
	// UpdateItems: the items are handed over by this many (the first
	// 1000 at once).
	UpdateItems int `json:"updateItems"`
}

type source struct{ p Params }

// New returns the source with these params (the source params of the
// options go over them).
func New(p Params) fi.Source {
	if p.UpdateItems <= 0 {
		p.UpdateItems = 50000
	}
	return source{p}
}

func (source) Name() string { return "fileexternal" }
func (source) Kind() string { return "file" }

func (s source) Gather(ctx context.Context, _ sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	p := s.p
	if err := fi.Decode(a.SourceParams, &p); err != nil {
		return err
	}
	if len(p.Cmd) == 0 {
		return errors.New("no cmd in the source params")
	}
	root := a.SourcePath()
	cmd := exec.CommandContext(ctx, p.Cmd[0], p.Cmd[1:]...)
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	size := 1000
	items := make([]fi.Item, 0, size)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var emitErr error
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		path := line
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		items = append(items, srcfile.Item(root, path, false))
		if len(items) >= size {
			if emitErr = emit(items); emitErr != nil {
				break
			}
			items = make([]fi.Item, 0, p.UpdateItems)
			size = p.UpdateItems
		}
	}
	if emitErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return emitErr
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", p.Cmd[0], err, msg)
		}
		return fmt.Errorf("%s: %w", p.Cmd[0], err)
	}
	return emit(items)
}
