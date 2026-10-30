package fi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/Shougo/goda.vim/sdk"
)

// redrawInterval is the least time between two redraws while items arrive.
var redrawInterval = 50 * time.Millisecond

// ackTimeout is how long a redraw waits for the acknowledgement of the one
// before (UIAcker) at most.
var ackTimeout = 2 * time.Second

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
	s.st = &state{s: s, e: e, h: h, name: name, selected: map[uint64]bool{}, views: map[int64][]Item{}}
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
		for {
			s.mu.Lock()
			if len(s.queue) == 0 {
				s.mu.Unlock()
				break
			}
			f := s.queue[0]
			s.queue[0] = nil
			s.queue = s.queue[1:]
			s.mu.Unlock()
			s.safe(f)
		}
	}
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
	gen    int
	cancel context.CancelFunc
	done   bool
	items  []Item
}

// state is a session: the items gathered, the input, the selection, and
// what the UI shows.
type state struct {
	s    *session
	e    *engine
	h    sdk.Host
	name string

	started bool
	patch   patch
	opts    Options
	ctx     Context
	ui      UI

	src   []*gathering
	input string
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
	// inflight is the version drawn and not acknowledged yet (UIAcker),
	// since inflightAt; deferred: a redraw waits for it.
	inflight   int64
	inflightAt time.Time
	deferred   bool
	search     string
	errShown   map[string]bool
	gatherGen  int
}

// start begins the session with options o (the merged patch p), or resumes
// it. With the option sync, the channel returned is closed once every item
// is there.
func (st *state) start(o Options, p patch, c Context, user patch) (<-chan struct{}, error) {
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
	st.patch, st.opts = p, o
	st.ui = st.e.b.uis[o.UI]
	c.Name = o.Name
	c.Path = c.Cwd
	if resume {
		c.Input, c.Done, c.MaxItems, c.Path = st.ctx.Input, st.ctx.Done, st.ctx.MaxItems, st.ctx.Path
	}
	st.ctx = c
	st.visible = true
	st.open = true
	st.sync = boolOr(o.Sync, false)
	st.search = o.SearchPath
	st.errShown = map[string]bool{}
	switch {
	case o.Input != nil:
		st.input = *o.Input
	case !resume:
		st.input = ""
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
		}
		st.started = true
		st.gather(nil)
	} else {
		st.redrawNow()
	}
	st.checkDone()
	if wait == nil {
		return nil, nil
	}
	return wait, nil
}

// gather gathers the sources of these indexes again (all, for nil).
func (st *state) gather(indexes []int) {
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
		if g := st.src[i]; g != nil && g.cancel != nil {
			g.cancel()
		}
		st.gatherGen++
		ctx, cancel := context.WithCancel(st.s.base)
		g := &gathering{gen: st.gatherGen, cancel: cancel}
		st.src[i] = g
		go st.gatherSource(ctx, i, g.gen, st.gatherArgs(i))
	}
	if len(indexes) == 0 {
		st.checkDone()
		return
	}
	// The items of the sources gathered again are gone: show it.
	st.scheduleRedraw()
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
			if err := in.OnInit(ctx, st.h, a); err != nil {
				return err
			}
		}
		return src.Gather(ctx, st.h, a, func(items []Item) error {
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
					g.items = append(g.items, out...)
					st.dirty = true
					st.scheduleRedraw()
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
		if boolOr(so.Volatile, false) {
			// The source filtered them.
			items = st.runFilters(ctx, &c, so, so.Converters, items)
		} else {
			items = st.runFilters(ctx, &c, so, so.Matchers, items)
			items = st.runFilters(ctx, &c, so, so.Sorters, items)
			items = st.runFilters(ctx, &c, so, so.Converters, items)
		}
		if max := intOr(so.MaxItems, 10000); len(items) > max {
			items = items[:max]
		}
		all = append(all, items...)
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

func (st *state) setInput(input string) {
	if input == st.input {
		return
	}
	st.input = input
	st.ctx.Input = input
	st.dirty = true
	var volatile []int
	for i := range st.opts.Sources {
		if boolOr(st.opts.sourceOptions(i).Volatile, false) {
			volatile = append(volatile, i)
		}
	}
	if len(volatile) > 0 {
		st.gather(volatile)
	}
}

func (st *state) refresh() {
	if !st.started {
		return
	}
	st.gather(nil)
}

// hide stops drawing (the session was pushed, or popped): another session of
// the name has the UI.
func (st *state) hide() {
	st.stopTimer()
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
	set := &actionSet{kind: kind, source: src, actions: map[string]Action{}}
	maps.Copy(set.actions, k.Actions())
	if s, ok := st.e.b.sources[items[0].sourceName].(SourceActioner); ok {
		maps.Copy(set.actions, s.Actions())
	}
	ko := st.opts.kindOptions(kind)
	set.aliases = ko.Actions
	set.def = ko.DefaultAction
	if src < len(st.opts.Sources) {
		if d := st.opts.sourceOptions(src).DefaultAction; d != "" {
			set.def = d
		}
	}
	return set, nil
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
	if quit && res.Flags&Persist != 0 {
		st.visible = true
		st.open = true
	}
	switch {
	case res.Flags&RefreshItems != 0:
		st.gather(nil)
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
	act := st.ui.Actions()[name]
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
		st.gather(nil)
	case flags&Redraw != 0:
		st.dirty = true
		st.redrawNow()
	}
	return nil
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
