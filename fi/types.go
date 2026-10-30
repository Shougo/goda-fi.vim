// Package fi is the fuzzy finder engine of goda-fi.vim, the successor of
// ddu.vim.
//
// Its extensions are plain Go values that the user's configuration links into
// one binary (see Builder), as in ddu.vim:
//
//   - Source: gathers items, as they come (a stream).
//   - Filter: a matcher, sorter or converter, by the list it is put in
//     (SourceOptions.Matchers, Sorters, Converters).
//   - Kind: the actions on items (open a file, ...), and their previews.
//   - UI: shows the items and takes the user's actions (ui/ff).
//
// Items are gathered, filtered and shown by one session per name (the "name"
// option): everything that changes it (items arriving, the input, actions)
// goes through one goroutine, in order, and every redraw of a UI carries a
// version number, so that nothing is drawn from an older state.
package fi

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PluginName is the name the Vim side addresses: goda#request('fi', ...).
const PluginName = "fi"

// Item is one candidate. The JSON names are those of ddu.vim, which is what
// the Vim side sees (goda_fi#ui#get_item()).
type Item struct {
	// Word is what is matched and shown, unless Display is set.
	Word    string `json:"word"`
	Display string `json:"display,omitempty"`
	// Action is the data of the kind: what its actions work on (a path,
	// a buffer number, ...). It reaches the kind as the source made it.
	Action any `json:"action,omitempty"`
	// Kind of the item; the source's Kind() when empty.
	Kind       string      `json:"kind,omitempty"`
	Highlights []Highlight `json:"highlights,omitempty"`
	// Info are lines shown under the item.
	Info   []Info  `json:"info,omitempty"`
	Status *Status `json:"status,omitempty"`
	// Data belongs to the source; it stays in the plugin process.
	Data any `json:"-"`

	// Set by the engine.
	id          uint64
	sourceIndex int
	sourceName  string
	matcherKey  string
}

// Highlight highlights Width bytes from the byte column Col (1-based, as in
// ddu.vim) of the text of an item (Display, or Word).
type Highlight struct {
	Name    string `json:"name"`
	HlGroup string `json:"hl_group"`
	Col     int    `json:"col"`
	Width   int    `json:"width"`
}

// Info is a line shown under an item.
type Info struct {
	Text    string `json:"text"`
	HlGroup string `json:"hl_group,omitempty"`
}

// Status of the item: of a file, its size and time.
type Status struct {
	Size int64 `json:"size,omitempty"`
	Time int64 `json:"time,omitempty"`
}

// Text is what the UI shows: Display, or Word.
func (i *Item) Text() string {
	if i.Display != "" {
		return i.Display
	}
	return i.Word
}

// SourceName is the name of the source that gathered the item.
func (i *Item) SourceName() string { return i.sourceName }

// SourceIndex is the index in Options.Sources of the source that gathered
// the item.
func (i *Item) SourceIndex() int { return i.sourceIndex }

// MatcherKey is what matchers compare: the attribute of the item named by
// SourceOptions.MatcherKey (default Word).
func (i *Item) MatcherKey() string {
	if i.matcherKey == "" {
		return i.Word
	}
	return i.matcherKey
}

// ID identifies the item in its session (a regather makes new ones).
func (i *Item) ID() uint64 { return i.id }

// Context is where a session runs, as ddu.vim's context.
type Context struct {
	// Name of the session (Options.Name).
	Name string `json:"name"`
	// Where it was started from: the buffer, the window, the directory.
	BufName string `json:"bufName"`
	BufNr   int    `json:"bufNr"`
	WinID   int    `json:"winId"`
	Cwd     string `json:"cwd"`
	// Mode of Vim when it was started ("n", "i", "c", ...).
	Mode string `json:"mode"`
	// Input is the text filtering the items.
	Input string `json:"input"`
	// Done: every source has gathered all its items.
	Done bool `json:"done"`
	// MaxItems is how many items were gathered.
	MaxItems int `json:"maxItems"`
	// Path is the path of the sources (SourceOptions.Path), when they have
	// one.
	Path string `json:"path"`
}

// ActionFlags say what follows an action.
type ActionFlags int

const (
	// None: the UI is closed (unless the action option quit is false or
	// the UI option persist is set).
	None ActionFlags = 0
	// RefreshItems: the items are gathered again.
	RefreshItems ActionFlags = 1 << 0
	// Redraw: the UI is drawn again.
	Redraw ActionFlags = 1 << 1
	// Persist: the UI stays open.
	Persist ActionFlags = 1 << 2
	// RestoreCursor: the cursor goes back to the window it was in.
	RestoreCursor ActionFlags = 1 << 3
)

// ActionResult is what an action returns.
type ActionResult struct {
	Flags ActionFlags
	// SearchPath: the UI puts the cursor on the item of this path.
	SearchPath string
}

// Previewer is what a kind shows of an item in a preview window, as in
// ddu.vim: Kind is "buffer" (a file or a buffer), "nofile" (Contents) or
// "terminal" (Cmds run in a terminal).
type Previewer struct {
	Kind string `json:"kind"`
	// buffer
	Path        string `json:"path,omitempty"`
	Expr        any    `json:"expr,omitempty"`
	UseExisting bool   `json:"useExisting,omitempty"`
	// nofile
	Contents []string `json:"contents,omitempty"`
	// terminal
	Cmds []string `json:"cmds,omitempty"`
	Cwd  string   `json:"cwd,omitempty"`
	// buffer and nofile
	LineNr   int    `json:"lineNr,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Syntax   string `json:"syntax,omitempty"`
	Filetype string `json:"filetype,omitempty"`
}

// PreviewContext is where the preview goes.
type PreviewContext struct {
	Row        int    `json:"row"`
	Col        int    `json:"col"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	IsFloating bool   `json:"isFloating"`
	Split      string `json:"split"`
}

// Params are the parameters of an extension, by name, as the configuration
// gives them. An extension decodes them into its own struct with Decode.
type Params map[string]any

// Decode decodes v (Params, or any value of generic JSON shape) into dst, a
// pointer to a struct holding the defaults: the keys of v replace the fields
// of the same JSON name. Unknown keys are an error.
func Decode(v any, dst any) error {
	if v == nil {
		return nil
	}
	if p, ok := v.(Params); ok && len(p) == 0 {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}
