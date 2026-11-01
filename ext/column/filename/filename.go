// Package filename is the column "filename" of goda-fi.vim, as
// ddu-column-filename: the name of a file, indented by its level in the
// tree, after an icon (a directory collapsed or expanded, a link).
package filename

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the column params (columnParams.filename), as
// ddu-column-filename's.
type Params struct {
	CollapsedIcon    string `json:"collapsedIcon"`
	ExpandedIcon     string `json:"expandedIcon"`
	IconWidth        int    `json:"iconWidth"`
	IndentationWidth int    `json:"indentationWidth"`
	LinkIcon         string `json:"linkIcon"`
	// MaxSymlinkTargetWidth: the target of a link ("name -> target") is
	// cut there ("..."); 0: not cut.
	MaxSymlinkTargetWidth int        `json:"maxSymlinkTargetWidth"`
	Highlights            Highlights `json:"highlights"`
}

// Highlights are the groups of the parts: directoryIcon (Special),
// directoryName (Directory), linkIcon and linkName (Comment).
type Highlights struct {
	DirectoryIcon string `json:"directoryIcon,omitempty"`
	DirectoryName string `json:"directoryName,omitempty"`
	LinkIcon      string `json:"linkIcon,omitempty"`
	LinkName      string `json:"linkName,omitempty"`
}

// DefaultParams are ddu-column-filename's.
func DefaultParams() Params {
	return Params{
		CollapsedIcon:         "+",
		ExpandedIcon:          "-",
		IconWidth:             1,
		IndentationWidth:      1,
		LinkIcon:              "@",
		MaxSymlinkTargetWidth: 48,
	}
}

type column struct{ p Params }

// New returns the column with these params over the defaults (the column
// params of the options go over them): New(filename.DefaultParams()).
func New(p Params) fi.Column { return column{p} }

func (column) Name() string { return "filename" }

func (c column) params(a fi.ColumnArgs) (Params, error) {
	p := c.p
	err := fi.Decode(a.ColumnParams, &p)
	return p, err
}

func action(it fi.Item) file.ActionData {
	var ad file.ActionData
	if it.Action != nil {
		_ = sdk.Decode(it.Action, &ad)
	}
	return ad
}

// base is the name shown: "dir/" for a tree, "name -> target" for a link.
func base(it fi.Item, p Params) string {
	ad := action(it)
	path := ad.Path
	if path == "" {
		path = it.Word
	}
	name := it.GroupedPath() + filepath.Base(strings.TrimRight(path, `/\`))
	if it.IsTree {
		name += "/"
	}
	if ad.IsLink && ad.Path != "" {
		target, err := filepath.EvalSymlinks(ad.Path)
		if err != nil {
			if t, lerr := os.Readlink(ad.Path); lerr == nil {
				target = t
			} else {
				target = "?"
			}
		}
		name += " -> " + truncate(target, p.MaxSymlinkTargetWidth)
	}
	return name
}

func truncate(s string, max int) string {
	if max <= 0 || fi.StrWidth(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := fi.StrWidth(string(r))
		if w+rw > max {
			return b.String() + "..."
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

func indent(it fi.Item, p Params) int { return max(p.IndentationWidth*it.TreeLevel(), 0) }

func (c column) Length(_ context.Context, _ sdk.Host, a fi.ColumnArgs) (int, error) {
	p, err := c.params(a)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range a.Items {
		n = max(n, indent(it, p)+p.IconWidth+1+fi.StrWidth(base(it, p)))
	}
	return n, nil
}

func (c column) Text(_ context.Context, _ sdk.Host, a fi.ColumnArgs, it fi.Item, startCol, endCol int) (fi.ColumnText, error) {
	p, err := c.params(a)
	if err != nil {
		return fi.ColumnText{}, err
	}
	ad := action(it)
	name := base(it, p)
	icon := " "
	var iconHl, nameHl string
	switch {
	case it.IsTree:
		icon = p.CollapsedIcon
		if it.Expanded() {
			icon = p.ExpandedIcon
		}
		iconHl, nameHl = or(p.Highlights.DirectoryIcon, "Special"), or(p.Highlights.DirectoryName, "Directory")
	case ad.IsLink:
		icon = p.LinkIcon
		iconHl, nameHl = or(p.Highlights.LinkIcon, "Comment"), or(p.Highlights.LinkName, "Comment")
	}
	pad := strings.Repeat(" ", indent(it, p))
	text := pad + icon + " " + name
	var hls []fi.Highlight
	if iconHl != "" {
		col := startCol + len(pad)
		hls = append(hls,
			fi.Highlight{Name: "column-filename-icon", HlGroup: iconHl, Col: col, Width: len(icon)},
			fi.Highlight{Name: "column-filename-name", HlGroup: nameHl, Col: col + len(icon) + 1, Width: len(name)},
		)
	}
	if w := fi.StrWidth(text); w < endCol-startCol {
		text += strings.Repeat(" ", endCol-startCol-w)
	}
	return fi.ColumnText{Text: text, Highlights: hls}, nil
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
