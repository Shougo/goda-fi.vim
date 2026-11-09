// Package register is the source "register" of goda-fi.vim, as
// ddu-source-register: the registers that are not empty, of the kind "word".
package register

import (
	"context"
	_ "embed"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/internal/vimscript"
	"github.com/Shougo/goda-fi.vim/ext/kind/word"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// The Vim side of the source (register.vim): goda_fi#source#register#*().
//
//go:embed register.vim
var vimSrc []byte

var script = vimscript.New("autoload/goda_fi/source/register.vim", vimSrc)

type source struct{}

// New returns the source.
func New() fi.Source { return source{} }

func (source) Name() string { return "register" }
func (source) Kind() string { return "word" }

type reg struct {
	Name     string   `json:"name"`
	Contents []string `json:"contents"`
	Type     string   `json:"type"`
}

func (source) Gather(ctx context.Context, h sdk.Host, _ fi.GatherArgs, emit func([]fi.Item) error) error {
	raw, err := script.Call(ctx, h, "goda_fi#source#register#_list")
	if err != nil {
		return err
	}
	var regs []reg
	if err := sdk.Decode(raw, &regs); err != nil {
		return err
	}
	items := make([]fi.Item, 0, len(regs))
	for _, r := range regs {
		text := strings.Join(r.Contents, "\n")
		// (The special keys of a macro are not there: the Vim script
		// removed them, where they are bytes.)
		if text == "" {
			continue
		}
		shown := strings.ReplaceAll(text, "\n", `\n`)
		if rs := []rune(shown); len(rs) > 200 {
			shown = string(rs[:200])
		}
		items = append(items, fi.Item{
			Word:       r.Name + ": " + shown,
			Action:     word.ActionData{Text: text, RegType: r.Type},
			Highlights: []fi.Highlight{{Name: "header", HlGroup: "Special", Col: 1, Width: 2}},
		})
	}
	return emit(items)
}
