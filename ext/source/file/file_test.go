package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
)

func gather(t *testing.T, root, input string, params fi.Params) ([]string, error) {
	t.Helper()
	var words []string
	err := New(Params{}).Gather(context.Background(), nil, fi.GatherArgs{
		Context:      &fi.Context{Cwd: root, Path: root},
		SourceParams: params,
		Input:        input,
	}, func(items []fi.Item) error {
		for _, it := range items {
			words = append(words, it.Text())
		}
		return nil
	})
	return words, err
}

func TestGather(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a", "sub/b"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		input, want string
		params      fi.Params
	}{
		{"", "a sub/", nil},
		{"sub/", "sub/b", nil},
		// A directory that is not there (yet): nothing, no error.
		{"nosuch/x", "", nil},
		{"nosuch/x", "[new] nosuch/x", fi.Params{"new": true}},
		{"a", "a", fi.Params{"ignoreDirectories": true}},
	} {
		got, err := gather(t, root, c.input, c.params)
		if err != nil {
			t.Errorf("%q: %v", c.input, err)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%q %v: got %q, want %q", c.input, c.params, got, c.want)
		}
	}
	// The directory of the source itself must be there.
	if _, err := gather(t, filepath.Join(root, "gone"), "", nil); err == nil {
		t.Error("a missing directory of the source must be an error")
	}
}
