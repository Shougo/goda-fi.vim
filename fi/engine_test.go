package fi

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Shougo/goda.vim/sdk"
)

// fakeHost records what the plugin asks of Vim.
type fakeHost struct {
	mu    sync.Mutex
	calls []string
	// answer, when set, is the result of Call.
	answer func(fn string, args []any) any
}

func (h *fakeHost) Meta() sdk.Meta { return sdk.Meta{Host: sdk.HostNone} }
func (h *fakeHost) record(s string) {
	h.mu.Lock()
	h.calls = append(h.calls, s)
	h.mu.Unlock()
}
func (h *fakeHost) Call(_ context.Context, fn string, args ...any) (any, error) {
	h.record(fmt.Sprint(fn, args))
	if h.answer != nil {
		return h.answer(fn, args), nil
	}
	return nil, nil
}
func (h *fakeHost) Notify(_ context.Context, fn string, args ...any) error {
	h.record(fmt.Sprint(fn, args))
	return nil
}
func (h *fakeHost) Eval(context.Context, string) (any, error) { return nil, nil }
func (h *fakeHost) Cmd(_ context.Context, c string) error     { h.record(c); return nil }
func (h *fakeHost) log() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

// listSource emits its words in batches; with a gate, it waits for it
// before the last batch.
type listSource struct {
	name    string
	batches [][]string
	gate    chan struct{}
}

func (s *listSource) Name() string { return s.name }
func (s *listSource) Kind() string { return "word" }
func (s *listSource) Gather(ctx context.Context, _ sdk.Host, a GatherArgs, emit func([]Item) error) error {
	for i, b := range s.batches {
		if s.gate != nil && i == len(s.batches)-1 {
			select {
			case <-s.gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		items := make([]Item, len(b))
		for j, w := range b {
			items[j] = Item{Word: w, Action: map[string]any{"path": "/" + w}}
		}
		if err := emit(items); err != nil {
			return err
		}
	}
	return nil
}

// volatileSource gathers the input.
type volatileSource struct{}

func (volatileSource) Name() string { return "echo" }
func (volatileSource) Kind() string { return "word" }
func (volatileSource) Gather(_ context.Context, _ sdk.Host, a GatherArgs, emit func([]Item) error) error {
	return emit([]Item{{Word: "echo:" + a.Input}})
}

type substring struct{}

func (substring) Name() string { return "substring" }
func (substring) Filter(_ context.Context, _ sdk.Host, a FilterArgs) ([]Item, error) {
	var out []Item
	for _, it := range a.Items {
		if strings.Contains(it.MatcherKey(), a.Input) {
			out = append(out, it)
		}
	}
	return out, nil
}

type upper struct{}

func (upper) Name() string { return "upper" }
func (upper) Filter(_ context.Context, _ sdk.Host, a FilterArgs) ([]Item, error) {
	out := make([]Item, len(a.Items))
	for i, it := range a.Items {
		it.Display = strings.ToUpper(it.Word)
		out[i] = it
	}
	return out, nil
}

// wordKind records its actions.
type wordKind struct {
	mu  sync.Mutex
	ran []string
	res ActionResult
}

func (k *wordKind) Name() string { return "word" }
func (k *wordKind) Actions() map[string]Action {
	return map[string]Action{
		"echo": {Run: func(_ context.Context, _ sdk.Host, a ActionArgs) (ActionResult, error) {
			k.mu.Lock()
			defer k.mu.Unlock()
			var words []string
			for _, it := range a.Items {
				words = append(words, it.Word)
			}
			k.ran = append(k.ran, fmt.Sprintf("%s %v", strings.Join(words, ","), a.ActionParams["x"]))
			return k.res, nil
		}},
	}
}

// testUI hands the views to the test.
type testUI struct {
	views chan *View
	quits chan struct{}
}

func newTestUI() *testUI {
	return &testUI{views: make(chan *View, 100), quits: make(chan struct{}, 10)}
}

func (u *testUI) Name() string { return "test" }
func (u *testUI) Redraw(_ context.Context, _ sdk.Host, v *View) error {
	if debugViews {
		fmt.Printf("view %d done=%v input=%q items=%q\n", v.Version, v.Context.Done, v.Context.Input, words(v.Items))
	}
	u.views <- v
	return nil
}
func (u *testUI) Quit(context.Context, sdk.Host, *View) error {
	u.quits <- struct{}{}
	return nil
}
func (u *testUI) Actions() map[string]UIAction {
	return map[string]UIAction{
		"itemAction": func(ctx context.Context, _ sdk.Host, a UIActionArgs) (ActionFlags, error) {
			it, ok := a.Session.ItemAt(a.Version, a.Cursor)
			if !ok {
				return None, fmt.Errorf("no item")
			}
			return None, a.Session.ItemAction(ctx, "default", []Item{it}, a.ActionParams)
		},
		"expandItem": func(_ context.Context, _ sdk.Host, a UIActionArgs) (ActionFlags, error) {
			var p struct {
				Mode      string `json:"mode"`
				MaxLevel  int    `json:"maxLevel"`
				IsInTree  bool   `json:"isInTree"`
				IsGrouped bool   `json:"isGrouped"`
			}
			if err := Decode(a.ActionParams, &p); err != nil {
				return None, err
			}
			if a.Item.Expanded() && p.Mode == "toggle" {
				a.Session.CollapseItem(*a.Item)
				return None, nil
			}
			a.Session.ExpandItem(*a.Item, ExpandOptions{MaxLevel: p.MaxLevel, InTree: p.IsInTree, Grouped: p.IsGrouped})
			return Redraw, nil
		},
		"collapseItem": func(_ context.Context, _ sdk.Host, a UIActionArgs) (ActionFlags, error) {
			a.Session.CollapseItem(*a.Item)
			return None, nil
		},
		"toggleSelectItem": func(_ context.Context, _ sdk.Host, a UIActionArgs) (ActionFlags, error) {
			if it, ok := a.Session.ItemAt(a.Version, a.Cursor); ok {
				a.Session.ToggleSelect(it.ID())
			}
			return Redraw, nil
		},
	}
}

func words(items []Item) string {
	var w []string
	for _, it := range items {
		w = append(w, it.Text())
	}
	return strings.Join(w, " ")
}

// waitView waits for a view that satisfies ok, and returns it.
func waitView(t *testing.T, u *testUI, ok func(*View) bool) *View {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case v := <-u.views:
			if ok(v) {
				return v
			}
		case <-timeout:
			t.Fatal("no such view")
			return nil
		}
	}
}

func done(v *View) bool { return v.Context.Done }

type fixture struct {
	e    *engine
	h    *fakeHost
	ui   *testUI
	kind *wordKind
}

func newFixture(t *testing.T, sources ...Source) *fixture {
	t.Helper()
	f := &fixture{h: &fakeHost{}, ui: newTestUI(), kind: &wordKind{}}
	b := New().AddUI(f.ui).AddKind(f.kind).AddFilter(substring{}).AddFilter(upper{})
	for _, s := range sources {
		b.AddSource(s)
	}
	b.PatchGlobal(Options{
		UI:            "test",
		SourceOptions: map[string]SourceOptions{"_": {Matchers: []string{"substring"}}},
		KindOptions:   map[string]KindOptions{"word": {DefaultAction: "echo"}},
	})
	p, err := b.Plugin()
	if err != nil {
		t.Fatal(err)
	}
	f.e = p.(*engine)
	t.Cleanup(func() {
		f.e.mu.Lock()
		defer f.e.mu.Unlock()
		for _, s := range f.e.sessions {
			s.close()
		}
	})
	return f
}

func (f *fixture) call(t *testing.T, method string, params ...any) any {
	t.Helper()
	if params == nil {
		params = []any{}
	}
	gen, err := sdk.Generic(params)
	if err != nil {
		t.Fatal(err)
	}
	// As Vim sends them: integers are int64.
	v, err := f.e.Handlers()[method](context.Background(), f.h, normalize(gen).([]any))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return v
}

func normalize(v any) any {
	switch x := v.(type) {
	case interface{ Int64() (int64, error) }:
		if i, err := x.Int64(); err == nil {
			return i
		}
		return v
	case []any:
		for i := range x {
			x[i] = normalize(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = normalize(x[k])
		}
	}
	return v
}

func TestGatherAndFilter(t *testing.T) {
	gate := make(chan struct{})
	src := &listSource{name: "list", batches: [][]string{{"apple", "banana"}, {"cherry"}}, gate: gate}
	f := newFixture(t, src)
	f.call(t, "start", map[string]any{"name": "a", "sources": []any{"list"}}, map[string]any{}, map[string]any{})

	v := waitView(t, f.ui, func(v *View) bool { return len(v.Items) == 2 })
	if v.Context.Done || !v.InitCursor || words(v.Items) != "apple banana" {
		t.Errorf("first view: done %v init %v %q", v.Context.Done, v.InitCursor, words(v.Items))
	}
	close(gate)
	v = waitView(t, f.ui, done)
	if words(v.Items) != "apple banana cherry" || v.Context.MaxItems != 3 || v.InitCursor {
		t.Errorf("last view: %q %d", words(v.Items), v.Context.MaxItems)
	}

	f.call(t, "redraw", "a", map[string]any{"input": "an"})
	v = waitView(t, f.ui, func(v *View) bool { return v.Context.Input == "an" })
	if words(v.Items) != "banana" {
		t.Errorf("filtered: %q", words(v.Items))
	}
}

func TestVersionsGrow(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"a"}, {"b"}, {"c"}}})
	f.call(t, "start", map[string]any{"name": "v", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	var last int64
	v := waitView(t, f.ui, func(v *View) bool {
		if v.Version <= last {
			t.Errorf("version %d after %d", v.Version, last)
		}
		last = v.Version
		return v.Context.Done
	})
	if words(v.Items) != "a b c" {
		t.Errorf("items: %q", words(v.Items))
	}
}

func TestItemActionByVersion(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"apple", "banana", "cherry"}}})
	f.call(t, "start", map[string]any{"name": "x", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	v1 := waitView(t, f.ui, done)

	f.call(t, "redraw", "x", map[string]any{"input": "e"})
	waitView(t, f.ui, func(v *View) bool { return v.Context.Input == "e" }) // apple cherry

	// Row 2 of the first version is banana, though apple cherry is shown.
	f.call(t, "uiAction", "x", "itemAction", map[string]any{"x": 1}, map[string]any{"version": v1.Version, "cursor": 2})
	select {
	case <-f.ui.quits:
	case <-time.After(5 * time.Second):
		t.Fatal("not quit")
	}
	if got := fmt.Sprint(f.kind.ran); got != "[banana 1]" {
		t.Errorf("ran %s", got)
	}
}

func TestSelectAndActionNames(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"a", "b", "c"}}})
	f.call(t, "start", map[string]any{"name": "s", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, done)
	f.call(t, "uiAction", "s", "toggleSelectItem", map[string]any{}, map[string]any{"version": v.Version, "cursor": 1})
	f.call(t, "uiAction", "s", "toggleSelectItem", map[string]any{}, map[string]any{"version": v.Version, "cursor": 3})
	v = waitView(t, f.ui, func(v *View) bool { return len(v.Selected) == 2 })
	if !v.Selected[v.Items[0].ID()] || !v.Selected[v.Items[2].ID()] {
		t.Errorf("selected %v", v.Selected)
	}
	names := f.call(t, "itemActionNames", "s", []any{v.Items[0].ID()})
	if fmt.Sprint(names) != "[echo]" {
		t.Errorf("names %v", names)
	}
	// The selected items, without ids.
	f.call(t, "itemAction", "s", "echo", []any{}, map[string]any{})
	if got := fmt.Sprint(f.kind.ran); got != "[a,c <nil>]" {
		t.Errorf("ran %s", got)
	}
}

func TestResume(t *testing.T) {
	src := &listSource{name: "list", batches: [][]string{{"apple", "banana"}}}
	f := newFixture(t, src)
	f.call(t, "start", map[string]any{"name": "r", "sources": []any{"list"}, "input": "ban"}, map[string]any{}, map[string]any{})
	v1 := waitView(t, f.ui, done)
	f.call(t, "quit", "r")
	<-f.ui.quits

	f.call(t, "start", map[string]any{"name": "r", "resume": true}, map[string]any{}, map[string]any{})
	v2 := waitView(t, f.ui, done)
	if v2.Context.Input != "ban" || words(v2.Items) != "banana" || v2.Items[0].ID() != v1.Items[0].ID() || v2.InitCursor {
		t.Errorf("resumed: %q %q", v2.Context.Input, words(v2.Items))
	}
	// Without resume: gathered again.
	f.call(t, "start", map[string]any{"name": "r", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	v3 := waitView(t, f.ui, done)
	if words(v3.Items) != "apple banana" || v3.Items[0].ID() == v1.Items[0].ID() {
		t.Errorf("restarted: %q", words(v3.Items))
	}
}

func TestSync(t *testing.T) {
	src := &listSource{name: "list", batches: [][]string{{"a"}, {"b"}}}
	f := newFixture(t, src)
	f.call(t, "start", map[string]any{"name": "y", "sources": []any{"list"}, "sync": true}, map[string]any{}, map[string]any{})
	// start returned: every item is there, in one view.
	select {
	case v := <-f.ui.views:
		if !v.Context.Done || words(v.Items) != "a b" {
			t.Errorf("view %q", words(v.Items))
		}
	default:
		t.Fatal("no view after a sync start")
	}
}

func TestVolatileAndConverter(t *testing.T) {
	f := newFixture(t, volatileSource{})
	f.call(t, "start", map[string]any{
		"name": "vo", "sources": []any{"echo"},
		"sourceOptions": map[string]any{"echo": map[string]any{"volatile": true, "converters": []any{"upper"}}},
	}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, done)
	f.call(t, "redraw", "vo", map[string]any{"input": "zz"})
	v := waitView(t, f.ui, func(v *View) bool {
		return v.Context.Done && v.Context.Input == "zz" && len(v.Items) == 1 && v.Items[0].Word == "echo:zz"
	})
	if v.Items[0].Text() != "ECHO:ZZ" {
		t.Errorf("converted %q", v.Items[0].Text())
	}
}

func TestPersistAndSearchPath(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"a", "b"}}})
	f.kind.res = ActionResult{Flags: Persist, SearchPath: "/b"}
	f.call(t, "start", map[string]any{"name": "p", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, done)
	f.call(t, "itemAction", "p", "echo", []any{v.Items[0].ID()}, map[string]any{})
	v = waitView(t, f.ui, func(v *View) bool { return v.Cursor != 0 })
	if v.Cursor != v.Items[1].ID() {
		t.Errorf("cursor %d", v.Cursor)
	}
}

func TestOptionsOrder(t *testing.T) {
	f := newFixture(t)
	f.e.b.PatchLocal("loc", Options{SearchPath: "builder-local"})
	o, _, err := f.e.options(
		map[string]any{"name": "loc", "unique": true},
		map[string]any{
			"global": map[string]any{"searchPath": "vim-global", "input": "g"},
			"local":  map[string]any{"loc": map[string]any{"searchPath": "vim-local"}},
		})
	if err != nil {
		t.Fatal(err)
	}
	if o.SearchPath != "vim-local" || strOr(o.Input, "") != "g" || !boolOr(o.Unique, false) || o.UI != "test" {
		t.Errorf("options %+v", o)
	}
	if _, _, err := f.e.options(map[string]any{"sources": []any{"nope"}}, nil); err == nil {
		t.Error("an unknown source must be an error")
	}
}

func TestErrorsReported(t *testing.T) {
	f := newFixture(t, failing{})
	f.call(t, "start", map[string]any{"name": "e", "sources": []any{"fail"}}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, done)
	found := false
	for _, c := range f.h.log() {
		if strings.Contains(c, "goda_fi#util#print_error") && strings.Contains(c, "boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("not reported: %v", f.h.log())
	}
}

type failing struct{}

func (failing) Name() string { return "fail" }
func (failing) Kind() string { return "word" }
func (failing) Gather(context.Context, sdk.Host, GatherArgs, func([]Item) error) error {
	panic("boom")
}

var debugViews = false

// ackUI acknowledges its redraws only when the test says so.
type ackUI struct{ *testUI }

func (ackUI) Acks() bool { return true }

func TestAcks(t *testing.T) {
	ui := ackUI{newTestUI()}
	b := New().AddUI(ui).AddKind(&wordKind{}).AddSource(&listSource{name: "list", batches: [][]string{{"a"}, {"b"}}})
	b.PatchGlobal(Options{UI: "test"})
	p, err := b.Plugin()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{e: p.(*engine), h: &fakeHost{}, ui: ui.testUI}
	t.Cleanup(func() {
		for _, s := range f.e.sessions {
			s.close()
		}
	})
	f.call(t, "start", map[string]any{"name": "k", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, func(*View) bool { return true })
	select {
	case v2 := <-f.ui.views:
		t.Fatalf("drawn before the acknowledgement: %d after %d", v2.Version, v.Version)
	case <-time.After(200 * time.Millisecond):
	}
	f.call(t, "event", "k", "drawn", v.Version)
	v = waitView(t, f.ui, done)
	if words(v.Items) != "a b" {
		t.Errorf("items %q", words(v.Items))
	}
}

func TestPushPop(t *testing.T) {
	f := newFixture(t, &listSource{name: "one", batches: [][]string{{"a"}}}, &listSource{name: "two", batches: [][]string{{"b"}}})
	f.call(t, "start", map[string]any{"name": "pp", "sources": []any{"one"}}, map[string]any{}, map[string]any{})
	v1 := waitView(t, f.ui, done)
	f.call(t, "start", map[string]any{"name": "pp", "sources": []any{"two"}, "push": true}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, func(v *View) bool { return done(v) && words(v.Items) == "b" })
	f.call(t, "pop", "pp")
	v := waitView(t, f.ui, func(v *View) bool { return words(v.Items) == "a" })
	if !v.Open || v.Items[0].ID() != v1.Items[0].ID() {
		t.Errorf("popped %+v", v)
	}
	if got := f.call(t, "sessions"); fmt.Sprint(got) != "[pp]" {
		t.Errorf("sessions %v", got)
	}
}

func TestAliases(t *testing.T) {
	f := newFixture(t)
	f.e.b.AddSourceAs("other", &listSource{name: "list", batches: [][]string{{"x"}}})
	f.e.b.AddActionAlias("shout", "echo")
	f.call(t, "start", map[string]any{
		"name": "al", "sources": []any{"other"},
		"actionParams": map[string]any{"shout": map[string]any{"x": "loud"}},
	}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, done)
	if len(v.Items) != 1 || v.Items[0].SourceName() != "other" {
		t.Fatalf("items %+v", v.Items)
	}
	if names := f.call(t, "itemActionNames", "al", []any{v.Items[0].ID()}); fmt.Sprint(names) != "[echo shout]" {
		t.Errorf("names %v", names)
	}
	f.call(t, "itemAction", "al", "shout", []any{v.Items[0].ID()}, map[string]any{})
	if got := fmt.Sprint(f.kind.ran); got != "[x loud]" {
		t.Errorf("ran %s", got)
	}
}

func TestUIDefaultAction(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"x"}}})
	f.e.b.AddActionAlias("other", "echo")
	f.call(t, "start", map[string]any{
		"name": "ud", "sources": []any{"list"},
		"uiOptions":    map[string]any{"test": map[string]any{"defaultAction": "other"}},
		"actionParams": map[string]any{"other": map[string]any{"x": 2}},
	}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, done)
	f.call(t, "itemAction", "ud", "default", []any{v.Items[0].ID()}, map[string]any{})
	if got := fmt.Sprint(f.kind.ran); got != "[x 2]" {
		t.Errorf("ran %s", got)
	}
}

// slowVolatile gathers the same words for any input, a while after.
type slowVolatile struct{}

func (slowVolatile) Name() string { return "slow" }
func (slowVolatile) Kind() string { return "word" }
func (slowVolatile) Gather(ctx context.Context, _ sdk.Host, _ GatherArgs, emit func([]Item) error) error {
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	return emit([]Item{{Word: "apple"}, {Word: "banana"}})
}

// A volatile source is filtered by the matchers too (as in ddu.vim), and its
// items stay shown while it gathers again for a new input.
func TestVolatileMatchedAndKept(t *testing.T) {
	f := newFixture(t, slowVolatile{})
	f.call(t, "start", map[string]any{
		"name": "sv", "sources": []any{"slow"},
		"sourceOptions": map[string]any{"slow": map[string]any{"volatile": true}},
	}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, func(v *View) bool { return done(v) && len(v.Items) == 2 })
	f.call(t, "redraw", "sv", map[string]any{"input": "an"})
	v := waitView(t, f.ui, func(v *View) bool {
		if len(v.Items) == 0 {
			t.Errorf("version %d shows nothing (input %q, done %v)", v.Version, v.Context.Input, v.Context.Done)
		}
		return done(v) && v.Context.Input == "an"
	})
	if words(v.Items) != "banana" {
		t.Errorf("matched %q", words(v.Items))
	}
}

// echoNew is the source file with new: one item, of the input.
type echoNew struct{ delay time.Duration }

func (echoNew) Name() string { return "new" }
func (echoNew) Kind() string { return "word" }
func (s echoNew) Gather(ctx context.Context, _ sdk.Host, a GatherArgs, emit func([]Item) error) error {
	if a.Input == "" {
		return nil
	}
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	return emit([]Item{{Word: a.Input}})
}

// While the input is typed, the item of a volatile source is drawn once it
// answered for the new input, not first without it.
func TestVolatileNoGap(t *testing.T) {
	f := newFixture(t, echoNew{delay: 30 * time.Millisecond}, &listSource{name: "list", batches: [][]string{{"shou", "go"}}})
	f.call(t, "start", map[string]any{
		"name": "nw", "sources": []any{"new", "list"},
		"sourceOptions": map[string]any{"new": map[string]any{"volatile": true}},
	}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, done)
	for _, in := range []string{"s", "sh", "sho"} {
		f.call(t, "redraw", "nw", map[string]any{"input": in})
		v := waitView(t, f.ui, func(v *View) bool { return v.Context.Input == in })
		if len(v.Items) == 0 || v.Items[0].Word != in {
			t.Errorf("input %q drawn with %q", in, words(v.Items))
		}
		waitView(t, f.ui, func(v *View) bool { return v.Context.Input == in && done(v) })
	}
}

// expandInput: what filters is the input expanded by Vim; the UI gets what
// was typed.
func TestExpandInput(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"/home/a", "/tmp/b", "~/c"}}})
	f.h.answer = func(fn string, args []any) any {
		if fn == "expand" {
			return strings.ReplaceAll(args[0].(string), "~", "/home")
		}
		return nil
	}
	f.call(t, "start", map[string]any{
		"name": "ex", "sources": []any{"list"}, "expandInput": true, "input": "~/",
	}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, done)
	if words(v.Items) != "/home/a" || v.Context.Input != "/home/" || v.Input != "~/" {
		t.Errorf("start: %q %q %q", words(v.Items), v.Context.Input, v.Input)
	}
	// Not expanded: an input that starts with "<".
	f.call(t, "redraw", "ex", map[string]any{"input": "<~"})
	v = waitView(t, f.ui, func(v *View) bool { return v.Input == "<~" })
	if v.Context.Input != "<~" {
		t.Errorf("<: %q", v.Context.Input)
	}
	f.call(t, "redraw", "ex", map[string]any{"input": "~/c"})
	v = waitView(t, f.ui, func(v *View) bool { return v.Input == "~/c" })
	if v.Context.Input != "/home/c" || len(v.Items) != 0 {
		t.Errorf("redraw: %q %q", v.Context.Input, words(v.Items))
	}
	// Without the option, as typed.
	f.call(t, "start", map[string]any{
		"name": "noex", "sources": []any{"list"}, "input": "~/",
	}, map[string]any{}, map[string]any{})
	v = waitView(t, f.ui, func(v *View) bool { return v.Context.Name == "noex" && done(v) })
	if words(v.Items) != "~/c" {
		t.Errorf("not expanded: %q", words(v.Items))
	}
}

// Actions of the configuration and of Vim, over those of the kind and the
// UI.
func TestCustomActions(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"x"}}})
	var ran []string
	f.e.b.AddKindAction("word", "mine", Action{Run: func(_ context.Context, _ sdk.Host, a ActionArgs) (ActionResult, error) {
		ran = append(ran, "kind "+a.Items[0].Word)
		return ActionResult{}, nil
	}})
	f.e.b.AddSourceAction("list", "echo", Action{Run: func(_ context.Context, _ sdk.Host, a ActionArgs) (ActionResult, error) {
		ran = append(ran, "source echo")
		return ActionResult{}, nil
	}})
	f.e.b.AddUIAction("test", "hello", func(context.Context, sdk.Host, UIActionArgs) (ActionFlags, error) {
		ran = append(ran, "ui hello")
		return None, nil
	})
	f.h.answer = func(fn string, args []any) any {
		if fn == "goda_fi#_call_action" {
			return int64(Persist)
		}
		return nil
	}
	f.call(t, "start", map[string]any{"name": "ca", "sources": []any{"list"}},
		map[string]any{}, map[string]any{"actions": map[string]any{
			"kind": map[string]any{"word": []any{"grep"}},
			"ui":   map[string]any{"test": []any{"vimui"}},
		}})
	v := waitView(t, f.ui, done)
	id := v.Items[0].ID()
	if names := f.call(t, "itemActionNames", "ca", []any{id}); fmt.Sprint(names) != "[echo grep mine]" {
		t.Errorf("names %v", names)
	}
	f.call(t, "itemAction", "ca", "mine", []any{id}, map[string]any{})
	// The source's goes before the kind's.
	f.call(t, "itemAction", "ca", "echo", []any{id}, map[string]any{})
	f.call(t, "uiAction", "ca", "hello", map[string]any{}, map[string]any{})
	if fmt.Sprint(ran) != "[kind x source echo ui hello]" || len(f.kind.ran) != 0 {
		t.Errorf("ran %q, kind %q", ran, f.kind.ran)
	}
	// A Vim action: the function gets the items, its flags are followed
	// (Persist: shown again).
	f.call(t, "itemAction", "ca", "grep", []any{id}, map[string]any{"p": 1})
	waitView(t, f.ui, func(v *View) bool { return v.Open })
	var call string
	for _, c := range f.h.log() {
		if strings.HasPrefix(c, "goda_fi#_call_action") {
			call = c
		}
	}
	if !strings.HasPrefix(call, "goda_fi#_call_action[kind word grep map[actionParams:map[p:1] context:") ||
		!strings.Contains(call, "items:[map[") || !strings.Contains(call, "__id:") || !strings.Contains(call, "word:x") {
		t.Errorf("call %s", call)
	}
	f.call(t, "uiAction", "ca", "vimui", map[string]any{}, map[string]any{})
	if c := f.h.log()[len(f.h.log())-1]; !strings.HasPrefix(c, "goda_fi#_call_action[ui test vimui") {
		t.Errorf("ui call %s", c)
	}
}

func TestCustomActionUnknown(t *testing.T) {
	b := New().AddKindAction("nosuch", "x", Action{Run: func(context.Context, sdk.Host, ActionArgs) (ActionResult, error) {
		return ActionResult{}, nil
	}})
	if err := b.Validate(); err == nil || !strings.Contains(err.Error(), `unknown kind "nosuch"`) {
		t.Errorf("err %v", err)
	}
}

// treeSource is a tree of directories: the children of SourceOptions.Path.
type treeSource struct {
	open bool
	root string
}

var treeFiles = map[string][]string{
	"/":    {"a/", "b"},
	"/a":   {"c/", "d"},
	"/a/c": {"e"},
	// For isGrouped.
	"/top":       {"g/", "z"},
	"/top/g":     {"h/"},
	"/top/g/h":   {"i/"},
	"/top/g/h/i": {"j", "k"},
}

func (treeSource) Name() string { return "tree" }
func (treeSource) Kind() string { return "word" }
func (s treeSource) Gather(_ context.Context, _ sdk.Host, a GatherArgs, emit func([]Item) error) error {
	dir := a.SourceOptions.Path
	if dir == "" {
		dir = cmp.Or(s.root, "/")
	}
	var items []Item
	for _, w := range treeFiles[dir] {
		name := strings.TrimSuffix(w, "/")
		path := strings.TrimSuffix(dir, "/") + "/" + name
		it := Item{Word: w, TreePath: path, IsTree: strings.HasSuffix(w, "/")}
		if s.open && path == "/a" {
			it.IsExpanded = true
		}
		items = append(items, it)
	}
	return emit(items)
}

// tree is the items of a view as "word:level" ("+" expanded).
func tree(v *View) string {
	var out []string
	for _, it := range v.Items {
		s := fmt.Sprintf("%s:%d", it.Word, it.TreeLevel())
		if it.Expanded() {
			s += "+"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func treeIs(want string) func(*View) bool {
	return func(v *View) bool { return tree(v) == want }
}

func idOf(t *testing.T, v *View, word string) uint64 {
	t.Helper()
	for _, it := range v.Items {
		if it.Word == word {
			return it.ID()
		}
	}
	t.Fatalf("no %s in %s", word, tree(v))
	return 0
}

func TestTree(t *testing.T) {
	f := newFixture(t, treeSource{})
	ui := func(action string, id uint64, params map[string]any) {
		f.call(t, "uiAction", "tree", action, params, map[string]any{"id": id})
	}
	f.call(t, "start", map[string]any{"name": "tree", "sources": []any{"tree"}}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, treeIs("a/:0 b:0"))

	// Expanded: its children under it; the cursor on the first one.
	ui("expandItem", idOf(t, v, "a/"), map[string]any{"isInTree": true})
	v = waitView(t, f.ui, treeIs("a/:0+ c/:1 d:1 b:0"))
	if v.Cursor != idOf(t, v, "c/") {
		t.Errorf("isInTree: cursor %d", v.Cursor)
	}
	// Collapsed from a child: its parent; the cursor on it.
	ui("collapseItem", idOf(t, v, "d"), nil)
	v = waitView(t, f.ui, treeIs("a/:0 b:0"))
	if v.Cursor != idOf(t, v, "a/") {
		t.Errorf("collapse: cursor %d", v.Cursor)
	}
	// All the levels.
	ui("expandItem", idOf(t, v, "a/"), map[string]any{"maxLevel": -1})
	v = waitView(t, f.ui, treeIs("a/:0+ c/:1+ e:2 d:1 b:0"))
	// Collapsed and expanded again: what was expanded under it stays.
	ui("expandItem", idOf(t, v, "a/"), map[string]any{"mode": "toggle"})
	v = waitView(t, f.ui, treeIs("a/:0 b:0"))
	ui("expandItem", idOf(t, v, "a/"), nil)
	v = waitView(t, f.ui, treeIs("a/:0+ c/:1+ e:2 d:1 b:0"))
	// Gathered again: expanded again.
	f.call(t, "redraw", "tree", map[string]any{"refresh": true})
	v = waitView(t, f.ui, func(v *View) bool { return done(v) && tree(v) == "a/:0+ c/:1+ e:2 d:1 b:0" })
	// A filter applies to the children too.
	f.call(t, "redraw", "tree", map[string]any{"input": "/"})
	waitView(t, f.ui, treeIs("a/:0+ c/:1+"))
	// The items of the children act as the others.
	f.call(t, "itemAction", "tree", "default", []any{idOf(t, v, "e")}, map[string]any{})
	if fmt.Sprint(f.kind.ran) != "[e <nil>]" {
		t.Errorf("ran %q", f.kind.ran)
	}
}

// IsExpanded: expanded from the start.
func TestTreeIsExpanded(t *testing.T) {
	f := newFixture(t, treeSource{open: true})
	f.call(t, "start", map[string]any{"name": "open", "sources": []any{"tree"}}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, treeIs("a/:0+ c/:1 d:1 b:0"))
}

// The inputs of goda_fi#redraw() come in order (__seq): one older than
// one taken is dropped.
func TestRedrawInputOrder(t *testing.T) {
	f := newFixture(t, &listSource{name: "list", batches: [][]string{{"apple", "banana"}}})
	f.call(t, "start", map[string]any{"name": "ord", "sources": []any{"list"}}, map[string]any{}, map[string]any{})
	waitView(t, f.ui, done)
	f.call(t, "redraw", "ord", map[string]any{"input": "ban", "__seq": 2})
	f.call(t, "redraw", "ord", map[string]any{"input": "a", "__seq": 1})
	f.call(t, "redraw", "ord", map[string]any{"refresh": false})
	v := waitView(t, f.ui, func(v *View) bool { return v.Input != "" })
	for v.Input != "ban" || words(v.Items) != "banana" {
		v = waitView(t, f.ui, func(*View) bool { return true })
	}
	var input string
	_ = f.e.sessions["ord"].call(func(st *state) error { input = st.input; return nil })
	if input != "ban" {
		t.Errorf("input %q", input)
	}
}

// isGrouped: the trees that are the only item of their parent go in its
// line.
func TestTreeGrouped(t *testing.T) {
	f := newFixture(t, treeSource{root: "/top"})
	ui := func(action string, id uint64, params map[string]any) {
		f.call(t, "uiAction", "gr", action, params, map[string]any{"id": id})
	}
	f.call(t, "start", map[string]any{"name": "gr", "sources": []any{"tree"}}, map[string]any{}, map[string]any{})
	v := waitView(t, f.ui, treeIs("g/:0 z:0"))
	ui("expandItem", idOf(t, v, "g/"), map[string]any{"isGrouped": true})
	v = waitView(t, f.ui, treeIs("g/h/i/:0+ j:1 k:1 z:0"))
	if it := v.Items[0]; it.GroupedPath() != "g/h/" || it.TreePath != "/top/g/h/i" {
		t.Errorf("grouped %q %q", it.GroupedPath(), it.TreePath)
	}
	// Collapsed: the first of them.
	ui("collapseItem", idOf(t, v, "j"), nil)
	v = waitView(t, f.ui, treeIs("g/:0 z:0"))
	// Not grouped: one a line.
	ui("expandItem", idOf(t, v, "g/"), map[string]any{"maxLevel": -1})
	waitView(t, f.ui, treeIs("g/:0+ h/:1+ i/:2+ j:3 k:3 z:0"))
}
