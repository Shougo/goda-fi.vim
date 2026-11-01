package fi

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Shougo/goda.vim/sdk"
)

type engine struct {
	b *Builder

	mu       sync.Mutex
	sessions map[string]*session
	// pushed are the sessions put aside by "push", by name, last first.
	pushed      map[string][]*session
	nextID      uint64
	nextVersion int64
}

func newEngine(b *Builder) *engine {
	return &engine{b: b, sessions: map[string]*session{}, pushed: map[string][]*session{}}
}

func (e *engine) Name() string { return PluginName }

// handlerNames are the methods of the plugin (brokenPlugin fails them).
var handlerNames = []string{
	"start", "redraw", "uiAction", "itemAction", "itemActionNames",
	"getItem", "getItems", "getContext", "event", "quit", "pop", "sessions",
}

func (e *engine) Handlers() map[string]sdk.Handler {
	return map[string]sdk.Handler{
		// start(options, context, patches): starts (or resumes) the
		// session of options.name. Meant as a notification;
		// goda_fi#start() with sync waits for it. context is where Vim is
		// ({bufName, bufNr, winId, cwd, mode}); patches are those of
		// goda_fi#custom#patch_*() ({global, local: {name: patch}}).
		"start": e.start,
		// redraw(name, {input, refresh}): filters with input, gathers
		// again (refresh), and draws the UI.
		"redraw": e.redraw,
		// uiAction(name, action, params, {version, cursor, id}): runs a UI
		// action; cursor is the row of the cursor in the UI as drawn in
		// version, id the ID of the item there (0: none).
		"uiAction": e.uiAction,
		// itemAction(name, action, ids, params): runs an item action on the
		// items of these IDs (the selected ones, or that under the cursor,
		// when empty).
		"itemAction": e.itemAction,
		// itemActionNames(name, ids) -> the actions of the items.
		"itemActionNames": e.itemActionNames,
		// getItem(name, id) -> the item of this ID ({} if none).
		"getItem": e.getItem,
		// getItems(name) -> the items shown.
		"getItems": e.getItems,
		// getContext(name) -> the context of the session.
		"getContext": e.getContext,
		// event(name, event, version): "close" (the UI window was closed,
		// as drawn in version), "drawn" (version was drawn, UIAcker).
		"event": e.event,
		// quit(name): closes the UI.
		"quit": e.quitHandler,
		// pop(name): ends the session and resumes the one it replaced
		// with push.
		"pop": e.pop,
		// sessions() -> the names of the sessions.
		"sessions": e.sessionNames,
		"status":   func(context.Context, sdk.Host, []any) (any, error) { return "", nil },
	}
}

func (e *engine) session(name string) (*session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.sessions[name]
	if s == nil {
		return nil, fmt.Errorf("goda-fi: %w: %q", errNoSession, name)
	}
	return s, nil
}

func (e *engine) newIDs(n int) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	first := e.nextID + 1
	e.nextID += uint64(n)
	return first
}

// newVersion is the version of a new View: it grows across sessions.
func (e *engine) newVersion() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextVersion++
	return e.nextVersion
}

func argString(params []any, i int) string {
	if i < len(params) {
		if s, ok := params[i].(string); ok {
			return s
		}
	}
	return ""
}

func argMap(params []any, i int) map[string]any {
	if i < len(params) {
		if m, ok := params[i].(map[string]any); ok {
			return m
		}
	}
	return nil
}

// options are the options of a start: the builder's, Vim's patches and
// those of the start, merged.
func (e *engine) options(user, patches map[string]any) (Options, patch, error) {
	name, _ := user["name"].(string)
	p := mergePatch(patch{"name": "default"}, e.b.global)
	vimGlobal, _ := patches["global"].(map[string]any)
	vimLocal, _ := patches["local"].(map[string]any)
	if name == "" {
		name, _ = mergePatch(p, vimGlobal)["name"].(string)
	}
	p = mergePatch(p, e.b.local[name])
	p = mergePatch(p, vimGlobal)
	if l, ok := vimLocal[name].(map[string]any); ok {
		p = mergePatch(p, l)
	}
	p = mergePatch(p, user)
	o, err := e.check(p)
	return o, p, err
}

// check decodes merged options and checks them.
func (e *engine) check(p patch) (Options, error) {
	o, err := decodeOptions(p)
	if err != nil {
		return o, err
	}
	if errs := e.b.checkNames("goda_fi#start()", &o); len(errs) > 0 {
		return o, fmt.Errorf("%s", errs[0])
	}
	if o.UI == "" {
		return o, fmt.Errorf("no ui is set")
	}
	return o, nil
}

func (e *engine) start(ctx context.Context, h sdk.Host, params []any) (any, error) {
	user := argMap(params, 0)
	var c Context
	if err := Decode(argMap(params, 1), &c); err != nil {
		return nil, err
	}
	patches := argMap(params, 2)
	o, p, err := e.options(user, patches)
	if err != nil {
		return nil, fmt.Errorf("goda-fi: %w", err)
	}
	c.Name = o.Name
	e.mu.Lock()
	s := e.sessions[o.Name]
	if s != nil && boolOr(o.Push, false) {
		e.pushed[o.Name] = append(e.pushed[o.Name], s)
		s.do(func(st *state) { st.hide() })
		s = nil
	}
	if s == nil {
		s = newSession(e, o.Name, h)
		e.sessions[o.Name] = s
	}
	e.mu.Unlock()
	var wait <-chan struct{}
	if err := s.call(func(st *state) error {
		var err error
		wait, err = st.start(o, p, c, user, vimActionNames(patches["actions"]))
		return err
	}); err != nil {
		return nil, fmt.Errorf("goda-fi: %w", err)
	}
	if wait != nil { // sync: until every item is there
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, nil
}

func (e *engine) redraw(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, err
	}
	opts := argMap(params, 1)
	s.do(func(st *state) {
		if in, ok := opts["input"].(string); ok {
			st.setInput(in)
		}
		if r, _ := opts["refresh"].(bool); r {
			st.refresh()
			return
		}
		st.redrawNow()
	})
	return nil, nil
}

type actionPos struct {
	Version int64  `json:"version"`
	Cursor  int    `json:"cursor"`
	ID      uint64 `json:"id"`
}

func (e *engine) uiAction(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, err
	}
	name := argString(params, 1)
	var ap Params
	if m := argMap(params, 2); m != nil {
		ap = Params(m)
	}
	var pos actionPos
	if err := Decode(argMap(params, 3), &pos); err != nil {
		return nil, err
	}
	return nil, s.call(func(st *state) error { return st.uiAction(ctx, name, ap, pos) })
}

func ids(v any) []uint64 {
	list, _ := v.([]any)
	out := make([]uint64, 0, len(list))
	for _, x := range list {
		switch n := x.(type) {
		case int64:
			out = append(out, uint64(n))
		case uint64:
			out = append(out, n)
		case float64:
			out = append(out, uint64(n))
		case map[string]any: // an item dictionary
			out = append(out, ids([]any{n["__id"]})...)
		}
	}
	return out
}

func (e *engine) itemAction(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, err
	}
	name := argString(params, 1)
	var list []uint64
	if len(params) > 2 {
		list = ids(params[2])
	}
	var ap Params
	if m := argMap(params, 3); m != nil {
		ap = Params(m)
	}
	return nil, s.call(func(st *state) error {
		items := st.itemsByID(list)
		if len(list) == 0 {
			items = st.selectedItems()
		}
		return st.itemAction(ctx, name, items, ap)
	})
}

func (e *engine) itemActionNames(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, err
	}
	var out []string
	err = s.call(func(st *state) error {
		var list []uint64
		if len(params) > 1 {
			list = ids(params[1])
		}
		out = st.itemActionNames(st.itemsByID(list))
		return nil
	})
	return out, err
}

func (e *engine) getItem(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return map[string]any{}, nil
	}
	var list []uint64
	if len(params) > 1 {
		list = ids([]any{params[1]})
	}
	var out any = map[string]any{}
	_ = s.call(func(st *state) error {
		if items := st.itemsByID(list); len(items) > 0 {
			out = itemDict(items[0])
		}
		return nil
	})
	return out, nil
}

func (e *engine) getItems(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return []any{}, nil
	}
	out := []any{}
	_ = s.call(func(st *state) error {
		for _, it := range st.items {
			out = append(out, itemDict(it))
		}
		return nil
	})
	return out, nil
}

func (e *engine) getContext(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return map[string]any{}, nil
	}
	var c Context
	_ = s.call(func(st *state) error { c = st.ctx; return nil })
	return c, nil
}

func (e *engine) event(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, nil
	}
	ev := argString(params, 1)
	var version int64
	if len(params) > 2 {
		version, _ = params[2].(int64)
	}
	s.do(func(st *state) { st.onEvent(ev, version) })
	return nil, nil
}

func (e *engine) quitHandler(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.session(argString(params, 0))
	if err != nil {
		return nil, nil
	}
	return nil, s.call(func(st *state) error { return st.quit(ctx) })
}

func (e *engine) pop(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	name := argString(params, 0)
	e.mu.Lock()
	stack := e.pushed[name]
	cur := e.sessions[name]
	if len(stack) == 0 {
		e.mu.Unlock()
		return nil, nil
	}
	prev := stack[len(stack)-1]
	e.pushed[name] = stack[:len(stack)-1]
	e.sessions[name] = prev
	e.mu.Unlock()
	if cur != nil {
		_ = cur.call(func(st *state) error { st.hide(); return nil })
		cur.close()
	}
	return nil, prev.call(func(st *state) error { st.show(); return nil })
}

func (e *engine) sessionNames(context.Context, sdk.Host, []any) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := []string{}
	for n := range e.sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// vimActionNames are the names of the actions of goda_fi#custom#action(),
// as the patches carry them: {type: {name: [action, ...]}}.
func vimActionNames(v any) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	types, _ := v.(map[string]any)
	for typ, x := range types {
		names, _ := x.(map[string]any)
		for name, y := range names {
			list, _ := y.([]any)
			for _, a := range list {
				if s, ok := a.(string); ok {
					if out[typ] == nil {
						out[typ] = map[string][]string{}
					}
					out[typ][name] = append(out[typ][name], s)
				}
			}
		}
	}
	return out
}

// itemDict is an item as Vim sees it: with its ID (__id) and source.
func itemDict(it Item) map[string]any {
	d := map[string]any{"word": it.Word, "__id": it.id, "__sourceName": it.sourceName, "__sourceIndex": it.sourceIndex}
	if it.Display != "" {
		d["display"] = it.Display
	}
	if it.Action != nil {
		d["action"] = it.Action
	}
	if it.Kind != "" {
		d["kind"] = it.Kind
	}
	return d
}
