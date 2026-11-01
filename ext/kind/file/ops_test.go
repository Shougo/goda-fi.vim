package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// host answers the questions of the actions as the test says.
type host struct {
	answers map[string][]any
	calls   []string
}

func (h *host) Meta() sdk.Meta { return sdk.Meta{Host: sdk.HostNone} }
func (h *host) Call(_ context.Context, fn string, args ...any) (any, error) {
	h.calls = append(h.calls, fmt.Sprint(fn, args))
	name := strings.TrimPrefix(fn, "goda_fi#kind#file#")
	if list := h.answers[name]; len(list) > 0 {
		h.answers[name] = list[1:]
		return list[0], nil
	}
	return nil, nil
}
func (h *host) Notify(context.Context, string, ...any) error { return nil }
func (h *host) Eval(context.Context, string) (any, error)    { return nil, nil }
func (h *host) Cmd(context.Context, string) error            { return nil }

// tree makes files (a name ending with "/" is a directory) under a new
// directory.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// files are the files under dir, "/" after the directories.
func files(t *testing.T, dir string) string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return strings.Join(out, " ")
}

func item(path string) fi.Item {
	return fi.Item{Word: filepath.Base(path), Action: ActionData{Path: path}}
}

type run struct {
	t    *testing.T
	k    Kind
	h    *host
	root string
}

func (r *run) do(name string, items []fi.Item, params fi.Params, answers map[string][]any) fi.ActionResult {
	r.t.Helper()
	r.h.answers = answers
	res, err := r.k.Actions()[name].Run(context.Background(), r.h, fi.ActionArgs{
		Context:       &fi.Context{Cwd: r.root, Path: r.root},
		SourceOptions: fi.SourceOptions{Path: r.root},
		ActionParams:  params,
		Items:         items,
	})
	if err != nil {
		r.t.Fatalf("%s: %v", name, err)
	}
	return res
}

func TestClipboard(t *testing.T) {
	root := tree(t, "a.txt", "sub/b.txt", "dir/x/y.txt")
	r := &run{t: t, k: New(), h: &host{}, root: root}
	// Copied into sub (the directory of the item), and a directory.
	r.do("copy", []fi.Item{item(filepath.Join(root, "a.txt")), item(filepath.Join(root, "dir"))}, nil, nil)
	res := r.do("paste", []fi.Item{item(filepath.Join(root, "sub", "b.txt"))}, nil, nil)
	if got := files(t, root); got != "a.txt dir/ dir/x/ dir/x/y.txt sub/ sub/a.txt sub/b.txt sub/dir/ sub/dir/x/ sub/dir/x/y.txt" {
		t.Errorf("copy: %s", got)
	}
	if res.Flags != fi.RefreshItems || res.SearchPath != filepath.Join(root, "sub", "dir") {
		t.Errorf("copy result %+v", res)
	}
	// There already: "Underbar".
	r.do("paste", []fi.Item{item(filepath.Join(root, "sub", "b.txt"))}, nil, map[string][]any{"confirm": {int64(3), int64(4)}})
	if !strings.Contains(files(t, root), "sub/a.txt_") || strings.Contains(files(t, root), "sub/dir_") {
		t.Errorf("underbar: %s", files(t, root))
	}
	// Undone: the copies are gone.
	r.do("undo", nil, nil, map[string][]any{"confirm": {int64(1)}})
	if strings.Contains(files(t, root), "sub/a.txt_") || !strings.Contains(files(t, root), "sub/a.txt ") {
		t.Errorf("undo: %s", files(t, root))
	}

	// Moved, and back with undo.
	r.do("move", []fi.Item{item(filepath.Join(root, "a.txt"))}, nil, nil)
	r.do("paste", []fi.Item{item(filepath.Join(root, "dir", "x"))}, nil, nil)
	if got := files(t, root); strings.HasPrefix(got, "a.txt") || !strings.Contains(got, "dir/a.txt") {
		t.Errorf("move: %s", got)
	}
	r.do("undo", nil, nil, map[string][]any{"confirm": {int64(1)}})
	if got := files(t, root); !strings.HasPrefix(got, "a.txt") || strings.Contains(got, "dir/a.txt") {
		t.Errorf("undo move: %s", got)
	}

	// A relative link.
	r.do("link", []fi.Item{item(filepath.Join(root, "a.txt"))}, fi.Params{"mode": "relative"}, nil)
	r.do("paste", []fi.Item{item(filepath.Join(root, "dir", "x", "y.txt"))}, nil, nil)
	if target, err := os.Readlink(filepath.Join(root, "dir", "x", "a.txt")); err != nil || target != filepath.Join("..", "..", "a.txt") {
		t.Errorf("link: %q %v", target, err)
	}
}

func TestNewRenameDelete(t *testing.T) {
	root := tree(t, "a.txt", "sub/")
	r := &run{t: t, k: New(), h: &host{}, root: root}
	res := r.do("newFile", []fi.Item{item(filepath.Join(root, "a.txt"))}, nil,
		map[string][]any{"cwd_input": {"n.txt, d/ ,a.txt"}})
	if got := files(t, root); got != "a.txt d/ n.txt sub/" {
		t.Errorf("newFile: %s", got)
	}
	if res.SearchPath != filepath.Join(root, "d") {
		t.Errorf("newFile search %q", res.SearchPath)
	}
	if !slices.ContainsFunc(r.h.calls, func(c string) bool { return strings.Contains(c, "already exists") }) {
		t.Errorf("not told: %q", r.h.calls)
	}
	r.do("undo", nil, nil, map[string][]any{"confirm": {int64(1)}})
	if got := files(t, root); got != "a.txt sub/" {
		t.Errorf("undo newFile: %s", got)
	}
	r.do("newDirectory", []fi.Item{item(filepath.Join(root, "a.txt"))}, nil,
		map[string][]any{"cwd_input": {"e/f"}})
	if got := files(t, root); got != "a.txt e/ e/f/ sub/" {
		t.Errorf("newDirectory: %s", got)
	}

	// Renamed (relative to the path of the source), and back.
	r.do("rename", []fi.Item{item(filepath.Join(root, "a.txt"))}, nil,
		map[string][]any{"modified": {false}, "cwd_input": {"sub/z.txt"}})
	if got := files(t, root); got != "e/ e/f/ sub/ sub/z.txt" {
		t.Errorf("rename: %s", got)
	}
	r.do("undo", nil, nil, map[string][]any{"confirm": {int64(1)}})
	if got := files(t, root); got != "a.txt e/ e/f/ sub/" {
		t.Errorf("undo rename: %s", got)
	}
	// A modified buffer is not renamed.
	r.do("rename", []fi.Item{item(filepath.Join(root, "a.txt"))}, nil,
		map[string][]any{"modified": {true}, "cwd_input": {"b.txt"}})
	if got := files(t, root); got != "a.txt e/ e/f/ sub/" {
		t.Errorf("modified: %s", got)
	}

	// Deleted when it is confirmed.
	r.do("delete", []fi.Item{item(filepath.Join(root, "e"))}, nil, map[string][]any{"confirm": {int64(2)}})
	if got := files(t, root); got != "a.txt e/ e/f/ sub/" {
		t.Errorf("not confirmed: %s", got)
	}
	res = r.do("delete", []fi.Item{item(filepath.Join(root, "e"))}, nil, map[string][]any{"confirm": {int64(1)}})
	if got := files(t, root); got != "a.txt sub/" || res.Flags != fi.RefreshItems {
		t.Errorf("delete: %s %+v", got, res)
	}
}

func TestSafeActionSelf(t *testing.T) {
	root := tree(t, "a.txt")
	p := filepath.Join(root, "a.txt")
	// Renamed to itself: still there.
	if err := safeAction("rename", p, p); err != nil {
		t.Fatal(err)
	}
	if got := files(t, root); got != "a.txt" {
		t.Errorf("self: %s", got)
	}
	if err := copyPath(root, filepath.Join(root, "in")); err == nil {
		t.Error("a directory copied into itself")
	}
}
