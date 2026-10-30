package filerec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
)

func gather(t *testing.T, root string, p Params, params fi.Params) []string {
	t.Helper()
	var words []string
	err := New(p).Gather(context.Background(), nil, fi.GatherArgs{
		Context:      &fi.Context{Cwd: root, Path: root},
		SourceParams: params,
	}, func(items []fi.Item) error {
		for _, it := range items {
			words = append(words, it.Word)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return words
}

func tree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGather(t *testing.T) {
	root := tree(t, "a", "d/b", "d/e/c", ".git/HEAD", "node_modules/x")
	got := strings.Join(gather(t, root, Params{}, nil), " ")
	if got != "a d/b d/e/c node_modules/x" {
		t.Errorf("default: %q", got)
	}
	got = strings.Join(gather(t, root, Params{ChunkSize: 1}, fi.Params{"ignoredDirectories": []any{"node_modules", "e"}}), " ")
	if got != "a .git/HEAD d/b" {
		t.Errorf("ignored: %q", got)
	}
}

func TestLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links")
	}
	root := tree(t, "d/a")
	if err := os.Symlink(filepath.Join(root, "d"), filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "d", "loop")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(gather(t, root, Params{}, nil), " "); got != "l d/a d/loop" {
		t.Errorf("links: %q", got)
	}
	// Every directory once: l is d.
	if got := strings.Join(gather(t, root, Params{ExpandSymbolicLink: true}, nil), " "); got != "d/a" && got != "l/a" {
		t.Errorf("expanded: %q", got)
	}
}

func TestCancel(t *testing.T) {
	root := tree(t, "a", "b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := New(Params{}).Gather(ctx, nil, fi.GatherArgs{Context: &fi.Context{Cwd: root}}, func([]fi.Item) error { return nil })
	if err == nil {
		t.Error("a cancelled gather must stop")
	}
}
