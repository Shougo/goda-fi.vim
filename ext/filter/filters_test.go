package filter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Shougo/goda-fi.vim/ext/filter/converterhldir"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherfiles"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherhidden"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherrelative"
	"github.com/Shougo/goda-fi.vim/ext/filter/sorteralpha"
	"github.com/Shougo/goda-fi.vim/ext/filter/sortermtime"
	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
)

func items(paths ...string) []fi.Item {
	out := make([]fi.Item, len(paths))
	for i, p := range paths {
		out[i] = fi.Item{Word: p, Action: file.ActionData{Path: "/r/" + p}}
	}
	return out
}

func run(t *testing.T, f fi.Filter, a fi.FilterArgs) string {
	t.Helper()
	out, err := f.Filter(context.Background(), nil, a)
	if err != nil {
		t.Fatal(err)
	}
	var w []string
	for _, it := range out {
		w = append(w, it.Word)
	}
	return strings.Join(w, " ")
}

func TestMatcherHidden(t *testing.T) {
	a := fi.FilterArgs{Items: items("a", ".b", "c/.d", ".e/f", "g/.h/i/j")}
	// The name and its directory only, as ddu-filter-matcher_hidden.
	if got := run(t, matcherhidden.New(), a); got != "a g/.h/i/j" {
		t.Errorf("hidden: %q", got)
	}
	a.Input = "."
	if got := run(t, matcherhidden.New(), a); got != "a .b c/.d .e/f g/.h/i/j" {
		t.Errorf("with a dot: %q", got)
	}
}

func TestMatcherRelative(t *testing.T) {
	// Absolute paths of this system ("/r" is not one on MS-Windows).
	root := t.TempDir()
	base := filepath.Join(root, "r")
	its := []fi.Item{
		{Word: "in", Action: file.ActionData{Path: filepath.Join(base, "in")}},
		{Word: "out", Action: file.ActionData{Path: filepath.Join(root, "elsewhere", "out")}},
		{Word: "none"},
	}
	a := fi.FilterArgs{Items: its, Context: &fi.Context{Cwd: base, Path: base}}
	if got := run(t, matcherrelative.New(), a); got != "in" {
		t.Errorf("relative: %q", got)
	}
}

func TestMatcherFiles(t *testing.T) {
	a := fi.FilterArgs{Items: items("a.go", "x/b.go", "x/c.txt", "test/d.vim")}
	f := matcherfiles.New(matcherfiles.Params{})
	if got := run(t, f, a); got != "a.go x/b.go x/c.txt test/d.vim" {
		t.Errorf("none: %q", got)
	}
	a.FilterParams = fi.Params{"globs": []any{"*.go"}}
	if got := run(t, f, a); got != "a.go x/b.go" {
		t.Errorf("globs: %q", got)
	}
	a.FilterParams = fi.Params{"globs": []any{"test/*.{vim,lua}"}, "patterns": []any{`\.txt$`}}
	if got := run(t, f, a); got != "x/c.txt test/d.vim" {
		t.Errorf("patterns: %q", got)
	}
}

func TestSorters(t *testing.T) {
	a := fi.FilterArgs{Items: items("b", "a", "c")}
	if got := run(t, sorteralpha.New(), a); got != "a b c" {
		t.Errorf("alpha: %q", got)
	}
	dir := t.TempDir()
	var its []fi.Item
	for i, name := range []string{"old", "new", "mid"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		age := map[int]time.Duration{0: 3 * time.Hour, 1: 0, 2: time.Hour}[i]
		mt := time.Now().Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		its = append(its, fi.Item{Word: name, Action: file.ActionData{Path: p}})
	}
	its = append(its, fi.Item{Word: "status", Status: &fi.Status{Time: time.Now().Add(time.Hour).UnixMilli()}})
	if got := run(t, sortermtime.New(), fi.FilterArgs{Items: its}); got != "status new mid old" {
		t.Errorf("mtime: %q", got)
	}
}

func TestConverterHlDir(t *testing.T) {
	f := converterhldir.New(converterhldir.Params{})
	out, err := f.Filter(context.Background(), nil, fi.FilterArgs{
		Items:        items("ab/c/d.go", "e"),
		FilterParams: fi.Params{"hlGroup": []any{"Directory", "Keyword"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := out[0].Highlights
	if len(h) != 2 || h[0] != (fi.Highlight{Name: "goda_fi_dir_0", HlGroup: "Directory", Col: 1, Width: 3}) ||
		h[1] != (fi.Highlight{Name: "goda_fi_dir_1", HlGroup: "Keyword", Col: 4, Width: 2}) {
		t.Errorf("highlights %+v", h)
	}
	if len(out[1].Highlights) != 0 {
		t.Errorf("no directory: %+v", out[1].Highlights)
	}
}
