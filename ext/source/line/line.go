// Package line is the source "line" of goda-fi.vim, as ddu-source-line: the
// lines of the buffer the session was started in, from the cursor line.
package line

import (
	"context"
	"strconv"

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

// chunkSize is how many lines are got and handed over at once: the first
// ones show before the lines of a large buffer are all got.
var chunkSize = 5000

type lines struct {
	BufNr  int      `json:"bufnr"`
	Begin  int      `json:"begin"`
	End    int      `json:"end"`
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
	raw, err := h.Call(ctx, "goda_fi#source#line#_lines", winID, bufNr, p.Range == "window", chunkSize)
	if err != nil {
		return err
	}
	var l lines
	if err := sdk.Decode(raw, &l); err != nil {
		return err
	}
	if l.End < l.Begin || len(l.Lines) == 0 {
		return nil
	}
	width := len(strconv.Itoa(l.End - l.Begin + 1))
	items := func(begin int, texts []string) []fi.Item {
		out := make([]fi.Item, len(texts))
		var buf []byte
		for i, text := range texts {
			lnum := begin + i
			buf = strconv.AppendInt(buf[:0], int64(lnum), 10)
			display := make([]byte, 0, width+2+len(text))
			for range width - len(buf) {
				display = append(display, '0')
			}
			display = append(display, buf...)
			display = append(display, ": "...)
			display = append(display, text...)
			out[i] = fi.Item{
				Word:    text,
				Display: string(display),
				Action:  file.ActionData{BufNr: l.BufNr, LineNr: lnum},
			}
		}
		return out
	}
	if err := emit(items(l.Cursor, l.Lines)); err != nil {
		return err
	}
	// The others: after the first ones to the end, then from the beginning
	// to the cursor line.
	get := func(begin, end int) error {
		for from := begin; from <= end; from += chunkSize {
			to := min(end, from+chunkSize-1)
			raw, err := h.Call(ctx, "goda_fi#source#line#_get", l.BufNr, from, to)
			if err != nil {
				return err
			}
			var texts []string
			if err := sdk.Decode(raw, &texts); err != nil {
				return err
			}
			if len(texts) == 0 {
				return nil
			}
			if err := emit(items(from, texts)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := get(l.Cursor+len(l.Lines), l.End); err != nil {
		return err
	}
	return get(l.Begin, l.Cursor-1)
}
