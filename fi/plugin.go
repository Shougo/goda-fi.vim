package fi

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Shougo/goda.vim/sdk"
	"github.com/Shougo/goda.vim/sdk/custom"
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
	// The versions start at the time the binary starts: those of a binary
	// started again (goda#build(), goda#stop()) are newer than those of the
	// one before, which the UIs of Vim still have (a version not newer than
	// theirs is not drawn). In microseconds: far fewer Views than that, and
	// exact in a float too (< 2^53).
	return &engine{b: b, sessions: map[string]*session{}, pushed: map[string][]*session{},
		nextVersion: time.Now().UnixMicro()}
}

func (e *engine) Name() string { return PluginName }

// ScriptVersion is the version of the Vim script (autoload/goda_fi.vim) this
// binary goes with (sdk.ScriptVersioner): goda.vim refuses a binary of
// another one and says to build it again. Bump it with s:version there when
// the Vim script and the binary no longer work together.
const ScriptVersion = 3

func (e *engine) ScriptVersion() int { return ScriptVersion }

// handlerNames are the methods of the plugin (brokenPlugin fails them).
var handlerNames = []string{
	"start", "redraw", "uiAction", "itemAction", "itemActionNames",
	"getItem", "getItems", "getContext", "getCurrent", "event", "quit", "pop", "sessions",
}

func (e *engine) Handlers() map[string]sdk.Handler {
	return map[string]sdk.Handler{
		// start(options, context): starts (or resumes) the session of
		// options.name. Meant as a notification; goda_fi#start() with sync
		// waits for it. context is where Vim is ({bufName, bufNr, winId,
		// cwd, mode}).
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
		// getCurrent(name) -> the options of the session, as set
		// (ddu#custom#get_current()); {} without the session.
		"getCurrent": e.getCurrent,
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

// sessionArg is the session of the first param (its name); rest are the
// other params, as sdk.DecodeArgs.
func (e *engine) sessionArg(params []any, rest ...any) (*session, error) {
	var name string
	if err := sdk.DecodeArgs(params, append([]any{&name}, rest...)...); err != nil {
		return nil, err
	}
	return e.session(name)
}

// options are the options of a start: the builder's and those of the start,
// merged.
func (e *engine) options(user map[string]any) (Options, patch, error) {
	o, p, err := e.b.custom.Resolve(user, custom.Request{})
	if err == nil {
		err = e.validate(&o)
	}
	return o, p, err
}

// check decodes merged options and checks them.
func (e *engine) check(p patch) (Options, error) {
	o, err := decodeOptions(p)
	if err != nil {
		return o, err
	}
	return o, e.validate(&o)
}

// validate checks the names of decoded options and the UI.
func (e *engine) validate(o *Options) error {
	if errs := e.b.checkNames("goda_fi#start()", o); len(errs) > 0 {
		return fmt.Errorf("%s", errs[0])
	}
	if o.UI == "" {
		return fmt.Errorf("no ui is set")
	}
	return nil
}

func (e *engine) start(ctx context.Context, h sdk.Host, params []any) (any, error) {
	var user, context map[string]any
	if err := sdk.DecodeArgs(params, &user, sdk.Optional(&context)); err != nil {
		return nil, err
	}
	var c Context
	if err := Decode(context, &c); err != nil {
		return nil, err
	}
	o, p, err := e.options(user)
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
		wait, err = st.start(o, p, c, user)
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
	var opts map[string]any
	s, err := e.sessionArg(params, sdk.Optional(&opts))
	if err != nil {
		return nil, err
	}
	// goda.vim handles the notifications in the order they were sent:
	// the inputs typed come in order.
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
	var name string
	var ap Params
	var pos actionPos
	s, err := e.sessionArg(params, &name, sdk.Optional(&ap), sdk.Optional(&pos))
	if err != nil {
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
	var name string
	var rawIDs any
	var ap Params
	s, err := e.sessionArg(params, &name, sdk.Optional(&rawIDs), sdk.Optional(&ap))
	if err != nil {
		return nil, err
	}
	list := ids(rawIDs)
	return nil, s.call(func(st *state) error {
		items := st.itemsByID(list)
		if len(list) == 0 {
			items = st.selectedItems()
		}
		return st.itemAction(ctx, name, items, ap)
	})
}

func (e *engine) itemActionNames(_ context.Context, _ sdk.Host, params []any) (any, error) {
	var rawIDs any
	s, err := e.sessionArg(params, sdk.Optional(&rawIDs))
	if err != nil {
		return nil, err
	}
	var out []string
	err = s.call(func(st *state) error {
		out = st.itemActionNames(st.itemsByID(ids(rawIDs)))
		return nil
	})
	return out, err
}

func (e *engine) getItem(_ context.Context, _ sdk.Host, params []any) (any, error) {
	var id any
	s, err := e.sessionArg(params, sdk.Optional(&id))
	if err != nil {
		return map[string]any{}, nil
	}
	list := ids([]any{id})
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
	s, err := e.sessionArg(params)
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

// contextOf is the context of the session name (GatherArgs.ContextOf).
func (e *engine) contextOf(name string) (Context, bool) {
	s, err := e.session(name)
	if err != nil {
		return Context{}, false
	}
	var c Context
	if err := s.call(func(st *state) error { c = st.ctx; return nil }); err != nil {
		return Context{}, false
	}
	return c, true
}

func (e *engine) getContext(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.sessionArg(params)
	if err != nil {
		return map[string]any{}, nil
	}
	var c Context
	_ = s.call(func(st *state) error { c = st.ctx; return nil })
	return c, nil
}

func (e *engine) getCurrent(_ context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.sessionArg(params)
	if err != nil {
		return map[string]any{}, nil
	}
	var p patch
	_ = s.call(func(st *state) error { p = st.patch; return nil })
	return dropNil(p), nil
}

// dropNil is a patch without its unset values (null in Vim).
func dropNil(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if e != nil {
				out[k] = dropNil(e)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			if e != nil {
				out = append(out, dropNil(e))
			}
		}
		return out
	}
	return v
}

func (e *engine) event(_ context.Context, _ sdk.Host, params []any) (any, error) {
	var ev string
	var version int64
	s, err := e.sessionArg(params, &ev, sdk.Optional(&version))
	if err != nil {
		return nil, nil
	}
	s.do(func(st *state) { st.onEvent(ev, version) })
	return nil, nil
}

func (e *engine) quitHandler(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	s, err := e.sessionArg(params)
	if err != nil {
		return nil, nil
	}
	return nil, s.call(func(st *state) error { return st.quit(ctx) })
}

func (e *engine) pop(ctx context.Context, _ sdk.Host, params []any) (any, error) {
	var name string
	if err := sdk.DecodeArgs(params, &name); err != nil {
		return nil, err
	}
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
	if it.TreePath != "" {
		d["treePath"] = it.TreePath
	}
	d["isTree"] = it.IsTree
	d["__level"] = it.level
	d["__expanded"] = it.expanded
	return d
}
