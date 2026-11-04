package register

import (
	"context"
	"testing"

	"github.com/Shougo/goda-fi.vim/ext/kind/word"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type host struct{ regs []any }

func (h host) Meta() sdk.Meta                                    { return sdk.Meta{} }
func (h host) Call(context.Context, string, ...any) (any, error) { return h.regs, nil }
func (host) Notify(context.Context, string, ...any) error        { return nil }
func (host) Eval(context.Context, string) (any, error)           { return nil, nil }
func (host) Cmd(context.Context, string) error                   { return nil }

func TestGather(t *testing.T) {
	h := host{regs: []any{
		map[string]any{"name": "a", "contents": []any{"あ、一つ。", "考え"}, "type": "V"},
	}}
	var items []fi.Item
	if err := New().Gather(context.Background(), h, fi.GatherArgs{},
		func(it []fi.Item) error { items = append(items, it...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("%+v", items)
	}
	if a := items[0].Action.(word.ActionData); a.Text != "あ、一つ。\n考え" || a.RegType != "V" {
		t.Errorf("text %q", a.Text)
	}
	if items[0].Word != `a: あ、一つ。\n考え` {
		t.Errorf("word %q", items[0].Word)
	}
}
