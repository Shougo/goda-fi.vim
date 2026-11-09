// Package word is the kind "word" of goda-fi.vim, as ddu-kind-word: items of
// text (ActionData), pasted, typed or yanked.
package word

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/internal/vimscript"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// The Vim side of the kind (word.vim): goda_fi#kind#word#*().
//
//go:embed word.vim
var vimSrc []byte

var script = vimscript.New("autoload/goda_fi/kind/word.vim", vimSrc)

// ActionData is the action of an item of this kind, as ddu-kind-word's.
type ActionData struct {
	// Text to paste or type.
	Text string `json:"text"`
	// RegType of the paste ("v", "V", "\x16{width}"); "v" when empty.
	RegType string `json:"regType,omitempty"`
	// Info is what the preview shows (Text when empty).
	Info string `json:"info,omitempty"`
}

// Kind is the kind "word".
type Kind struct{}

// New returns the kind, to add to the builder.
func New() Kind { return Kind{} }

func (Kind) Name() string { return "word" }

func texts(items []fi.Item) ([]ActionData, error) {
	out := make([]ActionData, 0, len(items))
	for _, it := range items {
		var ad ActionData
		if it.Action != nil {
			if err := sdk.Decode(it.Action, &ad); err != nil {
				return nil, fmt.Errorf("%s: %w", it.Word, err)
			}
		}
		if ad.Text == "" {
			ad.Text = it.Word
		}
		out = append(out, ad)
	}
	return out, nil
}

func vimAction(desc, fn string, flags fi.ActionFlags) fi.Action {
	return fi.Action{
		Description: desc,
		Run: func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
			ts, err := texts(a.Items)
			if err != nil {
				return fi.ActionResult{}, err
			}
			_, err = script.Call(ctx, h, "goda_fi#kind#word#"+fn, ts, a.Context.Mode)
			return fi.ActionResult{Flags: flags}, err
		},
	}
}

// Actions are ddu-kind-word's ("complete" is "feedkeys").
func (Kind) Actions() map[string]fi.Action {
	return map[string]fi.Action{
		"append":   vimAction("Paste the words like p.", "append", fi.None),
		"insert":   vimAction("Paste the words like P.", "insert", fi.None),
		"feedkeys": vimAction("Type the words with feedkeys().", "feedkeys", fi.None),
		"yank":     vimAction("Yank the words.", "yank", fi.Persist),
	}
}

// Preview shows the text (or Info).
func (Kind) Preview(_ context.Context, _ sdk.Host, a fi.PreviewArgs) (*fi.Previewer, error) {
	ts, err := texts([]fi.Item{a.Item})
	if err != nil {
		return nil, err
	}
	s := ts[0].Info
	if s == "" {
		s = ts[0].Text
	}
	return &fi.Previewer{Kind: "nofile", Contents: strings.Split(s, "\n")}, nil
}
