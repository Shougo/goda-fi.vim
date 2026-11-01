package fi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
)

// Options configure a session, as ddu.vim's options: the JSON names are
// ddu.vim's. They are both what is in effect and a patch: PatchGlobal and
// PatchLocal (and goda_fi#custom#patch_global() and the options of
// goda_fi#start()) merge one into what is there. Unset fields (nil, "") keep
// what is there; maps are merged by key, recursively; slices are replaced.
type Options struct {
	// Name of the session: a session is resumed by its name.
	Name string `json:"name,omitempty"`
	// UI is the name of the UI added to the builder, such as "ff".
	UI string `json:"ui,omitempty"`
	// Sources in order: a name, or {name, options, params}.
	Sources []SourceSpec `json:"sources"`

	SourceOptions map[string]SourceOptions `json:"sourceOptions"`
	SourceParams  map[string]Params        `json:"sourceParams"`
	FilterOptions map[string]FilterOptions `json:"filterOptions"`
	FilterParams  map[string]Params        `json:"filterParams"`
	KindOptions   map[string]KindOptions   `json:"kindOptions"`
	KindParams    map[string]Params        `json:"kindParams"`
	UIOptions     map[string]UIOptions     `json:"uiOptions"`
	UIParams      map[string]Params        `json:"uiParams"`
	ActionOptions map[string]ActionOptions `json:"actionOptions"`
	ActionParams  map[string]Params        `json:"actionParams"`
	// ColumnParams are the params of the columns, by name.
	ColumnParams map[string]Params `json:"columnParams"`

	// PostFilters run on the items of all the sources together.
	PostFilters []string `json:"postFilters"`
	// Actions, when set, are the only item actions offered.
	Actions []string `json:"actions"`

	// Input filters the items from the start.
	Input *string `json:"input,omitempty"`
	// Resume shows the session of the same name as it was, without
	// gathering again.
	Resume *bool `json:"resume,omitempty"`
	// Refresh gathers again when resuming.
	Refresh *bool `json:"refresh,omitempty"`
	// Push keeps the session it replaces, for pop (goda_fi#pop()).
	Push *bool `json:"push,omitempty"`
	// Sync: the UI is drawn once all the items are gathered.
	Sync *bool `json:"sync,omitempty"`
	// SearchPath puts the cursor on the item of this path.
	SearchPath string `json:"searchPath,omitempty"`
	// Unique drops the items whose word another item has.
	Unique *bool `json:"unique,omitempty"`
	// ExpandInput: the input is expanded by Vim's expand() ("~/", "$HOME",
	// "%:h", ...), unless it starts with "<". Useful for the sources of
	// paths.
	ExpandInput *bool `json:"expandInput,omitempty"`
}

// SourceSpec is a source of Options.Sources: in JSON, a name or
// {"name": ..., "options": ..., "params": ...}.
type SourceSpec struct {
	Name    string         `json:"name"`
	Options *SourceOptions `json:"options,omitempty"`
	Params  Params         `json:"params,omitempty"`
}

func (s *SourceSpec) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err == nil {
		*s = SourceSpec{Name: name}
		return nil
	}
	type plain SourceSpec
	var p plain
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return err
	}
	*s = SourceSpec(p)
	return nil
}

// Sources are SourceSpecs of names only: Sources: fi.Sources("file", "buffer").
func Sources(names ...string) []SourceSpec {
	out := make([]SourceSpec, len(names))
	for i, n := range names {
		out[i] = SourceSpec{Name: n}
	}
	return out
}

// SourceOptions are the options of a source, by name ("_" for every one).
type SourceOptions struct {
	// Matchers, Sorters and Converters are the filters of the items, by
	// name. [] (not nil) sets none.
	Matchers   []string `json:"matchers"`
	Sorters    []string `json:"sorters"`
	Converters []string `json:"converters"`
	// DefaultAction of the items, before that of their kind.
	DefaultAction string `json:"defaultAction,omitempty"`
	// IgnoreCase and SmartCase are for the matchers.
	IgnoreCase *bool `json:"ignoreCase,omitempty"`
	SmartCase  *bool `json:"smartCase,omitempty"`
	// MatcherKey is the attribute matched: "word" (default) or "display".
	MatcherKey string `json:"matcherKey,omitempty"`
	// MaxItems bounds the items of the source (default 10000).
	MaxItems *int `json:"maxItems,omitempty"`
	// Path of the source (a directory for file, file_rec, ...); the
	// current directory when empty.
	Path string `json:"path,omitempty"`
	// Preview: the items can be previewed (default true).
	Preview *bool `json:"preview,omitempty"`
	// Volatile: the source gathers again when the input changes (as rg,
	// which searches for it); the matchers apply to its items too.
	Volatile *bool `json:"volatile,omitempty"`
	// Columns make what the items show (Display), by name, one after the
	// other ("filename", say).
	Columns []string `json:"columns"`
	// LimitPath: an action does not change Path to a directory that is not
	// it or under it (narrow "..", say).
	LimitPath string `json:"limitPath,omitempty"`
}

// FilterOptions are the options of a filter, by name ("_" for every one).
type FilterOptions struct {
	// MinInputLength: the filter is skipped for a shorter input.
	MinInputLength *int `json:"minInputLength,omitempty"`
}

// KindOptions are the options of a kind, by name ("_" for every one).
type KindOptions struct {
	// DefaultAction is the action of "default" (the UI's itemAction
	// without a name).
	DefaultAction string `json:"defaultAction,omitempty"`
	// Actions: other names for actions ({"o": "open"}).
	Actions map[string]string `json:"actions"`
}

// UIOptions are the options of a UI, by name ("_" for every one).
type UIOptions struct {
	// DefaultAction is the item action of itemAction without a name, over
	// those of the source and the kind.
	DefaultAction string `json:"defaultAction,omitempty"`
	// FilterInputFunc is the Vim function that reads the filter
	// (prompt, text, completion), as input() (the default):
	// "cmdline#input", say.
	FilterInputFunc string `json:"filterInputFunc,omitempty"`
	// FilterInputOptsFunc, in Neovim, takes input()'s options instead.
	FilterInputOptsFunc string `json:"filterInputOptsFunc,omitempty"`
	// FilterUpdateCallback is a Vim function that gets the input as it is
	// typed and returns what filters.
	FilterUpdateCallback string `json:"filterUpdateCallback,omitempty"`
	// FilterPrompt is the prompt of the filter input.
	FilterPrompt *string `json:"filterPrompt,omitempty"`
	// FilterUpdateMax: with more items, the items are filtered when the
	// input is done only, not as it is typed (default 0: always).
	FilterUpdateMax *int `json:"filterUpdateMax,omitempty"`
	// Persist: the UI stays open after an action.
	Persist *bool `json:"persist,omitempty"`
	// Toggle: starting a session that is shown closes it.
	Toggle *bool `json:"toggle,omitempty"`
}

// ActionOptions are the options of an item action, by name ("_" for every
// one).
type ActionOptions struct {
	// Quit: the UI is closed before the action (default true).
	Quit *bool `json:"quit,omitempty"`
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func strOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

// patch is an Options as a generic map, to be merged.
type patch = map[string]any

func toPatch(v any) (patch, error) {
	if p, ok := v.(map[string]any); ok {
		return p, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var p patch
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return p, nil
}

// mergePatch merges src into dst: nil values are unset, maps are merged
// recursively, the rest is replaced. The maps of dst are not modified.
func mergePatch(dst, src patch) patch {
	out := maps.Clone(dst)
	if out == nil {
		out = patch{}
	}
	for k, v := range src {
		if v == nil {
			continue
		}
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := out[k].(map[string]any); ok {
				out[k] = mergePatch(dm, sm)
				continue
			}
			out[k] = mergePatch(nil, sm)
			continue
		}
		out[k] = v
	}
	return out
}

// decodeOptions decodes merged patches.
func decodeOptions(p patch) (Options, error) {
	var o Options
	b, err := json.Marshal(p)
	if err != nil {
		return o, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return o, fmt.Errorf("invalid options: %w", err)
	}
	return o, nil
}

// sourceOptions are the options of the source at index i: "_", those of
// its name, those of the SourceSpec.
func (o *Options) sourceOptions(i int) SourceOptions {
	spec := o.Sources[i]
	p := patch{}
	for _, v := range []any{o.SourceOptions["_"], o.SourceOptions[spec.Name], spec.Options} {
		if v == nil {
			continue
		}
		if sp, ok := v.(*SourceOptions); ok && sp == nil {
			continue
		}
		vp, err := toPatch(v)
		if err == nil {
			p = mergePatch(p, vp)
		}
	}
	var so SourceOptions
	_ = Decode(p, &so)
	return so
}

// sourceParams are the params of the source at index i.
func (o *Options) sourceParams(i int) Params {
	spec := o.Sources[i]
	return mergeParams(o.SourceParams["_"], o.SourceParams[spec.Name], spec.Params)
}

func mergeParams(ps ...Params) Params {
	out := patch{}
	for _, p := range ps {
		if p != nil {
			out = mergePatch(out, p)
		}
	}
	return Params(out)
}

func mergeNamed[T any](m map[string]T, name string) T {
	p := patch{}
	for _, k := range []string{"_", name} {
		if v, ok := m[k]; ok {
			if vp, err := toPatch(v); err == nil {
				p = mergePatch(p, vp)
			}
		}
	}
	var out T
	_ = Decode(p, &out)
	return out
}

func (o *Options) filterOptions(name string) FilterOptions {
	return mergeNamed(o.FilterOptions, name)
}

func (o *Options) kindOptions(name string) KindOptions { return mergeNamed(o.KindOptions, name) }

func (o *Options) uiOptions() UIOptions { return mergeNamed(o.UIOptions, o.UI) }

func (o *Options) actionOptions(name string) ActionOptions {
	return mergeNamed(o.ActionOptions, name)
}

func (o *Options) uiParams() Params { return mergeParams(o.UIParams["_"], o.UIParams[o.UI]) }

func (o *Options) filterParams(name string) Params {
	return mergeParams(o.FilterParams["_"], o.FilterParams[name])
}

func (o *Options) columnParams(name string) Params {
	return mergeParams(o.ColumnParams["_"], o.ColumnParams[name])
}

func (o *Options) kindParams(name string) Params {
	return mergeParams(o.KindParams["_"], o.KindParams[name])
}

func (o *Options) actionParams(name string, user Params) Params {
	return mergeParams(o.ActionParams["_"], o.ActionParams[name], user)
}
