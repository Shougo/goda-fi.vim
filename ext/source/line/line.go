// Package line is the source "line" of goda-fi.vim, as ddu-source-line: the
// lines of the buffer the session was started in, from the cursor line.
package line

import (
	"context"
	"fmt"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the source params, as ddu-source-line's.
type Params struct {
	// BufNr: this buffer instead.
	BufNr int `json:"bufNr"`
	// IgnoreEmptyInput: nothing without an input (the option "input").
	IgnoreEmptyInput bool `json:"ignoreEmptyInput"`
	// Range: "buffer", or "window" (the lines shown).
	Range string `json:"range"`
	// WinID: this window instead.
	WinID int `json:"winId"`
}

type source struct{ p Params }

// New returns the source with these params (the source params of the options
// go over them).
func New(p Params) fi.Source {
	if p.Range == "" {
		p.Range = "buffer"
	}
	return source{p}
}

func (source) Name() string { return "line" }
func (source) Kind() string { return "file" }

type lines struct {
	BufNr  int      `json:"bufnr"`
	Begin  int      `json:"begin"`
	Cursor int      `json:"cursor"`
	Lines  []string `json:"lines"`
}

func (s source) Gather(ctx context.Context, h sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	p := s.p
	if err := fi.Decode(a.SourceParams, &p); err != nil {
		return err
	}
	if p.IgnoreEmptyInput && a.Context.Input == "" {
		return nil
	}
	winID, bufNr := p.WinID, p.BufNr
	if winID <= 0 {
		winID = a.Context.WinID
	}
	if bufNr <= 0 {
		bufNr = a.Context.BufNr
	}
	raw, err := h.Call(ctx, "goda_fi#source#line#_lines", winID, bufNr, p.Range == "window")
	if err != nil {
		return err
	}
	var l lines
	if err := sdk.Decode(raw, &l); err != nil {
		return err
	}
	width := len(fmt.Sprint(len(l.Lines)))
	var before, from []fi.Item
	for i, text := range l.Lines {
		lnum := l.Begin + i
		it := fi.Item{
			Word:    text,
			Display: fmt.Sprintf("%0*d: %s", width, lnum, text),
			Action:  file.ActionData{BufNr: l.BufNr, LineNr: lnum},
		}
		if lnum >= l.Cursor {
			from = append(from, it)
		} else {
			before = append(before, it)
		}
	}
	return emit(append(from, before...))
}
