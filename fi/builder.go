package fi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Shougo/goda.vim/host"
	"github.com/Shougo/goda.vim/sdk"
)

// Builder collects the extensions and the configuration. It is what a user's
// main.go is written against:
//
//	b := fi.New()
//	b.AddUI(ff.New())
//	b.AddSource(filerec.New(filerec.Params{}))
//	b.AddFilter(matchersubstring.New(matchersubstring.Params{}))
//	b.AddKind(kindfile.New())
//	b.PatchGlobal(fi.Options{UI: "ff", ...})
//	b.Main()
type Builder struct {
	sources map[string]Source
	filters map[string]Filter
	kinds   map[string]Kind
	uis     map[string]UI

	actionAliases map[string]string

	global patch
	local  map[string]patch
	errs   []string
}

// New returns an empty Builder; the kind "base" (no actions) is there.
func New() *Builder {
	b := &Builder{
		sources: map[string]Source{},
		filters: map[string]Filter{},
		kinds:   map[string]Kind{},
		uis:     map[string]UI{},

		actionAliases: map[string]string{},

		global: patch{},
		local:  map[string]patch{},
	}
	b.AddKind(baseKind{})
	return b
}

func (b *Builder) fail(format string, args ...any) {
	b.errs = append(b.errs, fmt.Sprintf(format, args...))
}

func add[T interface{ Name() string }](b *Builder, what string, m map[string]T, name string, v T) *Builder {
	if _, dup := m[name]; dup && name != "base" {
		b.fail("%s %q added twice", what, name)
	}
	m[name] = v
	return b
}

// AddSource adds a source under its Name.
func (b *Builder) AddSource(s Source) *Builder { return add(b, "source", b.sources, s.Name(), s) }

// AddFilter adds a filter under its Name.
func (b *Builder) AddFilter(f Filter) *Builder { return add(b, "filter", b.filters, f.Name(), f) }

// AddKind adds a kind under its Name.
func (b *Builder) AddKind(k Kind) *Builder { return add(b, "kind", b.kinds, k.Name(), k) }

// AddUI adds a UI under its Name.
func (b *Builder) AddUI(u UI) *Builder { return add(b, "ui", b.uis, u.Name(), u) }

// AddSourceAs adds a source under another name, as ddu.vim's aliases
// (setAlias): its options and params are those of that name.
//
//	b.AddSourceAs("filegit", fileexternal.New(fileexternal.Params{
//		Cmd: []string{"git", "ls-files", "-co", "--exclude-standard"},
//	}))
func (b *Builder) AddSourceAs(name string, s Source) *Builder {
	return add(b, "source", b.sources, name, s)
}

// AddFilterAs adds a filter under another name.
func (b *Builder) AddFilterAs(name string, f Filter) *Builder {
	return add(b, "filter", b.filters, name, f)
}

// AddActionAlias makes alias another name of the action base, for every
// kind that has base (ddu.vim's setAlias("_", "action", alias, base)). The
// options and params of an action are those of the name it is called by.
func (b *Builder) AddActionAlias(alias, base string) *Builder {
	b.actionAliases[alias] = base
	return b
}

// PatchGlobal merges o into the options of every session.
func (b *Builder) PatchGlobal(o Options) *Builder {
	p, err := toPatch(o)
	if err != nil {
		b.fail("PatchGlobal: %v", err)
		return b
	}
	b.global = mergePatch(b.global, p)
	return b
}

// PatchLocal merges o into the options of the sessions named name.
func (b *Builder) PatchLocal(name string, o Options) *Builder {
	p, err := toPatch(o)
	if err != nil {
		b.fail("PatchLocal: %v", err)
		return b
	}
	b.local[name] = mergePatch(b.local[name], p)
	return b
}

// Validate reports the errors of the configuration: extensions added twice,
// names of the options that were never added.
func (b *Builder) Validate() error {
	errs := append([]string(nil), b.errs...)
	check := func(where string, p patch) {
		o, err := decodeOptions(p)
		if err != nil {
			errs = append(errs, where+": "+err.Error())
			return
		}
		errs = append(errs, b.checkNames(where, &o)...)
	}
	check("global", b.global)
	for name, p := range b.local {
		check("local "+name, mergePatch(b.global, p))
	}
	if len(errs) > 0 {
		return fmt.Errorf("goda-fi configuration:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// checkNames reports the names of extensions that the options use and that
// were not added.
func (b *Builder) checkNames(where string, o *Options) []string {
	var errs []string
	if o.UI != "" {
		if _, ok := b.uis[o.UI]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown ui %q", where, o.UI))
		}
	}
	for _, s := range o.Sources {
		if _, ok := b.sources[s.Name]; !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown source %q", where, s.Name))
		}
	}
	filters := func(list []string) {
		for _, f := range list {
			if _, ok := b.filters[f]; !ok {
				errs = append(errs, fmt.Sprintf("%s: unknown filter %q", where, f))
			}
		}
	}
	for _, so := range o.SourceOptions {
		filters(so.Matchers)
		filters(so.Sorters)
		filters(so.Converters)
	}
	filters(o.PostFilters)
	return errs
}

// Plugin validates the configuration and returns the plugin, to hand to
// host.Main with other plugins.
func (b *Builder) Plugin() (sdk.Plugin, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return newEngine(b), nil
}

// Main runs the plugin, with the other plugins given, as the whole program:
// the last line of main.go. A configuration error does not end the program:
// goda_fi#start() shows it (fi.status), and the other plugins work.
func (b *Builder) Main(others ...sdk.Plugin) {
	host.Main(append([]sdk.Plugin{b.MainPlugin()}, others...)...)
}

// MainPlugin is the plugin, for the Main of another binary (that of
// goda-cmp.vim, say): with an invalid configuration, one that tells it
// (goda_fi#start() shows it) and does nothing else.
func (b *Builder) MainPlugin() sdk.Plugin {
	p, err := b.Plugin()
	if err != nil {
		log.Print(err)
		return brokenPlugin{err}
	}
	return p
}

// brokenPlugin answers status with the configuration error, and fails the
// rest.
type brokenPlugin struct{ err error }

func (brokenPlugin) Name() string { return PluginName }

func (p brokenPlugin) Handlers() map[string]sdk.Handler {
	hs := map[string]sdk.Handler{}
	for _, name := range handlerNames {
		hs[name] = func(context.Context, sdk.Host, []any) (any, error) { return nil, p.err }
	}
	hs["status"] = func(context.Context, sdk.Host, []any) (any, error) { return p.err.Error(), nil }
	return hs
}

// baseKind is the kind of items without one: no actions.
type baseKind struct{}

func (baseKind) Name() string               { return "base" }
func (baseKind) Actions() map[string]Action { return nil }

var errNoSession = errors.New("no such session")
