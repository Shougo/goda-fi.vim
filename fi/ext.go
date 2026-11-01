package fi

import (
	"context"

	"github.com/Shougo/goda.vim/sdk"
)

// Source gathers items.
//
// Gather hands the items over with emit as they come, as many times as it
// likes, and returns when it has no more (or ctx is done: the session
// started over, or quit). emit fails once ctx is done. The engine draws the
// UI while items arrive.
type Source interface {
	Name() string
	// Kind is the kind of the items that do not say theirs.
	Kind() string
	Gather(ctx context.Context, h sdk.Host, a GatherArgs, emit func([]Item) error) error
}

// GatherArgs are what a source gathers with.
type GatherArgs struct {
	// Context is where the session was started: by the time a source
	// gathers, the current window of Vim may be the UI's, so look at
	// Context.WinID and Context.BufNr (getcurpos(winid), ...), not at "%".
	Context       *Context
	Options       *Options
	SourceOptions SourceOptions
	// SourceParams are the params of the source, to decode (Decode).
	SourceParams Params
	// Input is the input, for a volatile source (SourceOptions.Volatile).
	Input string
}

// SourceIniter is a Source that prepares something (once per session) before
// gathering.
type SourceIniter interface {
	OnInit(ctx context.Context, h sdk.Host, a GatherArgs) error
}

// SourceActioner is a Source with actions of its own, over those of the kind.
type SourceActioner interface {
	Actions() map[string]Action
}

// Filter is a matcher, sorter or converter.
type Filter interface {
	Name() string
	Filter(ctx context.Context, h sdk.Host, a FilterArgs) ([]Item, error)
}

// FilterArgs are what a filter works on. Items must not be modified in place:
// return a new slice (items may be copied and changed).
type FilterArgs struct {
	Context       *Context
	Options       *Options
	SourceOptions SourceOptions
	FilterOptions FilterOptions
	FilterParams  Params
	// Input is the text typed.
	Input string
	Items []Item
}

// IgnoreCase reports whether a matcher ignores case: the source options
// ignoreCase, or smartCase with an input without upper case letters.
func (a FilterArgs) IgnoreCase() bool {
	if boolOr(a.SourceOptions.IgnoreCase, false) {
		return true
	}
	if !boolOr(a.SourceOptions.SmartCase, false) {
		return false
	}
	for _, r := range a.Input {
		if 'A' <= r && r <= 'Z' {
			return false
		}
	}
	return true
}

// Kind is what can be done with items: its actions.
type Kind interface {
	Name() string
	Actions() map[string]Action
}

// KindPreviewer is a Kind that can preview items.
type KindPreviewer interface {
	Preview(ctx context.Context, h sdk.Host, a PreviewArgs) (*Previewer, error)
}

// PreviewArgs are what a preview is made with.
type PreviewArgs struct {
	Options        *Options
	ActionParams   Params
	PreviewContext PreviewContext
	Item           Item
}

// Action is an action on items.
type Action struct {
	Description string
	Run         func(ctx context.Context, h sdk.Host, a ActionArgs) (ActionResult, error)
}

// ActionArgs are what an action works with.
type ActionArgs struct {
	Context       *Context
	Options       *Options
	SourceOptions SourceOptions
	SourceParams  Params
	KindOptions   KindOptions
	KindParams    Params
	ActionParams  Params
	Items         []Item
}

// UI shows the items of sessions, and has actions of its own (move the
// cursor, filter, run an item action, ...). One UI value serves every
// session: it keeps what it needs by session name.
type UI interface {
	Name() string
	// Redraw shows the state of a session.
	Redraw(ctx context.Context, h sdk.Host, v *View) error
	// Quit closes the UI of a session.
	Quit(ctx context.Context, h sdk.Host, v *View) error
	// Actions are the UI actions, by name.
	Actions() map[string]UIAction
}

// UIAcker is a UI whose Vim side tells when it has drawn a View:
// goda#notify('fi', 'event', name, 'drawn', version). Then a redraw is not
// sent before the one before is drawn (or 2 seconds passed): when Vim is
// slower than the items come, the redraws in between are skipped instead of
// queued.
type UIAcker interface {
	Acks() bool
}

// UIAction is an action of a UI.
type UIAction func(ctx context.Context, h sdk.Host, a UIActionArgs) (ActionFlags, error)

// UIActionArgs are what a UI action works with.
type UIActionArgs struct {
	View         *View
	ActionParams Params
	// Cursor is the row of the cursor in the UI (1-based) when the action
	// was asked, in the version Version of the view: the UI may have been
	// drawn again since.
	Cursor  int
	Version int64
	// Item is the item under the cursor, as the UI told it (nil: none).
	Item *Item
	// Session does what the action asks the engine for.
	Session Session
}

// Session is what UI actions can ask of the engine.
type Session interface {
	// ItemAction runs the item action name ("default" for the default
	// one) on items.
	ItemAction(ctx context.Context, name string, items []Item, params Params) error
	// ItemActionNames are the actions of items.
	ItemActionNames(items []Item) []string
	// SetInput filters with input.
	SetInput(input string)
	// Refresh gathers the items again.
	Refresh()
	// Quit closes the UI; the session stays, to be resumed.
	Quit(ctx context.Context) error
	// Preview is what the kind of item shows for it.
	Preview(ctx context.Context, item Item, params Params, pc PreviewContext) (*Previewer, error)
	// ItemAt is the item at row of the view of version (1-based), as the
	// UI drew it.
	ItemAt(version int64, row int) (Item, bool)
	// Item is the item of this ID.
	Item(id uint64) (Item, bool)
	// Selected are the items selected, in the order shown.
	Selected() []Item
	// ToggleSelect selects the item, or not.
	ToggleSelect(id uint64)
	// ToggleSelectAll selects every item, or not.
	ToggleSelectAll()
	// ClearSelect selects none.
	ClearSelect()
}

// View is a state of a session, for a UI to show.
type View struct {
	// Version grows with every View of a session.
	Version int64
	Context Context
	Options *Options
	// UIOptions and UIParams of the UI.
	UIOptions UIOptions
	UIParams  Params
	// Items are those of the sources, filtered, in order.
	Items []Item
	// Selected are the IDs of the items selected.
	Selected map[uint64]bool
	// Sources are the names of the sources, in order.
	Sources []string
	// Cursor, when not 0, is the item ID the UI moves the cursor to (after
	// an action that asks for it, SearchPath).
	Cursor uint64
	// InitCursor: put the cursor on the first item (a new session).
	InitCursor bool
	// Open: the UI is not shown (it was closed, or never shown): show it
	// and focus it. Without it the UI updates what it shows only.
	Open bool
	// Input is the input as it was typed: Context.Input is what filters,
	// expanded with the option expandInput.
	Input string
}
