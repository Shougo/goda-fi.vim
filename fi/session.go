package fi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Shougo/goda.vim/sdk"
)

// redrawInterval is the least time between two redraws while items arrive.
var redrawInterval = 50 * time.Millisecond

// ackTimeout is how long a redraw waits for the acknowledgement of the one
// before (UIAcker) at most.
var ackTimeout = 2 * time.Second

// holdMax is how long a redraw waits for the volatile sources to answer for
// a new input at most.
var holdMax = 300 * time.Millisecond

// keptViews is how many of the last views ItemAt knows.
const keptViews = 8

// session runs the state of one session name in one goroutine: do and call
// queue functions that the goroutine runs in order. Nothing else touches the
// state.
type session struct {
	name string

	mu     sync.Mutex
	queue  []func(*state)
	wake   chan struct{}
	closed bool

	base   context.Context
	cancel context.CancelFunc
	st     *state
}

func newSession(e *engine, name string, h sdk.Host) *session {
	base, cancel := context.WithCancel(context.Background())
	s := &session{name: name, wake: make(chan struct{}, 1), base: base, cancel: cancel}
	s.st = &state{s: s, e: e, raw: h, name: name, selected: map[uint64]bool{}, views: map[int64][]Item{},
		expanded: map[treeKey]bool{}, nodes: map[treeKey]*treeNode{}}
	if h != nil {
		s.st.h = actorHost{h, s}
	}
	go s.run()
	return s
}

// do queues f; it never blocks (a queue without bound: sources must not
// wait for the UI).
func (s *session) do(f func(*state)) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, f)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// call runs f in the goroutine of the session and waits for it. It must not
// be called from that goroutine.
func (s *session) call(f func(*state) error) error {
	done := make(chan error, 1)
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return fmt.Errorf("goda-fi: %w: %q", errNoSession, s.name)
	}
	s.do(func(st *state) { done <- f(st) })
	select {
	case err := <-done:
		return err
	case <-s.base.Done():
		return fmt.Errorf("goda-fi: %w: %q", errNoSession, s.name)
	}
}

// close ends the session: its gathering and its goroutine.
func (s *session) close() {
	s.mu.Lock()
	s.closed = true
	s.queue = nil
	s.mu.Unlock()
	s.cancel()
}

func (s *session) run() {
	for {
		select {
		case <-s.wake:
		case <-s.base.Done():
			return
		}
		s.drain()
	}
}

// drain runs what is queued, in order, until nothing is.
func (s *session) drain() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		f := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		s.mu.Unlock()
		s.safe(f)
	}
}

// actorHost is the host of the goroutine of a session. While it waits for
// Vim, the session runs what is queued: Vim, asked something, may ask the
// session something before it answers (an autocommand of a window closed
// by the UI sends a request, say), and the session must answer it then.
type actorHost struct {
	sdk.Host
	s *session
}

// wait waits for f (a call of Vim), running what the session is asked
// meanwhile.
func (h actorHost) wait(f func() (any, error)) (any, error) {
	type result struct {
		v   any
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := f()
		done <- result{v, err}
	}()
	for {
		select {
		case r := <-done:
			return r.v, r.err
		case <-h.s.wake:
			h.s.drain()
		case <-h.s.base.Done():
			r := <-done
			return r.v, r.err
		}
	}
}

func (h actorHost) Call(ctx context.Context, fn string, args ...any) (any, error) {
	return h.wait(func() (any, error) { return h.Host.Call(ctx, fn, args...) })
}

func (h actorHost) Eval(ctx context.Context, expr string) (any, error) {
	return h.wait(func() (any, error) { return h.Host.Eval(ctx, expr) })
}

func (h actorHost) Cmd(ctx context.Context, cmd string) error {
	_, err := h.wait(func() (any, error) { return nil, h.Host.Cmd(ctx, cmd) })
	return err
}

// safe runs f, reporting a panic instead of ending the program.
func (s *session) safe(f func(*state)) {
	defer func() {
		if p := recover(); p != nil {
			s.st.report(fmt.Errorf("panic: %v", p))
		}
	}()
	f(s.st)
}

// gathering is the gathering of one source.
type gathering struct {
	// at: when it started.
	at     time.Time
	gen    int
	cancel context.CancelFunc
	done   bool
	items  []Item
	// stale: items are those of the gathering before (a volatile source
	// gathering again for a new input), shown until new ones come, so that
	// the UI does not show the source empty in between.
	stale bool
}

// treeKey is a tree item: its source and its TreePath.
type treeKey struct {
	src  int
	path string
}

// treeNode is the gathering of the children of a tree item.
type treeNode struct {
	// parent is the item expanded.
	parent Item
	// stale: items are those of the gathering before, shown until new
	// ones come.
	stale  bool
	at     time.Time
	gen    int
	cancel context.CancelFunc
	done   bool
	items  []Item
	// depth is how many levels under it are expanded with it (negative:
	// all), inTree: the cursor goes to its first child.
	depth  int
	inTree bool
}

// state is a session: the items gathered, the input, the selection, and
// what the UI shows.
type state struct {
	s *session
	e *engine
	// h is the host of what runs in the goroutine of the session (actions,
	// the UI, filters): while it waits for Vim, it runs what is queued
	// (actorHost). raw is the host itself, for the other goroutines (the
	// sources).
	h    sdk.Host
	raw  sdk.Host
	name string

	started bool
	patch   patch
	opts    Options
	ctx     Context
	ui      UI

	src []*gathering
	// input filters; typed is what was typed (input before expandInput).
	input string
	typed string
	// vimActions are the actions of goda_fi#custom#action(): their names,
	// by type ("kind", "source", "ui") and extension name.
	vimActions map[string]map[string][]string
	// items are the items filtered, as last drawn (or about to be).
	items []Item
	dirty bool

	selected map[uint64]bool

	version   int64
	views     map[int64][]Item
	viewOrder []int64
	visible   bool
	sync      bool
	waiters   []chan struct{}
	timer     *time.Timer
	lastDraw  time.Time
	initCur   bool
	open      bool
	lastOpen  int64
	// holding are the volatile sources gathering for a new input; the UI is
	// not drawn before they answered, or holdUntil.
	holding   map[int]bool
	holdUntil time.Time
	// inflight is the version drawn and not acknowledged yet (UIAcker),
	// since inflightAt; deferred: a redraw waits for it.
	inflight   int64
	inflightAt time.Time
	deferred   bool
	search     string
	errShown   map[string]bool
	gatherGen  int
	// expanded are the tree items expanded (false: collapsed; an item not
	// there is as its IsExpanded says), nodes the gatherings of their
	// children. cursor: the item the cursor goes to at the next view.
	expanded map[treeKey]bool
	nodes    map[treeKey]*treeNode
	cursor   uint64
	// grouped are the trees expanded with isGrouped.
	grouped map[treeKey]bool
	// roots are the root items of the sources (View.Roots), rootIDs
	// their IDs by source and path.
	roots   []Item
	rootIDs map[treeKey]uint64
	// paths are the paths of the sources set by actions (ActionResult.Path),
	// by index: a resume keeps them.
	paths map[int]string
}

// start begins the session with options o (the merged patch p), or resumes
// it. With the option sync, the channel returned is closed once every item
// is there.
func (st *state) start(o Options, p patch, c Context, user patch, vimActions map[string]map[string][]string) (<-chan struct{}, error) {
	st.vimActions = vimActions
	resume := boolOr(o.Resume, false) && st.started
	if resume {
		// The options of the session, with those of this start over.
		u := maps.Clone(user)
		delete(u, "resume")
		p = mergePatch(st.patch, u)
		var err error
		if o, err = st.e.check(p); err != nil {
			return nil, err
		}
	}
	if st.started && st.visible && boolOr(o.uiOptions().Toggle, false) {
		return nil, st.quit(st.s.base)
	}
	if st.ui != nil && st.ui.Name() != o.UI && st.visible {
		_ = st.quit(st.s.base)
	}
	if !resume {
		st.paths = nil
	}
	st.patch, st.opts = p, o
	for i, path := range st.paths {
		st.setSourcePath(i, path)
	}
	st.ui = st.e.b.uis[o.UI]
	c.Name = o.Name
	c.Path = c.Cwd
	if resume {
		c.Input, c.Done, c.MaxItems, c.Path = st.ctx.Input, st.ctx.Done, st.ctx.MaxItems, st.ctx.Path
		c.PathHistories = st.ctx.PathHistories
	}
	if !resume && len(o.Sources) > 0 {
		if p := o.sourceOptions(0).Path; p != "" {
			// That of the sources, as ddu.vim's.
			c.Path = abs(&c, p)
		}
	}
	st.ctx = c
	st.visible = true
	st.open = true
	st.sync = boolOr(o.Sync, false)
	st.search = o.SearchPath
	st.errShown = map[string]bool{}
	switch {
	case o.Input != nil:
		st.typed = *o.Input
		st.input = st.expand(st.typed)
	case !resume:
		st.typed, st.input = "", ""
	}
	st.ctx.Input = st.input
	st.dirty = true

	var wait chan struct{}
	if st.sync {
		wait = make(chan struct{})
		st.waiters = append(st.waiters, wait)
	}
	if !resume || boolOr(o.Refresh, false) || len(st.src) != len(o.Sources) {
		if !resume {
			st.selected = map[uint64]bool{}
			st.initCur = true
			st.expanded = map[treeKey]bool{}
		}
		st.started = true
		st.gather(nil, false)
	} else {
		st.redrawNow()
	}
	st.checkDone()
	if wait == nil {
		return nil, nil
	}
	return wait, nil
}

// gather gathers the sources of these indexes again (all, for nil). With
// keep, their items stay shown until new ones come.
func (st *state) gather(indexes []int, keep bool) {
	n := len(st.opts.Sources)
	if indexes == nil || len(st.src) != n {
		for _, g := range st.src {
			if g != nil && g.cancel != nil {
				g.cancel()
			}
		}
		st.src = make([]*gathering, n)
		indexes = make([]int, n)
		for i := range indexes {
			indexes[i] = i
		}
	}
	st.ctx.Done = false
	st.dirty = true
	for _, i := range indexes {
		if keep {
			// The trees expanded are gathered again too, their items
			// shown until new ones come.
			for k, n := range st.nodes {
				if k.src == i {
					st.regatherNode(n)
				}
			}
		} else {
			// The children are gathered again as their parents come
			// back (expanded stays).
			st.dropNodes(func(k treeKey) bool { return k.src == i })
		}
		old := st.src[i]
		if old != nil && old.cancel != nil {
			old.cancel()
		}
		st.gatherGen++
		ctx, cancel := context.WithCancel(st.s.base)
		g := &gathering{gen: st.gatherGen, cancel: cancel, at: time.Now()}
		if keep && old != nil {
			g.items, g.stale = old.items, true
		}
		st.src[i] = g
		go st.gatherSource(ctx, i, g.gen, st.gatherArgs(i))
	}
	if len(indexes) == 0 {
		st.checkDone()
		return
	}
	if !keep {
		// The items of the sources gathered again are gone: show it.
		st.scheduleRedraw()
	}
}

func (st *state) gatherArgs(i int) GatherArgs {
	c := st.ctx
	o := st.opts
	return GatherArgs{
		Context:       &c,
		Options:       &o,
		SourceOptions: st.opts.sourceOptions(i),
		SourceParams:  st.opts.sourceParams(i),
		Input:         st.input,
		ContextOf:     st.e.contextOf,
	}
}

// gatherSource runs in a goroutine of its own: what it gathers goes to the
// session with do.
func (st *state) gatherSource(ctx context.Context, i, gen int, a GatherArgs) {
	spec := a.Options.Sources[i]
	src := st.e.b.sources[spec.Name]
	key := a.SourceOptions.MatcherKey
	err := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("panic: %v", p)
			}
		}()
		if in, ok := src.(SourceIniter); ok {
			if err := in.OnInit(ctx, st.raw, a); err != nil {
				return err
			}
		}
		return src.Gather(ctx, st.raw, a, func(items []Item) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(items) == 0 {
				return nil
			}
			out := make([]Item, len(items))
			first := st.e.newIDs(len(items))
			for j, it := range items {
				it.id = first + uint64(j)
				it.sourceIndex = i
				it.sourceName = spec.Name
				it.level = it.Level
				if it.Kind == "" {
					it.Kind = src.Kind()
				}
				it.matcherKey = it.Word
				if key == "display" && it.Display != "" {
					it.matcherKey = it.Display
				}
				out[j] = it
			}
			st.s.do(func(st *state) {
				if i < len(st.src) && st.src[i] != nil && st.src[i].gen == gen {
					g := st.src[i]
					if g.stale {
						g.items, g.stale = nil, false
					}
					g.items = append(g.items, out...)
					st.dirty = true
					st.autoExpand(out, 0)
					if st.holding[i] {
						st.release(i)
					} else {
						st.scheduleRedraw()
					}
				}
			})
			return nil
		})
	}()
	st.s.do(func(st *state) {
		if i >= len(st.src) || st.src[i] == nil || st.src[i].gen != gen {
			return
		}
		st.src[i].done = true
		if st.src[i].stale {
			// Nothing came for the new input.
			st.src[i].items, st.src[i].stale = nil, false
			st.dirty = true
		}
		if st.holding[i] {
			st.release(i)
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			st.report(fmt.Errorf("source %s: %w", spec.Name, err))
		}
		st.checkDone()
	})
}

// checkDone ends the gathering when every source is done: the last redraw,
// and those waiting for it (sync).
func (st *state) checkDone() {
	for _, g := range st.src {
		if g == nil || !g.done {
			return
		}
	}
	if st.ctx.Done && len(st.waiters) == 0 && st.timer == nil && !st.dirty {
		return
	}
	st.ctx.Done = true
	st.redrawNow()
	for _, w := range st.waiters {
		close(w)
	}
	st.waiters = nil
}

func (st *state) allDone() bool {
	for _, g := range st.src {
		if g == nil || !g.done {
			return false
		}
	}
	return true
}

// scheduleRedraw redraws soon: at once if the last redraw is old enough,
// else when it is.
func (st *state) scheduleRedraw() {
	if !st.visible || st.timer != nil || (st.sync && !st.allDone()) {
		return
	}
	wait := redrawInterval - time.Since(st.lastDraw)
	if wait <= 0 {
		st.redrawNow()
		return
	}
	st.timer = time.AfterFunc(wait, func() {
		st.s.do(func(st *state) {
			st.timer = nil
			st.redrawNow()
		})
	})
}

func (st *state) stopTimer() {
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
}

// filter filters the items of every source.
func (st *state) filter(ctx context.Context) {
	if !st.dirty {
		return
	}
	st.dirty = false
	o := &st.opts
	c := st.ctx
	var all []Item
	total := 0
	for i, g := range st.src {
		if g == nil {
			continue
		}
		total += len(g.items)
		so := o.sourceOptions(i)
		items := g.items
		// As ddu.vim: the items of a volatile source are filtered too.
		items = st.runFilters(ctx, &c, so, so.Matchers, items)
		items = st.runFilters(ctx, &c, so, so.Sorters, items)
		items = st.runFilters(ctx, &c, so, so.Converters, items)
		if max := intOr(so.MaxItems, 10000); len(items) > max {
			items = items[:max]
		}
		start := len(all)
		all = st.flatten(ctx, &c, so, i, items, all, &total, -1)
		st.applyColumns(ctx, &c, so, all[start:])
	}
	all = st.runFilters(ctx, &c, SourceOptions{}, o.PostFilters, all)
	if boolOr(o.Unique, false) {
		seen := map[string]bool{}
		out := all[:0:0]
		for _, it := range all {
			if !seen[it.Word] {
				seen[it.Word] = true
				out = append(out, it)
			}
		}
		all = out
	}
	st.ctx.MaxItems = total
	st.items = all
}

func (st *state) runFilters(ctx context.Context, c *Context, so SourceOptions, names []string, items []Item) []Item {
	for _, name := range names {
		f := st.e.b.filters[name]
		if f == nil {
			continue
		}
		fo := st.opts.filterOptions(name)
		if len([]rune(st.input)) < intOr(fo.MinInputLength, 0) {
			continue
		}
		out, err := f.Filter(ctx, st.h, FilterArgs{
			Context:       c,
			Options:       &st.opts,
			SourceOptions: so,
			FilterOptions: fo,
			FilterParams:  st.opts.filterParams(name),
			Input:         st.input,
			Items:         items,
		})
		if err != nil {
			st.report(fmt.Errorf("filter %s: %w", name, err))
			continue
		}
		items = out
	}
	return items
}

// view is the state to draw, as a new version.
func (st *state) view() *View {
	st.filter(st.s.base)
	// Versions grow across the sessions of a name (push and pop share
	// the UI).
	st.version = st.e.newVersion()
	st.views[st.version] = st.items
	st.viewOrder = append(st.viewOrder, st.version)
	if len(st.viewOrder) > keptViews {
		delete(st.views, st.viewOrder[0])
		st.viewOrder = st.viewOrder[1:]
	}
	v := &View{
		Version:    st.version,
		Context:    st.ctx,
		Options:    &st.opts,
		UIOptions:  st.opts.uiOptions(),
		UIParams:   st.opts.uiParams(),
		Items:      st.items,
		Selected:   maps.Clone(st.selected),
		InitCursor: st.initCur,
		Open:       st.open,
		Input:      st.typed,
	}
	for _, s := range st.opts.Sources {
		v.Sources = append(v.Sources, s.Name)
	}
	if st.search != "" {
		for _, it := range st.items {
			if itemPath(it) == st.search {
				v.Cursor = it.id
				st.search = ""
				break
			}
		}
	}
	if st.cursor != 0 {
		for _, it := range st.items {
			if it.id == st.cursor {
				v.Cursor = it.id
				st.cursor = 0
				break
			}
		}
	}
	v.Roots = st.rootItems()
	return v
}

// currentView is the view last drawn, without a new version.
func (st *state) currentView() *View {
	v := &View{
		Version:   st.version,
		Context:   st.ctx,
		Options:   &st.opts,
		UIOptions: st.opts.uiOptions(),
		UIParams:  st.opts.uiParams(),
		Items:     st.views[st.version],
		Selected:  maps.Clone(st.selected),
		Input:     st.typed,
	}
	for _, s := range st.opts.Sources {
		v.Sources = append(v.Sources, s.Name)
	}
	return v
}

// itemPath is the path of the action of an item ("path"), if it has one.
func itemPath(it Item) string {
	if m, ok := it.Action.(map[string]any); ok {
		p, _ := m["path"].(string)
		return p
	}
	var a struct {
		Path string `json:"path"`
	}
	if it.Action != nil && sdk.Decode(it.Action, &a) == nil {
		return a.Path
	}
	return ""
}

// redrawNow draws the UI.
func (st *state) redrawNow() {
	st.stopTimer()
	if !st.visible || st.ui == nil || (st.sync && !st.allDone()) {
		return
	}
	if len(st.holding) > 0 {
		if wait := time.Until(st.holdUntil); wait > 0 {
			if st.timer == nil {
				st.timer = time.AfterFunc(wait, func() {
					st.s.do(func(st *state) {
						st.timer = nil
						st.holding = nil
						st.redrawNow()
					})
				})
			}
			return
		}
		st.holding = nil
	}
	acker, acks := st.ui.(UIAcker)
	acks = acks && acker.Acks()
	if acks && st.inflight != 0 {
		// Vim has not drawn the last one yet: draw what there is then.
		if wait := ackTimeout - time.Since(st.inflightAt); wait > 0 {
			st.deferred = true
			st.timer = time.AfterFunc(wait, func() {
				st.s.do(func(st *state) {
					st.timer = nil
					st.inflight = 0
					st.redrawNow()
				})
			})
			return
		}
	}
	st.deferred = false
	v := st.view()
	st.lastDraw = time.Now()
	if acks {
		st.inflight, st.inflightAt = v.Version, st.lastDraw
	}
	if st.open {
		st.lastOpen = v.Version
	}
	st.open = false
	if len(v.Items) > 0 || v.Context.Done {
		st.initCur = false
	}
	if err := st.ui.Redraw(st.s.base, st.h, v); err != nil {
		st.report(fmt.Errorf("ui %s: %w", st.ui.Name(), err))
	}
}

// report shows an error in Vim, once per message for a start.
func (st *state) report(err error) {
	msg := "goda-fi: " + err.Error()
	if st.errShown != nil {
		if st.errShown[msg] {
			return
		}
		st.errShown[msg] = true
	}
	log.Print(msg)
	if st.h != nil {
		_ = st.h.Notify(st.s.base, "goda_fi#util#print_error", msg)
	}
}

// expand is the input expanded by Vim, with the option expandInput.
func (st *state) expand(input string) string {
	if !boolOr(st.opts.ExpandInput, false) || input == "" || strings.HasPrefix(input, "<") || st.h == nil {
		// expand() breaks an input that starts with "<" (as ddu.vim).
		return input
	}
	r, err := st.h.Call(st.s.base, "expand", input)
	if s, ok := r.(string); ok && err == nil {
		return s
	}
	return input
}

func (st *state) setInput(typed string) {
	if typed == st.typed {
		return
	}
	st.typed = typed
	input := st.expand(typed)
	if st.typed != typed {
		// Another input came while Vim expanded this one (actorHost): it
		// is newer.
		return
	}
	st.dirty = true
	if input == st.input {
		return
	}
	st.input = input
	st.ctx.Input = input
	var volatile []int
	for i := range st.opts.Sources {
		if boolOr(st.opts.sourceOptions(i).Volatile, false) {
			volatile = append(volatile, i)
		}
	}
	if len(volatile) > 0 {
		st.gather(volatile, true)
		// As ddu.vim: drawn once they answered for the new input (what they
		// gathered for the old one may not match it), within holdMax.
		st.holding = map[int]bool{}
		for _, i := range volatile {
			st.holding[i] = true
		}
		st.holdUntil = time.Now().Add(holdMax)
	}
}

// release ends the wait for the volatile source i (it answered).
func (st *state) release(i int) {
	if !st.holding[i] {
		return
	}
	delete(st.holding, i)
	if len(st.holding) == 0 {
		st.stopTimer()
		st.redrawNow()
	}
}

func (st *state) refresh() {
	if !st.started {
		return
	}
	st.gather(nil, false)
}

// hide stops drawing (the session was pushed, or popped): another session of
// the name has the UI.
func (st *state) hide() {
	st.stopTimer()
	st.holding = nil
	st.visible = false
	st.inflight, st.deferred = 0, false
}

// show draws the UI again (after pop).
func (st *state) show() {
	st.visible = true
	st.open = true
	st.dirty = true
	st.redrawNow()
}

// quit closes the UI; the session stays, to be resumed.
func (st *state) quit(ctx context.Context) error {
	st.stopTimer()
	st.holding = nil
	st.inflight, st.deferred = 0, false
	if !st.visible || st.ui == nil {
		st.visible = false
		return nil
	}
	st.visible = false
	return st.ui.Quit(ctx, st.h, st.currentView())
}

func (st *state) onEvent(ev string, version int64) {
	switch ev {
	case "close":
		if version < st.lastOpen {
			return // closed before it was opened again
		}
		st.visible = false
		st.stopTimer()
		st.inflight, st.deferred = 0, false
	case "drawn":
		if st.inflight == 0 || version < st.inflight {
			return
		}
		st.inflight = 0
		if st.deferred {
			st.stopTimer()
			st.deferred = false
			st.redrawNow()
		}
	}
}

func (st *state) itemAt(version int64, row int) (Item, bool) {
	items, ok := st.views[version]
	if !ok {
		items = st.views[st.version]
	}
	if row < 1 || row > len(items) {
		return Item{}, false
	}
	return items[row-1], true
}

// itemsByID are the items of these IDs, in that order.
func (st *state) itemsByID(list []uint64) []Item {
	if len(list) == 0 {
		return nil
	}
	want := map[uint64]int{}
	for i, id := range list {
		want[id] = i
	}
	out := make([]Item, len(list))
	found := make([]bool, len(list))
	for _, g := range st.src {
		if g == nil {
			continue
		}
		for _, it := range g.items {
			if i, ok := want[it.id]; ok {
				out[i], found[i] = it, true
			}
		}
	}
	for _, n := range st.nodes {
		for _, it := range n.items {
			if i, ok := want[it.id]; ok {
				out[i], found[i] = it, true
			}
		}
	}
	for _, it := range st.roots {
		if i, ok := want[it.id]; ok {
			out[i], found[i] = it, true
		}
	}
	// A converter may have changed them: those shown first.
	for _, it := range st.items {
		if i, ok := want[it.id]; ok {
			out[i], found[i] = it, true
		}
	}
	res := out[:0]
	for i, it := range out {
		if found[i] {
			res = append(res, it)
		}
	}
	return res
}

func (st *state) selectedItems() []Item {
	var out []Item
	for _, it := range st.items {
		if st.selected[it.id] {
			out = append(out, it)
		}
	}
	return out
}

// actions are the item actions of items, and what they are run with.
type actionSet struct {
	kind    string
	source  int
	actions map[string]Action
	// aliases from KindOptions.Actions
	aliases map[string]string
	def     string
}

func (st *state) actionsOf(items []Item) (*actionSet, error) {
	if len(items) == 0 {
		return nil, errors.New("no items")
	}
	kind, src := items[0].Kind, items[0].sourceIndex
	for _, it := range items[1:] {
		if it.Kind != kind {
			return nil, fmt.Errorf("items of different kinds: %s and %s", kind, it.Kind)
		}
		if it.sourceName != items[0].sourceName {
			return nil, fmt.Errorf("items of different sources: %s and %s", items[0].sourceName, it.sourceName)
		}
	}
	k := st.e.b.kinds[kind]
	if k == nil {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	// As ddu.vim: those of the kind, then its custom ones, then those of
	// the source and its custom ones. Custom: of the configuration, then of
	// Vim (goda_fi#custom#action()).
	set := &actionSet{kind: kind, source: src, actions: map[string]Action{}}
	maps.Copy(set.actions, k.Actions())
	st.customActions(set.actions, "kind", kind)
	srcName := items[0].sourceName
	if s, ok := st.e.b.sources[srcName].(SourceActioner); ok {
		maps.Copy(set.actions, s.Actions())
	}
	st.customActions(set.actions, "source", srcName)
	ko := st.opts.kindOptions(kind)
	set.aliases = maps.Clone(st.e.b.actionAliases)
	if set.aliases == nil {
		set.aliases = map[string]string{}
	}
	maps.Copy(set.aliases, ko.Actions)
	set.def = ko.DefaultAction
	if src < len(st.opts.Sources) {
		if d := st.opts.sourceOptions(src).DefaultAction; d != "" {
			set.def = d
		}
	}
	return set, nil
}

// customActions adds the custom actions of the extension name of type typ
// ("kind" or "source") to actions.
func (st *state) customActions(actions map[string]Action, typ, name string) {
	maps.Copy(actions, st.e.b.customActions[typ][name])
	for _, a := range st.vimActions[typ][name] {
		actions[a] = vimAction(typ, name, a)
	}
}

// vimAction is the action of goda_fi#custom#action(): the Vim function gets
// {context, options, actionParams, items} (the items as
// goda_fi#ui#get_item() returns them) and returns the action flags (a
// Number; anything else is 0).
func vimAction(typ, name, action string) Action {
	return Action{
		Description: "goda_fi#custom#action()",
		Run: func(ctx context.Context, h sdk.Host, a ActionArgs) (ActionResult, error) {
			items := make([]any, len(a.Items))
			for i, it := range a.Items {
				items[i] = itemDict(it)
			}
			flags, err := callVimAction(ctx, h, typ, name, action, map[string]any{
				"context":      a.Context,
				"options":      a.Options,
				"actionParams": nonNil(a.ActionParams),
				"items":        items,
			})
			return ActionResult{Flags: flags}, err
		},
	}
}

func callVimAction(ctx context.Context, h sdk.Host, typ, name, action string, args map[string]any) (ActionFlags, error) {
	r, err := h.Call(ctx, "goda_fi#_call_action", typ, name, action, args)
	if err != nil {
		return None, err
	}
	switch n := r.(type) {
	case int64:
		return ActionFlags(n), nil
	case float64:
		return ActionFlags(n), nil
	case int:
		return ActionFlags(n), nil
	}
	return None, nil
}

func nonNil(p Params) Params {
	if p == nil {
		return Params{}
	}
	return p
}

// names are the names of the actions, sorted, as the option actions allows.
func (a *actionSet) names(allowed []string) []string {
	var out []string
	for n := range a.actions {
		out = append(out, n)
	}
	for n, to := range a.aliases {
		if _, ok := a.actions[to]; ok {
			out = append(out, n)
		}
	}
	if allowed != nil {
		out = slices.DeleteFunc(out, func(n string) bool { return !slices.Contains(allowed, n) })
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (st *state) itemActionNames(items []Item) []string {
	set, err := st.actionsOf(items)
	if err != nil {
		return []string{}
	}
	return set.names(st.opts.Actions)
}

// itemAction runs the item action name on items, then what its result asks
// for.
func (st *state) itemAction(ctx context.Context, name string, items []Item, params Params) error {
	if len(items) == 0 {
		return nil
	}
	set, err := st.actionsOf(items)
	if err != nil {
		return err
	}
	if name == "" || name == "default" {
		name = set.def
		if d := st.opts.uiOptions().DefaultAction; d != "" {
			name = d
		}
		if name == "" {
			return fmt.Errorf("no default action for kind %q", set.kind)
		}
	}
	if st.opts.Actions != nil && !slices.Contains(st.opts.Actions, name) {
		return fmt.Errorf("action %q is not allowed", name)
	}
	actual := name
	if to, ok := set.aliases[name]; ok {
		actual = to
	}
	act, ok := set.actions[actual]
	if !ok {
		return fmt.Errorf("unknown action %q of kind %q", name, set.kind)
	}
	uo := st.opts.uiOptions()
	quit := boolOr(st.opts.actionOptions(name).Quit, true) && !boolOr(uo.Persist, false)
	if quit {
		if err := st.quit(ctx); err != nil {
			return err
		}
	}
	res, err := act.Run(ctx, st.h, ActionArgs{
		Context:       &st.ctx,
		Options:       &st.opts,
		SourceOptions: st.opts.sourceOptions(set.source),
		SourceParams:  st.opts.sourceParams(set.source),
		KindOptions:   st.opts.kindOptions(set.kind),
		KindParams:    st.opts.kindParams(set.kind),
		ActionParams:  st.opts.actionParams(name, params),
		Items:         items,
	})
	if err != nil {
		return fmt.Errorf("action %s: %w", name, err)
	}
	if res.SearchPath != "" {
		st.search = res.SearchPath
	}
	if res.Path != "" && set.source < len(st.opts.Sources) && st.withinLimit(set.source, res.Path) {
		if st.paths == nil {
			st.paths = map[int]string{}
		}
		st.paths[set.source] = res.Path
		if prev := sourcePath(&st.ctx, st.opts.sourceOptions(set.source)); prev != "" && prev != res.Path {
			st.ctx.PathHistories = append(slices.Clone(st.ctx.PathHistories), prev)
		}
		st.setSourcePath(set.source, res.Path)
		st.typed, st.input, st.ctx.Input = "", "", ""
		st.dirty = true
	}
	if quit && res.Flags&Persist != 0 {
		st.visible = true
		st.open = true
	}
	switch {
	case res.Flags&RefreshItems != 0 && res.Path == "":
		// The same places: what is shown stays until the new items come
		// (a file pasted, deleted...).
		st.gather(allIndexes(len(st.opts.Sources)), true)
	case res.Flags&RefreshItems != 0:
		st.gather(nil, false)
	case res.Flags&(Redraw|Persist) != 0 || !quit || res.SearchPath != "":
		st.dirty = true
		st.redrawNow()
	}
	return nil
}

// uiAction runs the UI action name.
func (st *state) uiAction(ctx context.Context, name string, params Params, pos actionPos) error {
	if st.ui == nil {
		return errors.New("no ui")
	}
	act := st.uiActionOf(name)
	if act == nil {
		return fmt.Errorf("unknown action %q of ui %s", name, st.ui.Name())
	}
	a := UIActionArgs{
		View:         st.currentView(),
		ActionParams: mergeParams(params),
		Cursor:       pos.Cursor,
		Version:      pos.Version,
		Session:      sessionAPI{st, ctx},
	}
	if pos.ID != 0 {
		if items := st.itemsByID([]uint64{pos.ID}); len(items) > 0 {
			a.Item = &items[0]
		}
	} else if it, ok := st.itemAt(pos.Version, pos.Cursor); ok && pos.Cursor > 0 {
		a.Item = &it
	}
	flags, err := act(ctx, st.h, a)
	if err != nil {
		return err
	}
	switch {
	case flags&RefreshItems != 0:
		st.gather(nil, false)
	case flags&Redraw != 0:
		st.dirty = true
		st.redrawNow()
	}
	return nil
}

// uiActionOf is the UI action name: of Vim (goda_fi#custom#action()), of
// the configuration (Builder.AddUIAction), or of the UI, in this order (as
// ddu.vim).
func (st *state) uiActionOf(name string) UIAction {
	ui := st.ui.Name()
	if slices.Contains(st.vimActions["ui"][ui], name) {
		return func(ctx context.Context, h sdk.Host, a UIActionArgs) (ActionFlags, error) {
			items := []any{}
			if a.Item != nil {
				items = append(items, itemDict(*a.Item))
			}
			return callVimAction(ctx, h, "ui", ui, name, map[string]any{
				"context":      a.View.Context,
				"options":      a.View.Options,
				"actionParams": nonNil(a.ActionParams),
				"items":        items,
			})
		}
	}
	if act := st.e.b.customUIActions[ui][name]; act != nil {
		return act
	}
	return st.ui.Actions()[name]
}

// sessionAPI is the Session of UI actions: they run in the goroutine of the
// session, so it works on the state directly.
type sessionAPI struct {
	st  *state
	ctx context.Context
}

func (a sessionAPI) ItemAction(ctx context.Context, name string, items []Item, params Params) error {
	return a.st.itemAction(ctx, name, items, params)
}

func (a sessionAPI) ItemActionNames(items []Item) []string { return a.st.itemActionNames(items) }

func (a sessionAPI) SetInput(input string) {
	a.st.setInput(input)
	a.st.scheduleRedraw()
}

func (a sessionAPI) Refresh() { a.st.refresh() }

func (a sessionAPI) Quit(ctx context.Context) error { return a.st.quit(ctx) }

func (a sessionAPI) Preview(ctx context.Context, item Item, params Params, pc PreviewContext) (*Previewer, error) {
	k, ok := a.st.e.b.kinds[item.Kind].(KindPreviewer)
	if !ok {
		return nil, nil
	}
	if !boolOr(a.st.opts.sourceOptions(item.sourceIndex).Preview, true) {
		return nil, nil
	}
	return k.Preview(ctx, a.st.h, PreviewArgs{
		Options:        &a.st.opts,
		ActionParams:   params,
		PreviewContext: pc,
		Item:           item,
	})
}

func (a sessionAPI) ItemAt(version int64, row int) (Item, bool) { return a.st.itemAt(version, row) }

func (a sessionAPI) Item(id uint64) (Item, bool) {
	items := a.st.itemsByID([]uint64{id})
	if len(items) == 0 {
		return Item{}, false
	}
	return items[0], true
}

func (a sessionAPI) Selected() []Item { return a.st.selectedItems() }

func (a sessionAPI) ToggleSelect(id uint64) {
	if a.st.selected[id] {
		delete(a.st.selected, id)
	} else {
		a.st.selected[id] = true
	}
}

func (a sessionAPI) ToggleSelectAll() {
	all := len(a.st.items) > 0
	for _, it := range a.st.items {
		if !a.st.selected[it.id] {
			all = false
			break
		}
	}
	a.st.selected = map[uint64]bool{}
	if !all {
		for _, it := range a.st.items {
			a.st.selected[it.id] = true
		}
	}
}

func (a sessionAPI) ClearSelect() { a.st.selected = map[uint64]bool{} }

func (a sessionAPI) ExpandItem(item Item, opts ExpandOptions) { a.st.expandItem(item, opts) }

func (a sessionAPI) CollapseItem(item Item) { a.st.collapseItem(item) }

func (a sessionAPI) UpdateOptions(p Params) error { return a.st.updateOptions(p) }

// ---------------------------------------------------------------------------
// Trees

func keyOf(it Item) treeKey { return treeKey{it.sourceIndex, it.TreePath} }

// isExpanded: the tree item shows its children (asked for, or IsExpanded).
func (st *state) isExpanded(it Item) bool {
	if !it.IsTree || it.TreePath == "" {
		return false
	}
	if e, ok := st.expanded[keyOf(it)]; ok {
		return e
	}
	return it.IsExpanded
}

// autoExpand gathers the children of the tree items that came and are
// expanded (asked for before a regather, IsExpanded, or depth levels under
// one expanded: negative for all, but the hidden ones).
func (st *state) autoExpand(items []Item, depth int) {
	for _, it := range items {
		if !it.IsTree || it.TreePath == "" {
			continue
		}
		k := keyOf(it)
		e, asked := st.expanded[k]
		switch {
		case asked && !e:
			continue
		case asked || it.IsExpanded:
		case depth != 0 && !strings.HasPrefix(filepath.Base(it.TreePath), "."):
			st.expanded[k] = true
		default:
			continue
		}
		if st.nodes[k] == nil {
			st.gatherChildren(it, deeper(depth), false)
		}
	}
}

// deeper is the depth of the trees one level down.
func deeper(depth int) int {
	switch {
	case depth > 0:
		return depth - 1
	case depth < 0:
		return -1
	}
	return 0
}

// expandItem expands a tree item: its children are gathered (again, if
// they were).
func (st *state) expandItem(it Item, opts ExpandOptions) {
	if !it.IsTree || it.TreePath == "" {
		return
	}
	k := keyOf(it)
	st.expanded[k] = true
	if st.grouped == nil {
		st.grouped = map[treeKey]bool{}
	}
	// Grouped or not, as this expand says, down the tree.
	for g := range st.grouped {
		if g.src == k.src && isUnder(g.path, k.path) {
			delete(st.grouped, g)
		}
	}
	st.grouped[k] = opts.Grouped
	st.dropNodes(func(x treeKey) bool { return x == k })
	st.gatherChildren(it, opts.MaxLevel, opts.InTree)
	st.dirty = true
}

// collapseItem collapses a tree item, or its parent when it is not
// expanded; the cursor goes to it. The trees under it stay expanded for
// the next time.
func (st *state) collapseItem(it Item) {
	if !st.isExpanded(it) {
		parent, ok := st.parentOf(it)
		if !ok {
			return
		}
		it = parent
	}
	k := keyOf(it)
	if it.groupedPath != "" {
		// Shown in the line of the trees above it: they are collapsed.
		k = it.groupedFrom
	}
	st.expanded[k] = false
	st.dropNodes(func(x treeKey) bool { return x == k || x.src == k.src && isUnder(x.path, k.path) })
	st.cursor = it.id
	st.dirty = true
	st.redrawNow()
}

// parentOf is the tree item shown whose child it is.
func (st *state) parentOf(it Item) (Item, bool) {
	if it.TreePath == "" || it.level == 0 {
		return Item{}, false
	}
	dir := parentPath(it.TreePath)
	for _, p := range st.items {
		if p.sourceIndex == it.sourceIndex && p.IsTree && p.expanded &&
			strings.TrimRight(p.TreePath, `/\`) == dir {
			return p, true
		}
	}
	return Item{}, false
}

// parentPath is the directory of path, with its separators ("/" or "\"),
// as TreePath has them (filepath.Dir would make them those of the system).
func parentPath(path string) string {
	path = strings.TrimRight(path, `/\`)
	i := strings.LastIndexAny(path, `/\`)
	switch {
	case i < 0:
		return ""
	case i == 0:
		return path[:1]
	}
	return path[:i]
}

func isUnder(path, dir string) bool {
	dir = strings.TrimRight(dir, `/\`)
	return len(path) > len(dir)+1 && strings.HasPrefix(path, dir) && strings.ContainsRune(`/\`, rune(path[len(dir)]))
}

// dropNodes stops and forgets the gatherings of children of these items.
func (st *state) dropNodes(match func(treeKey) bool) {
	for k, n := range st.nodes {
		if match(k) {
			if n.cancel != nil {
				n.cancel()
			}
			delete(st.nodes, k)
		}
	}
}

// gatherChildren gathers the children of the tree item parent in a
// goroutine: its source with the path TreePath.
func (st *state) gatherChildren(parent Item, depth int, inTree bool) {
	if st.nodes == nil {
		st.nodes = map[treeKey]*treeNode{}
	}
	k := keyOf(parent)
	st.gatherGen++
	ctx, cancel := context.WithCancel(st.s.base)
	n := &treeNode{parent: parent, gen: st.gatherGen, cancel: cancel, depth: depth, inTree: inTree, at: time.Now()}
	st.nodes[k] = n
	i := parent.sourceIndex
	if i >= len(st.opts.Sources) {
		return
	}
	a := st.gatherArgs(i)
	a.SourceOptions.Path = parent.TreePath
	p := parent
	a.Parent = &p
	gen := n.gen
	level := parent.level + 1
	spec := st.opts.Sources[i]
	src := st.e.b.sources[spec.Name]
	key := a.SourceOptions.MatcherKey
	current := func(st *state) *treeNode {
		if n := st.nodes[k]; n != nil && n.gen == gen {
			return n
		}
		return nil
	}
	go func() {
		err := func() (err error) {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("panic: %v", p)
				}
			}()
			return src.Gather(ctx, st.raw, a, func(items []Item) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if len(items) == 0 {
					return nil
				}
				out := make([]Item, len(items))
				first := st.e.newIDs(len(items))
				for j, it := range items {
					it.id = first + uint64(j)
					it.sourceIndex = i
					it.sourceName = spec.Name
					it.level = level
					if it.Kind == "" {
						it.Kind = src.Kind()
					}
					it.matcherKey = it.Word
					if key == "display" && it.Display != "" {
						it.matcherKey = it.Display
					}
					out[j] = it
				}
				st.s.do(func(st *state) {
					if n := current(st); n != nil {
						if n.stale {
							n.items, n.stale = nil, false
						}
						n.items = append(n.items, out...)
						st.dirty = true
						st.scheduleRedraw()
					}
				})
				return nil
			})
		}()
		st.s.do(func(st *state) {
			n := current(st)
			if n == nil {
				return
			}
			n.done = true
			if n.stale {
				n.items, n.stale = nil, false
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				st.report(fmt.Errorf("source %s: %w", spec.Name, err))
			}
			st.autoExpand(n.items, n.depth)
			if st.grouped[k] && len(n.items) == 1 && n.items[0].IsTree && n.items[0].TreePath != "" {
				// Its only item: shown in its line, expanded too.
				ck := keyOf(n.items[0])
				if e, asked := st.expanded[ck]; !asked || e {
					st.expanded[ck] = true
					st.grouped[ck] = true
					if st.nodes[ck] == nil {
						st.gatherChildren(n.items[0], deeper(n.depth), false)
					}
				}
			}
			if n.inTree && len(n.items) > 0 {
				// The first child shown (sorted, filtered).
				st.filter(st.s.base)
				for j, it := range st.items {
					if it.id == parent.id && j+1 < len(st.items) && st.items[j+1].level > it.level {
						st.cursor = st.items[j+1].id
					}
				}
			}
			st.dirty = true
			st.redrawNow()
		})
	}()
}

// flatten appends items (of the source i, filtered) to all, with the
// children of those expanded under them, filtered as they are. level is
// that of the items shown (-1: their own, at the top).
func (st *state) flatten(ctx context.Context, c *Context, so SourceOptions, i int, items, all []Item, total *int, level int) []Item {
	for _, it := range items {
		if level >= 0 {
			it.level = level
		}
		n := st.nodes[keyOf(it)]
		it.expanded = n != nil && st.isExpanded(it)
		var kids []Item
		if it.expanded {
			*total += len(n.items)
			kids = st.filterChildren(ctx, c, so, n.items)
		}
		// isGrouped: a tree, the only item of one, in its line.
		for it.expanded && st.grouped[keyOf(it)] && len(kids) == 1 && kids[0].IsTree {
			kid := kids[0]
			kid.level = it.level
			kid.groupedPath = it.groupedPath + groupName(it)
			kid.groupedFrom = keyOf(it)
			if it.groupedPath != "" {
				kid.groupedFrom = it.groupedFrom
			}
			kid.Word = it.Word + kid.Word
			kn := st.nodes[keyOf(kid)]
			kid.expanded = kn != nil && st.isExpanded(kid)
			kids = nil
			if kid.expanded {
				*total += len(kn.items)
				kids = st.filterChildren(ctx, c, so, kn.items)
			}
			it = kid
		}
		all = append(all, it)
		if it.expanded {
			all = st.flatten(ctx, c, so, i, kids, all, total, it.level+1)
		}
	}
	return all
}

// setSourcePath makes path the path of the source i.
func (st *state) setSourcePath(i int, path string) {
	if i >= len(st.opts.Sources) {
		return
	}
	sources := slices.Clone(st.opts.Sources)
	spec := sources[i]
	so := SourceOptions{}
	if spec.Options != nil {
		so = *spec.Options
	}
	so.Path = path
	spec.Options = &so
	sources[i] = spec
	st.opts.Sources = sources
	if p, err := toPatch(Options{Sources: sources}); err == nil {
		st.patch = maps.Clone(st.patch)
		st.patch["sources"] = p["sources"]
	}
	st.ctx.Path = path
}

// rootItems are the root items of the sources: their paths.
func (st *state) rootItems() []Item {
	if st.rootIDs == nil {
		st.rootIDs = map[treeKey]uint64{}
	}
	roots := make([]Item, len(st.opts.Sources))
	for i, spec := range st.opts.Sources {
		path := sourcePath(&st.ctx, st.opts.sourceOptions(i))
		k := treeKey{i, path}
		id := st.rootIDs[k]
		if id == 0 {
			id = st.e.newIDs(1)
			st.rootIDs[k] = id
		}
		kind := "base"
		if src := st.e.b.sources[spec.Name]; src != nil {
			kind = src.Kind()
		}
		roots[i] = Item{
			Word: path, Kind: kind, TreePath: path,
			id: id, sourceIndex: i, sourceName: spec.Name, level: -1, expanded: true,
			matcherKey: path,
		}
	}
	st.roots = roots
	return roots
}

// applyColumns makes what the items of a source show from its columns.
func (st *state) applyColumns(ctx context.Context, c *Context, so SourceOptions, items []Item) {
	if len(so.Columns) == 0 || len(items) == 0 {
		return
	}
	type col struct {
		c      Column
		args   ColumnArgs
		length int
	}
	var cols []col
	for _, name := range so.Columns {
		cl := st.e.b.columns[name]
		if cl == nil {
			continue
		}
		a := ColumnArgs{Context: c, Options: &st.opts, ColumnParams: st.opts.columnParams(name), Items: items}
		n, err := cl.Length(ctx, st.h, a)
		if err != nil {
			st.report(fmt.Errorf("column %s: %w", name, err))
			continue
		}
		cols = append(cols, col{cl, a, n})
	}
	for j := range items {
		it := &items[j]
		var display strings.Builder
		var hls []Highlight
		for _, cl := range cols {
			startCol := display.Len() + 1
			t, err := cl.c.Text(ctx, st.h, cl.args, *it, startCol, startCol+cl.length)
			if err != nil {
				st.report(fmt.Errorf("column %s: %w", cl.c.Name(), err))
				continue
			}
			display.WriteString(t.Text)
			hls = append(hls, t.Highlights...)
		}
		it.Display = display.String()
		it.Highlights = hls
	}
}

// withinLimit: path is the limitPath of the source i, or under it (or it
// has none).
func (st *state) withinLimit(i int, path string) bool {
	so := st.opts.sourceOptions(i)
	if so.LimitPath == "" {
		return true
	}
	limit := filepath.Clean(abs(&st.ctx, so.LimitPath))
	path = filepath.Clean(abs(&st.ctx, path))
	return path == limit || isUnder(path, limit)
}

// updateOptions merges p into the options of the session (the UI action
// updateOptions): what is gathered and drawn next follows them.
func (st *state) updateOptions(p Params) error {
	merged := mergePatch(st.patch, patch(p))
	o, err := st.e.check(merged)
	if err != nil {
		return err
	}
	st.patch, st.opts = merged, o
	for i, path := range st.paths {
		st.setSourcePath(i, path)
	}
	st.ui = st.e.b.uis[o.UI]
	st.dirty = true
	return nil
}

// checkItems gathers again the sources whose items changed since they
// gathered them (or those of a tree expanded): the trees stay expanded.
func (st *state) checkItems(ctx context.Context) bool {
	updated := false
	for i, g := range st.src {
		if g == nil || !g.done || i >= len(st.opts.Sources) {
			continue
		}
		c, ok := st.e.b.sources[st.opts.Sources[i].Name].(SourceUpdateChecker)
		if !ok {
			continue
		}
		a := st.gatherArgs(i)
		if u, err := c.Updated(ctx, st.h, a, g.at); err == nil && u {
			updated = true
			break
		}
		for k, n := range st.nodes {
			if k.src != i || !n.done {
				continue
			}
			a := st.gatherArgs(i)
			a.SourceOptions.Path = k.path
			if u, err := c.Updated(ctx, st.h, a, n.at); err == nil && u {
				updated = true
				break
			}
		}
		if updated {
			break
		}
	}
	if updated {
		st.gather(allIndexes(len(st.opts.Sources)), true)
	}
	return updated
}

func (a sessionAPI) CheckItems(ctx context.Context) bool { return a.st.checkItems(ctx) }

func allIndexes(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// regatherNode gathers the items of a tree again, those it has shown until
// new ones come.
func (st *state) regatherNode(old *treeNode) {
	items := old.items
	if old.cancel != nil {
		old.cancel()
	}
	st.gatherChildren(old.parent, old.depth, false)
	if n := st.nodes[keyOf(old.parent)]; n != nil {
		n.items, n.stale = items, true
	}
}

func (st *state) filterChildren(ctx context.Context, c *Context, so SourceOptions, items []Item) []Item {
	items = st.runFilters(ctx, c, so, so.Matchers, items)
	items = st.runFilters(ctx, c, so, so.Sorters, items)
	return st.runFilters(ctx, c, so, so.Converters, items)
}

// groupName is the name of a tree in the line of a tree under it: "dir/".
func groupName(it Item) string {
	return filepath.Base(strings.TrimRight(it.TreePath, `/\`)) + "/"
}
