package rg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
)

func gather(t *testing.T, root string, so fi.SourceOptions, params fi.Params, input string) []fi.Item {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("no rg")
	}
	var out []fi.Item
	err := New(Params{}).Gather(context.Background(), nil, fi.GatherArgs{
		Context:       &fi.Context{Cwd: root, Path: root},
		SourceOptions: so,
		SourceParams:  params,
		Input:         input,
	}, func(items []fi.Item) error {
		out = append(out, items...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("foo\nbar foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func words(items []fi.Item) string {
	var w []string
	for _, it := range items {
		w = append(w, it.Text())
	}
	return strings.Join(w, "|")
}

func TestLines(t *testing.T) {
	root := tree(t)
	items := gather(t, root, fi.SourceOptions{}, fi.Params{"input": "foo"}, "")
	if got := words(items); got != "a.txt:1:1: foo|a.txt:2:5: bar foo" {
		t.Fatalf("items %q", got)
	}
	ad := items[1].Action.(file.ActionData)
	if ad.Path != filepath.Join(root, "a.txt") || ad.LineNr != 2 || ad.Col != 5 || ad.Text != "bar foo" {
		t.Errorf("action %+v", ad)
	}
	if h := items[1].Highlights; len(h) != 3 || h[2].Col != len("a.txt:2:5: ")+5 || h[2].Width != 3 {
		t.Errorf("highlights %+v", h)
	}
}

func TestJSONAndCategory(t *testing.T) {
	root := tree(t)
	items := gather(t, root, fi.SourceOptions{}, fi.Params{
		"input": "foo", "args": []any{"--json"}, "category": true,
		"highlights": map[string]any{"path": "Directory"},
	}, "")
	if got := words(items); got != "a.txt:|  foo|  bar foo" {
		t.Fatalf("items %q", got)
	}
	if items[0].Highlights[0].HlGroup != "Directory" || items[2].Action.(file.ActionData).Col != 5 {
		t.Errorf("items %+v", items)
	}
}

func TestVolatile(t *testing.T) {
	root := tree(t)
	yes := true
	so := fi.SourceOptions{Volatile: &yes}
	if items := gather(t, root, so, nil, "f"); len(items) != 0 {
		t.Errorf("shorter than minVolatileInputLength: %q", words(items))
	}
	if got := words(gather(t, root, so, nil, "nothing")); got != "b.txt:1:1: nothing" {
		t.Errorf("volatile %q", got)
	}
	// A pattern being typed is no error; nothing found is none.
	gather(t, root, so, nil, "fo(")
	gather(t, root, fi.SourceOptions{}, fi.Params{"input": "zzz"}, "")
}
