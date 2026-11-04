// Package ff is the fuzzy finder UI of goda-fi.vim, as ddu-ui-ff: the items in
// a buffer of a split window, filtered by what is typed in the command line
// (input(), not a popup: the window and Vim stay yours).
//
// The Go side makes what the buffer shows; autoload/goda_fi/ui/ff.vim applies
// it in one call, and drops what is older than what it shows. Moving the
// cursor and the filter input are handled in Vim; the actions on items come
// here, with the version of what Vim showed.
package ff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the UI params (uiParams.ff), as ddu-ui-ff's. Sizes may be
// numbers or Vim expressions ("&lines / 2").
type Params struct {
	// Split: "horizontal", "vertical", "tab" or "no" (the current window).
	Split string `json:"split"`
	// SplitDirection: "botright", "topleft", "belowright" or "aboveleft".
	SplitDirection string `json:"splitDirection"`
	WinHeight      any    `json:"winHeight"`
	WinWidth       any    `json:"winWidth"`
	// MaxDisplayItems bounds the lines of the buffer.
	MaxDisplayItems int `json:"maxDisplayItems"`
	// MaxHighlightItems bounds the items whose highlights are drawn.
	MaxHighlightItems int `json:"maxHighlightItems"`
	// DisplaySourceName: "no", "short" or "long" (see sourcePrefixes).
	DisplaySourceName string `json:"displaySourceName"`
	// DisplayTree: the items of trees are indented by their level, after
	// "+ " (collapsed), "- " (expanded) or "  " (not a tree).
	DisplayTree bool `json:"displayTree"`
	// AutoResize: the window is no higher than its items (horizontal).
	AutoResize bool `json:"autoResize"`
	// Reversed: the first item at the bottom.
	Reversed bool `json:"reversed"`
	// IgnoreEmpty: no window when the sources gathered nothing.
	IgnoreEmpty bool `json:"ignoreEmpty"`
	// CursorPos: the line of the cursor when the window opens (0: the
	// first item).
	CursorPos int `json:"cursorPos"`
	// Highlights: groups of "selected" (Statement) and "filterText".
	Highlights map[string]string `json:"highlights"`
	// OverwriteStatusline: the window shows the name, the counts and the
	// input in its status line.
	OverwriteStatusline bool `json:"overwriteStatusline"`
	// Preview window: "horizontal", "vertical" or "no" (none).
	PreviewSplit  string `json:"previewSplit"`
	PreviewHeight any    `json:"previewHeight"`
	PreviewWidth  any    `json:"previewWidth"`
	// PreviewMaxSize: larger files are not previewed.
	PreviewMaxSize int `json:"previewMaxSize"`
	// PreviewFloating: the preview in a floating window (Neovim) or a
	// popup (Vim), beside the window of the items (or at PreviewRow,
	// PreviewCol).
	PreviewFloating         bool   `json:"previewFloating"`
	PreviewFloatingBorder   any    `json:"previewFloatingBorder"`
	PreviewFloatingTitle    any    `json:"previewFloatingTitle"`
	PreviewFloatingTitlePos string `json:"previewFloatingTitlePos"`
	PreviewFloatingZindex   int    `json:"previewFloatingZindex"`
	PreviewRow              any    `json:"previewRow"`
	PreviewCol              any    `json:"previewCol"`
	// OnPreview is a Vim function called after a preview with
	// {previewWinId, lineNr} ("normal! zt" there, say).
	OnPreview string `json:"onPreview"`

	// Split "floating" (Neovim; Vim splits horizontally): the window is
	// at WinRow, WinCol (expressions may use uiParams).
	WinRow           any    `json:"winRow"`
	WinCol           any    `json:"winCol"`
	FloatingBorder   any    `json:"floatingBorder"`
	FloatingTitle    any    `json:"floatingTitle"`
	FloatingTitlePos string `json:"floatingTitlePos"`
	// FloatingBlend is the 'winblend' of the floating windows.
	FloatingBlend int `json:"floatingBlend"`

	// AutoAction is the UI action run when the cursor moves to another
	// item ({name: "preview"}), once StartAutoAction or the action
	// toggleAutoAction enabled it.
	AutoAction      AutoAction `json:"autoAction"`
	StartAutoAction bool       `json:"startAutoAction"`

	// The filer's (ui/filer), as ddu-ui-filer's:
	// DisplayRoot: a line of the path of each source before its items
	// ("file:~/work"), its root item.
	DisplayRoot bool `json:"displayRoot"`
	// Sort: "filename", "natural", "extension", "size", "time" or "none";
	// upper case reverses it. The items of a tree are sorted among them.
	Sort string `json:"sort"`
	// SortTreesFirst: the trees before the other items.
	SortTreesFirst bool `json:"sortTreesFirst"`
	// FileFilter: a regexp (Go's) the words of the items that are not
	// trees must match.
	FileFilter string `json:"fileFilter"`
}

// AutoAction is the param autoAction.
type AutoAction struct {
	Name   string    `json:"name,omitempty"`
	Params fi.Params `json:"params,omitempty"`
	// Delay in milliseconds (100).
	Delay *int `json:"delay,omitempty"`
	// Sync is accepted for ddu-ui-ff's configurations; actions are
	// always run in order.
	Sync *bool `json:"sync,omitempty"`
}

// DefaultParams are those of ddu-ui-ff.
func DefaultParams() Params {
	return Params{
		Split:                   "horizontal",
		SplitDirection:          "botright",
		WinHeight:               20,
		WinWidth:                "&columns / 2",
		MaxDisplayItems:         1000,
		MaxHighlightItems:       100,
		DisplaySourceName:       "no",
		OverwriteStatusline:     true,
		PreviewSplit:            "horizontal",
		PreviewHeight:           10,
		PreviewWidth:            80,
		PreviewMaxSize:          1000000,
		PreviewFloatingBorder:   "none",
		PreviewFloatingTitle:    "",
		PreviewFloatingTitlePos: "left",
		PreviewFloatingZindex:   100,
		PreviewRow:              0,
		PreviewCol:              0,
		WinRow:                  "&lines / 2 - 10",
		WinCol:                  "(&columns - eval(uiParams.winWidth)) / 2",
		FloatingBorder:          "none",
		FloatingTitle:           "",
		FloatingTitlePos:        "left",
	}
}

// vimParams are the params as Vim sees them (the JSON names), and extra.
func vimParams(p Params, extra map[string]any) map[string]any {
	b, _ := json.Marshal(p)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	if m["autoAction"] != nil {
		aa := m["autoAction"].(map[string]any)
		if _, ok := aa["delay"]; !ok {
			aa["delay"] = 100
		}
		if _, ok := aa["params"]; !ok {
			aa["params"] = map[string]any{}
		}
	}
	if m["highlights"] == nil {
		// Not null: Vim looks into it (get()).
		m["highlights"] = map[string]any{}
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// UI is the ff UI, or the filer (NewFiler). One serves every session.
type UI struct {
	name  string
	filer bool
	mu    sync.Mutex
	state map[string]*session
}

type session struct {
	// input last drawn: the cursor goes to the first item when it changes.
	input string
	// shown: the window was opened.
	shown bool
}

// New returns the UI, to add to the builder.
func New() *UI { return &UI{name: "ff", state: map[string]*session{}} }

// NewFiler returns the filer UI "filer" (ui/filer): the same UI, with the
// buffer goda-fi-filer-{name} of filetype "goda-fi-filer", the root items
// of the sources, and their items sorted.
func NewFiler() *UI { return &UI{name: "filer", filer: true, state: map[string]*session{}} }

func (u *UI) Name() string { return u.name }

// Acks: goda_fi#ui#ff#_redraw() acknowledges every redraw.
func (*UI) Acks() bool { return true }

func (u *UI) session(name string) *session {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := u.state[name]
	if s == nil {
		s = &session{}
		u.state[name] = s
	}
	return s
}

func (u *UI) params(v *fi.View) (Params, error) {
	p := DefaultParams()
	if u.filer {
		p = DefaultFilerParams()
	}
	if err := fi.Decode(v.UIParams, &p); err != nil {
		return p, err
	}
	return p, nil
}

// DefaultFilerParams are those of ddu-ui-filer.
func DefaultFilerParams() Params {
	p := DefaultParams()
	p.SplitDirection = "aboveleft"
	p.DisplayRoot = true
	p.Sort = "none"
	return p
}

// hl is a highlight: [row, col (1-based, bytes), length (0: to the end),
// group, priority].
type hl = [5]any

const (
	itemPriority     = 100
	selectedPriority = 10000
)

// render is what Vim applies (goda_fi#ui#ff#_redraw).
type render struct {
	Version int64  `json:"version"`
	Name    string `json:"name"`
	// UI: "ff" or "filer" (the buffer, the filetype).
	UI string `json:"ui"`
	// Levels are those of the items in the tree, by line.
	Levels []int `json:"levels"`
	// Path is that of the first source (the filer's
	// b:goda_fi_ui_filer_path and t:goda_fi_ui_filer_path).
	Path       string         `json:"path"`
	Open       bool           `json:"open"`
	Lines      []string       `json:"lines"`
	IDs        []uint64       `json:"ids"`
	Highlights []hl           `json:"highlights"`
	Info       []info         `json:"info"`
	Cursor     int            `json:"cursor"`
	Reset      bool           `json:"reset"`
	Input      string         `json:"input"`
	Status     [2]string      `json:"status"`
	PrevWinID  int            `json:"prevWinid"`
	Params     map[string]any `json:"params"`
	Done       bool           `json:"done"`
}

type info struct {
	Row   int     `json:"row"`
	Lines [][]any `json:"lines"`
}

// sourcePrefixes are what goes before the items of every source: "[name] "
// with displaySourceName "long"; with "short", a short name that tells it
// from the others (shortNames).
func sourcePrefixes(mode string, sources []string) map[string]string {
	out := map[string]string{}
	switch mode {
	case "long":
		for _, s := range sources {
			out[s] = "[" + s + "] "
		}
	case "short":
		for name, short := range shortNames(sources) {
			out[name] = "[" + short + "] "
		}
	}
	return out
}

// shortNames are the short names of the sources. A name shares its first
// characters with the other names, the most with one of them: the short
// name is the shorter of
//
//   - its first character and what follows the shared part (file, fileold
//     and filerec are f, fold and frec), and
//   - the shared part and one character more (rg and register are rg and
//     re).
//
// A name that shares nothing is its first character.
func shortNames(sources []string) map[string]string {
	out := map[string]string{}
	for _, s := range sources {
		if s == "" {
			continue
		}
		shared := 0
		for _, o := range sources {
			if o != s {
				shared = max(shared, commonPrefix(s, o))
			}
		}
		rs := []rune(s)
		first := string(rs[0])
		cut := first + s[max(shared, len(first)):]
		head := string(rs[:min(len([]rune(s[:shared]))+1, len(rs))])
		if len([]rune(head)) < len([]rune(cut)) {
			out[s] = head
		} else {
			out[s] = cut
		}
	}
	return out
}

// commonPrefix is the length in bytes of what a and b start with (whole
// characters).
func commonPrefix(a, b string) int {
	n := 0
	for i, r := range a {
		c := string(r)
		if !strings.HasPrefix(b[min(i, len(b)):], c) {
			break
		}
		n = i + len(c)
	}
	return n
}

// Redraw sends the lines of the view to Vim.
func (u *UI) Redraw(ctx context.Context, h sdk.Host, v *fi.View) error {
	p, err := u.params(v)
	if err != nil {
		return err
	}
	s := u.session(v.Context.Name)
	if v.Open {
		s.shown = false
	}
	if p.IgnoreEmpty && !s.shown && len(v.Items) == 0 {
		// Not opened for nothing (items may still come); no
		// acknowledgement comes for it.
		return fi.ErrNotDrawn
	}
	items := v.Items
	if u.filer {
		if items, err = filerItems(v, p); err != nil {
			return err
		}
	}
	n := min(len(items), max(p.MaxDisplayItems, 0))
	r := render{
		Version:    v.Version,
		Name:       v.Context.Name,
		UI:         u.name,
		Levels:     make([]int, n),
		Path:       rootPath(v),
		Open:       v.Open || !s.shown,
		Lines:      make([]string, n),
		IDs:        make([]uint64, n),
		Highlights: []hl{},
		Info:       []info{},
		Input:      v.Input,
		PrevWinID:  v.Context.WinID,
		Done:       v.Context.Done,
		Params: vimParams(p, map[string]any{
			"filterPrompt":         derefString(v.UIOptions.FilterPrompt),
			"filterUpdateMax":      derefInt(v.UIOptions.FilterUpdateMax),
			"filterInputFunc":      v.UIOptions.FilterInputFunc,
			"filterInputOptsFunc":  v.UIOptions.FilterInputOptsFunc,
			"filterUpdateCallback": v.UIOptions.FilterUpdateCallback,
			"itemCount":            len(v.Items),
		}),
	}
	selected := p.Highlights["selected"]
	if selected == "" {
		selected = "Statement"
	}
	row := func(i int) int {
		if p.Reversed {
			return n - i
		}
		return i + 1
	}
	prefixes := sourcePrefixes(p.DisplaySourceName, v.Sources)
	shown := 0
	for i := range n {
		it := &items[i]
		r.Levels[row(i)-1] = it.TreeLevel()
		if it.TreeLevel() >= 0 {
			shown++
		}
		prefix := prefixes[it.SourceName()]
		if p.DisplayTree {
			prefix += treePrefix(it)
		}
		text := prefix + oneLine(it.Text())
		r.Lines[row(i)-1] = text
		r.IDs[row(i)-1] = it.ID()
		if i < p.MaxHighlightItems {
			for _, x := range it.Highlights {
				r.Highlights = append(r.Highlights, hl{row(i), len(prefix) + x.Col, x.Width, x.HlGroup, itemPriority})
			}
		}
		if v.Selected[it.ID()] {
			r.Highlights = append(r.Highlights, hl{row(i), 1, 0, selected, selectedPriority})
		}
		if len(it.Info) > 0 {
			in := info{Row: row(i)}
			for _, x := range it.Info {
				in.Lines = append(in.Lines, []any{x.Text, x.HlGroup})
			}
			r.Info = append(r.Info, in)
		}
		if v.Cursor != 0 && it.ID() == v.Cursor {
			r.Cursor = row(i)
		}
	}
	if v.InitCursor && r.Cursor == 0 && n > 0 {
		r.Cursor = row(0)
		if p.CursorPos > 0 {
			r.Cursor = min(p.CursorPos, n)
		}
	}
	if v.Input != s.input {
		r.Reset = true
		s.input = v.Input
	}
	r.Status = status(v, shown)
	s.shown = true
	return h.Notify(ctx, "goda_fi#ui#ff#_redraw", r.Name, r)
}

// status is the status line around the line numbers, as ddu-ui-ff's:
// [goda-fi-name] shown/gathered, and the input and [async].
func status(v *fi.View, n int) [2]string {
	head := "[goda-fi-" + v.Context.Name + "]"
	if n != v.Context.MaxItems {
		head += fmt.Sprintf(" %d/%d", n, v.Context.MaxItems)
	}
	var foot string
	if v.Input != "" {
		foot += " " + v.Input
	}
	if !v.Context.Done {
		foot += " [async]"
	}
	return [2]string{head, foot}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func oneLine(s string) string {
	if strings.ContainsAny(s, "\n\r") {
		s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	}
	return s
}

// Quit closes the window.
func (u *UI) Quit(ctx context.Context, h sdk.Host, v *fi.View) error {
	u.session(v.Context.Name).shown = false
	_, err := h.Call(ctx, "goda_fi#ui#ff#_quit", v.Context.Name, v.Version)
	return err
}

// Actions are the UI actions done in Go; those of the cursor and the filter
// are in Vim (autoload/goda_fi/ui/ff.vim).
func (u *UI) Actions() map[string]fi.UIAction {
	return map[string]fi.UIAction{
		"itemAction":          itemAction,
		"toggleSelectItem":    toggleSelectItem,
		"toggleAllItems":      toggleAllItems,
		"clearSelectAllItems": clearSelectAllItems,
		"preview":             preview,
		"quit":                quit,
		"redraw":              redraw,
		"expandItem":          expandItem,
		"collapseItem":        collapseItem,
		"togglePreview":       togglePreview,
		"updateOptions":       updateOptions,
		"checkItems":          checkItems,
		"chooseAction":        chooseAction,
		"inputActionNames":    inputActionNames,
	}
}

// togglePreview is preview with params.toggle.
func togglePreview(ctx context.Context, h sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	p := fi.Params{}
	for k, v := range a.ActionParams {
		p[k] = v
	}
	p["toggle"] = true
	a.ActionParams = p
	return preview(ctx, h, a)
}

// updateOptions merges params into the options of the session (as
// ddu.vim's): what is gathered and drawn next follows them (the action
// redraw).
func updateOptions(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	return fi.None, a.Session.UpdateOptions(a.ActionParams)
}

// checkItems gathers again when the files changed (the directories of the
// source file).
func checkItems(ctx context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	a.Session.CheckItems(ctx)
	return fi.None, nil
}

// chooseAction lists the item actions of the items in a session pushed on
// this one (goda_fi#pop() goes back); the one chosen runs on them.
func chooseAction(ctx context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	items := targets(a)
	if len(items) == 0 {
		return fi.None, nil
	}
	return fi.None, a.Session.ChooseAction(ctx, items)
}

// inputActionNames gives Vim the item actions of the items, for
// inputAction (autoload/goda_fi/ui/ff.vim): goda_fi#ui#ff#_input_action().
func inputActionNames(ctx context.Context, h sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	items := targets(a)
	if len(items) == 0 {
		return fi.None, nil
	}
	_, err := h.Call(ctx, "goda_fi#ui#ff#_input_action", a.View.Context.Name, a.Session.ItemActionNames(items))
	return fi.None, err
}

// treePrefix is what goes before an item with displayTree, as ddu-ui-ff's.
func treePrefix(it *fi.Item) string {
	if it.TreeLevel() < 0 {
		return ""
	}
	mark := "  "
	switch {
	case it.IsTree && it.Expanded():
		mark = "- "
	case it.IsTree:
		mark = "+ "
	}
	return strings.Repeat(" ", it.TreeLevel()) + mark
}

// expandItem expands the tree under the cursor (params: mode "toggle"
// collapses it when it is expanded; maxLevel, the levels under it expanded
// too, negative for all; isInTree, the cursor goes to its first child).
func expandItem(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	var p struct {
		Mode     string `json:"mode"`
		MaxLevel int    `json:"maxLevel"`
		IsInTree bool   `json:"isInTree"`
		// IsGrouped: a tree that is the only item of its parent goes in
		// the line of its parent ("a/b/").
		IsGrouped bool `json:"isGrouped"`
	}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	if a.Item == nil || !a.Item.IsTree {
		return fi.None, nil
	}
	if a.Item.Expanded() {
		if p.Mode == "toggle" {
			a.Session.CollapseItem(*a.Item)
		}
		return fi.None, nil
	}
	a.Session.ExpandItem(*a.Item, fi.ExpandOptions{MaxLevel: p.MaxLevel, InTree: p.IsInTree, Grouped: p.IsGrouped})
	return fi.Redraw, nil
}

// collapseItem collapses the tree under the cursor, or the one it is in.
func collapseItem(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	if a.Item != nil {
		a.Session.CollapseItem(*a.Item)
	}
	return fi.None, nil
}

// targets are the items of an action: those selected, or that under the
// cursor.
func targets(a fi.UIActionArgs) []fi.Item {
	if sel := a.Session.Selected(); len(sel) > 0 {
		return sel
	}
	if a.Item != nil {
		return []fi.Item{*a.Item}
	}
	return nil
}

// itemAction runs the item action params.name ("default" when unset) with
// params.params.
func itemAction(ctx context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	var p struct {
		Name   string    `json:"name"`
		Params fi.Params `json:"params"`
	}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	items := targets(a)
	if len(items) == 0 {
		return fi.None, nil
	}
	if p.Name == "" {
		p.Name = "default"
	}
	a.Session.ClearSelect()
	return fi.None, a.Session.ItemAction(ctx, p.Name, items, p.Params)
}

func toggleSelectItem(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	if a.Item != nil {
		a.Session.ToggleSelect(a.Item.ID())
	}
	return fi.Redraw, nil
}

func toggleAllItems(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	a.Session.ToggleSelectAll()
	return fi.Redraw, nil
}

func clearSelectAllItems(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	a.Session.ClearSelect()
	return fi.Redraw, nil
}

func quit(ctx context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	return fi.None, a.Session.Quit(ctx)
}

// redraw: gathers the items again (as ddu-ui-ff's redraw with method
// "refreshItems", its default), or draws them again ({method: "uiRedraw"}).
func redraw(_ context.Context, _ sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	var p struct {
		Method string `json:"method"`
	}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	if p.Method == "uiRedraw" || p.Method == "uiRefresh" {
		return fi.Redraw, nil
	}
	return fi.RefreshItems, nil
}

// preview shows the preview of the item under the cursor in the preview
// window, or closes it ({toggle: true} when it shows that item).
func preview(ctx context.Context, h sdk.Host, a fi.UIActionArgs) (fi.ActionFlags, error) {
	// The other params are the kind's (previewCmds, ...).
	var p struct {
		Toggle bool `json:"toggle"`
	}
	if err := sdk.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	name := a.View.Context.Name
	if a.Item == nil {
		_, err := h.Call(ctx, "goda_fi#ui#ff#_close_preview", name)
		return fi.None, err
	}
	// The defaults of ff: the preview is the same in the filer.
	up, err := (&UI{}).params(a.View)
	if err != nil {
		return fi.None, err
	}
	pv, err := a.Session.Preview(ctx, *a.Item, a.ActionParams, fi.PreviewContext{Split: up.PreviewSplit})
	if err != nil {
		return fi.None, err
	}
	if pv == nil {
		_, err := h.Call(ctx, "goda_fi#ui#ff#_close_preview", name)
		return fi.None, err
	}
	_, err = h.Call(ctx, "goda_fi#ui#ff#_preview", name, a.Item.ID(), pv, vimParams(up, map[string]any{
		"toggle":  p.Toggle,
		"maxSize": up.PreviewMaxSize,
	}))
	return fi.None, err
}

func rootPath(v *fi.View) string {
	if len(v.Roots) == 0 {
		return ""
	}
	return v.Roots[0].Word
}
