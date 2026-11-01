package ff

import (
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
)

func TestNaturalCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"file2", "file10", -1},
		{"File10", "file2", 1},
		{"a", "a", 0},
		{"a01", "a1", 0},
		{"a", "ab", -1},
	} {
		if got := naturalCompare(c.a, c.b); got != c.want {
			t.Errorf("%q %q: %d", c.a, c.b, got)
		}
	}
}

func TestSortTree(t *testing.T) {
	// The items as the engine gives them: those of a tree under it.
	var items []fi.Item
	for _, s := range []string{"z/0/t", "z/1/f", "b/0/f", "a/0/t", "y/1/f", "c/1/t", "x/2/f", "w/1/f"} {
		p := strings.Split(s, "/")
		it := fi.Item{Word: p[0], TreePath: "/" + p[0], IsTree: p[2] == "t", Level: int(p[1][0] - '0')}
		items = append(items, it)
	}
	// TreeLevel is set by the engine: Level stands for it here.
	level := func(it *fi.Item) int { return it.Level }
	words := func(items []fi.Item) string {
		var w []string
		for _, it := range items {
			w = append(w, it.Word)
		}
		return strings.Join(w, " ")
	}
	got := sortTree(items, level, sortFunc("filename"), true)
	if words(got) != "a c x w y z z b" {
		t.Errorf("filename, trees first: %s", words(got))
	}
	got = sortTree(items, level, sortFunc("FILENAME"), false)
	if words(got) != "z z b a y w c x" {
		t.Errorf("reversed: %s", words(got))
	}
}
