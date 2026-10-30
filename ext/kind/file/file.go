// Package file is the kind "file" of goda-fi.vim, as ddu-kind-file: items of
// files and buffers (ActionData), opened, yanked, put in the quickfix list...
// and previewed.
//
// The work in Vim is done by autoload/goda_fi/kind/file.vim, one call per
// action.
package file

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// ActionData is the action of an item of this kind, as ddu-kind-file's.
type ActionData struct {
	// BufNr: the buffer (when it is one); Path otherwise.
	BufNr       int    `json:"bufNr,omitempty"`
	Path        string `json:"path,omitempty"`
	IsDirectory bool   `json:"isDirectory,omitempty"`
	IsLink      bool   `json:"isLink,omitempty"`
	// LineNr and Col: where the cursor goes (1-based).
	LineNr int `json:"lineNr,omitempty"`
	Col    int `json:"col,omitempty"`
	// Text: of the quickfix list.
	Text string `json:"text,omitempty"`
}

// Kind is the kind "file".
type Kind struct{}

// New returns the kind, to add to the builder.
func New() Kind { return Kind{} }

func (Kind) Name() string { return "file" }

// target is an item as Vim's helpers take it.
type target struct {
	ActionData
	Word string `json:"word"`
}

func targets(items []fi.Item) ([]target, error) {
	out := make([]target, 0, len(items))
	for _, it := range items {
		var t target
		if it.Action != nil {
			if err := sdk.Decode(it.Action, &t.ActionData); err != nil {
				return nil, fmt.Errorf("%s: %w", it.Word, err)
			}
		}
		t.Word = it.Word
		if t.Path == "" && t.BufNr == 0 {
			t.Path = it.Word
		}
		out = append(out, t)
	}
	return out, nil
}

// vimAction is an action done by goda_fi#kind#file#{fn}(targets, params,
// context) with these flags.
func vimAction(desc, fn string, flags fi.ActionFlags) fi.Action {
	return fi.Action{
		Description: desc,
		Run: func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
			ts, err := targets(a.Items)
			if err != nil {
				return fi.ActionResult{}, err
			}
			params := a.ActionParams
			if params == nil {
				params = fi.Params{}
			}
			_, err = h.Call(ctx, "goda_fi#kind#file#"+fn, ts, params, a.Context)
			return fi.ActionResult{Flags: flags}, err
		},
	}
}

// Actions are those of ddu-kind-file that do not change files (those come
// with the filer).
func (Kind) Actions() map[string]fi.Action {
	return map[string]fi.Action{
		"open": {
			Description: "Open the items (params: command, default \"edit\"; maxSize).",
			Run:         open,
		},
		"append":   vimAction("Paste the path like p.", "append", fi.None),
		"insert":   vimAction("Paste the path like P.", "insert", fi.None),
		"cd":       vimAction("chdir() to the directory of the item.", "cd", fi.None),
		"execute":  vimAction("Run params.command (default \"edit\") with the path.", "execute", fi.None),
		"feedkeys": vimAction("Type the path with feedkeys() (on the command line, say).", "feedkeys", fi.None),
		"loclist":  vimAction("Set the location list, and open it.", "loclist", fi.None),
		"quickfix": vimAction("Set the quickfix list, and open it.", "quickfix", fi.None),
		"yank":     vimAction("Yank the path.", "yank", fi.Persist),
	}
}

// open opens the items; binary and large files after a confirmation.
func open(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	var p struct {
		Command string `json:"command"`
		MaxSize int64  `json:"maxSize"`
	}
	p.Command = "edit"
	p.MaxSize = 500000
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.ActionResult{}, err
	}
	ts, err := targets(a.Items)
	if err != nil {
		return fi.ActionResult{}, err
	}
	var open []target
	for _, t := range ts {
		if t.BufNr == 0 && t.Path != "" {
			if st, err := os.Stat(t.Path); err == nil {
				if st.IsDir() {
					_, _ = h.Call(ctx, "goda_fi#kind#file#print", t.Path+" is a directory.")
					continue
				}
				var ask string
				if isBinary(t.Path) {
					ask = fmt.Sprintf("%q has binary code.  Open?", t.Path)
				} else if st.Size() > p.MaxSize {
					ask = fmt.Sprintf("%q: %d bytes are too huge.  Open?", t.Path, st.Size())
				}
				if ask != "" {
					yes, err := h.Call(ctx, "goda_fi#kind#file#confirm", ask)
					if err != nil {
						return fi.ActionResult{}, err
					}
					if yes != true {
						continue
					}
				}
			}
		}
		open = append(open, t)
	}
	if len(open) == 0 {
		return fi.ActionResult{}, nil
	}
	_, err = h.Call(ctx, "goda_fi#kind#file#open", open, fi.Params{"command": p.Command}, a.Context)
	return fi.ActionResult{}, err
}

// isBinary: a NUL in the first 8000 bytes, as git says.
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := io.ReadFull(f, buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// Preview is the file (or buffer) of the item at its line.
func (Kind) Preview(_ context.Context, _ sdk.Host, a fi.PreviewArgs) (*fi.Previewer, error) {
	ts, err := targets([]fi.Item{a.Item})
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	t := ts[0]
	if t.BufNr > 0 {
		return &fi.Previewer{Kind: "buffer", Expr: t.BufNr, LineNr: t.LineNr}, nil
	}
	// Sources give absolute paths (this process has no idea of Vim's
	// current directory).
	path := t.Path
	st, err := os.Stat(path)
	switch {
	case err != nil:
		return &fi.Previewer{Kind: "nofile", Contents: []string{path + " is not found"}}, nil
	case st.IsDir():
		return &fi.Previewer{Kind: "nofile", Contents: []string{path + " is a directory"}}, nil
	case isBinary(path):
		return &fi.Previewer{Kind: "nofile", Contents: []string{path + " is a binary file"}}, nil
	}
	return &fi.Previewer{Kind: "buffer", Path: path, LineNr: t.LineNr}, nil
}
