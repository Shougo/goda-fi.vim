package ff

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// host keeps the last render.
type host struct {
	r     *render
	calls []string
}

func (h *host) Meta() sdk.Meta { return sdk.Meta{} }
func (h *host) Call(_ context.Context, fn string, _ ...any) (any, error) {
	h.calls = append(h.calls, fn)
	return nil, nil
}
func (h *host) Notify(_ context.Context, fn string, args ...any) error {
	if fn == "goda_fi#ui#ff#_redraw" {
		r := args[1].(render)
		h.r = &r
	}
	return nil
}
func (h *host) Eval(context.Context, string) (any, error) { return nil, nil }
func (h *host) Cmd(context.Context, string) error         { return nil }

func view(params fi.Params, items ...fi.Item) *fi.View {
	return &fi.View{
		Version:  1,
		Context:  fi.Context{Name: "t", Done: true, MaxItems: len(items)},
		UIParams: params,
		Items:    items,
		Selected: map[uint64]bool{},
	}
}

func TestRender(t *testing.T) {
	u := New()
	h := &host{}
	items := []fi.Item{
		{Word: "a", Highlights: []fi.Highlight{{HlGroup: "Search", Col: 1, Width: 1}}},
		{Word: "b\nc", Info: []fi.Info{{Text: "info", HlGroup: "Comment"}}},
	}
	v := view(fi.Params{"reversed": true, "maxDisplayItems": 5}, items...)
	v.Open, v.InitCursor = true, true
	if err := u.Redraw(context.Background(), h, v); err != nil {
		t.Fatal(err)
	}
	r := h.r
	if !reflect.DeepEqual(r.Lines, []string{"b c", "a"}) || !r.Open || r.Cursor != 2 {
		t.Errorf("render %+v", r)
	}
	if len(r.Highlights) != 1 || r.Highlights[0] != (hl{2, 1, 1, "Search", itemPriority}) {
		t.Errorf("highlights %v", r.Highlights)
	}
	if len(r.Info) != 1 || r.Info[0].Row != 1 {
		t.Errorf("info %+v", r.Info)
	}
	if r.Status != [2]string{"[goda-fi-t]", ""} {
		t.Errorf("status %q", r.Status)
	}

	// Not opened again; the input changed; a cursor asked for.
	v = view(fi.Params{"maxDisplayItems": 1}, items...)
	v.Version, v.Input, v.Context.Input, v.Context.Done = 2, "x", "x", false
	v.Selected = map[uint64]bool{items[0].ID(): true}
	if err := u.Redraw(context.Background(), h, v); err != nil {
		t.Fatal(err)
	}
	r = h.r
	if r.Open || !r.Reset || len(r.Lines) != 1 {
		t.Errorf("second render %+v", r)
	}
	if r.Status != [2]string{"[goda-fi-t] 1/2", " x [async]"} {
		t.Errorf("status %q", r.Status)
	}
	sel := r.Highlights[len(r.Highlights)-1]
	if sel != (hl{1, 1, 0, "Statement", selectedPriority}) {
		t.Errorf("selected %v", sel)
	}
}

func TestIgnoreEmpty(t *testing.T) {
	u := New()
	h := &host{}
	v := view(fi.Params{"ignoreEmpty": true})
	v.Open = true
	if err := u.Redraw(context.Background(), h, v); err != nil || h.r != nil {
		t.Fatalf("drawn: %v %+v", err, h.r)
	}
	v = view(fi.Params{"ignoreEmpty": true}, fi.Item{Word: "a"})
	if err := u.Redraw(context.Background(), h, v); err != nil || h.r == nil || !h.r.Open {
		t.Fatalf("not opened: %v %+v", err, h.r)
	}
}

func TestBadParams(t *testing.T) {
	if err := New().Redraw(context.Background(), &host{}, view(fi.Params{"nope": 1})); err == nil {
		t.Error("unknown params must be an error")
	}
}

func TestSourcePrefix(t *testing.T) {
	for _, c := range []struct {
		sources []string
		want    map[string]string
	}{
		{[]string{"file", "fileold", "filerec", "file"}, map[string]string{"file": "f", "fileold": "fold", "filerec": "frec"}},
		{[]string{"fileold", "filegit", "file"}, map[string]string{"fileold": "fold", "filegit": "fgit", "file": "f"}},
		{[]string{"filerec", "buffer"}, map[string]string{"filerec": "f", "buffer": "b"}},
		{[]string{"rg", "register"}, map[string]string{"rg": "rg", "register": "re"}},
		{[]string{"rg", "register", "line"}, map[string]string{"rg": "rg", "register": "re", "line": "l"}},
		{[]string{"buffer", "bufferlist"}, map[string]string{"buffer": "b", "bufferlist": "blist"}},
		{[]string{"line"}, map[string]string{"line": "l"}},
		{[]string{"ファイル", "ファイル古"}, map[string]string{"ファイル": "フ", "ファイル古": "フ古"}},
	} {
		got := shortNames(c.sources)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%v: got %v, want %v", c.sources, got, c.want)
		}
	}
	if got := sourcePrefixes("long", []string{"file"})["file"]; got != "[file] " {
		t.Errorf("long %q", got)
	}
	if got := sourcePrefixes("no", []string{"file"})["file"]; got != "" {
		t.Errorf("no %q", got)
	}
}
