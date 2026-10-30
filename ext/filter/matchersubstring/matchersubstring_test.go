package matchersubstring

import (
	"context"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
)

func run(t *testing.T, input string, so fi.SourceOptions, params fi.Params, words ...string) []fi.Item {
	t.Helper()
	items := make([]fi.Item, len(words))
	for i, w := range words {
		items[i] = fi.Item{Word: w}
	}
	out, err := New(Params{}).Filter(context.Background(), nil, fi.FilterArgs{
		Input: input, Items: items, SourceOptions: so, FilterParams: params,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func words(items []fi.Item) string {
	var w []string
	for _, it := range items {
		w = append(w, it.Word)
	}
	return strings.Join(w, " ")
}

func TestModes(t *testing.T) {
	all := []string{"foo.go", "bar_foo.txt", "Foo.md", "foo bar"}
	for _, c := range []struct {
		input, want string
	}{
		{"", "foo.go bar_foo.txt Foo.md foo bar"},
		{"foo", "foo.go bar_foo.txt foo bar"},
		{"foo !go", "bar_foo.txt foo bar"},
		{"^bar", "bar_foo.txt"},
		{"go$", "foo.go"},
		{"<foo", "foo.go foo bar"},
		{`foo\ bar`, "foo bar"},
		{"foo txt", "bar_foo.txt"},
	} {
		if got := words(run(t, c.input, fi.SourceOptions{}, nil, all...)); got != c.want {
			t.Errorf("%q: got %q, want %q", c.input, got, c.want)
		}
	}
}

func TestCase(t *testing.T) {
	yes := true
	so := fi.SourceOptions{IgnoreCase: &yes}
	if got := words(run(t, "foo", so, nil, "Foo.md", "x")); got != "Foo.md" {
		t.Errorf("ignoreCase: %q", got)
	}
	so = fi.SourceOptions{SmartCase: &yes}
	if got := words(run(t, "Foo", so, nil, "Foo.md", "foo")); got != "Foo.md" {
		t.Errorf("smartCase upper: %q", got)
	}
	if got := words(run(t, "foo", so, nil, "Foo.md", "foo")); got != "Foo.md foo" {
		t.Errorf("smartCase lower: %q", got)
	}
}

func TestParams(t *testing.T) {
	out := run(t, "o", fi.SourceOptions{}, fi.Params{"limit": 1, "highlightMatched": "Search"}, "xo", "yo")
	if words(out) != "xo" {
		t.Fatalf("limit: %q", words(out))
	}
	if h := out[0].Highlights; len(h) != 1 || h[0].Col != 2 || h[0].Width != 1 || h[0].HlGroup != "Search" {
		t.Errorf("highlights %+v", h)
	}
	if words(run(t, "o", fi.SourceOptions{}, fi.Params{"maxLength": 2}, "xo", "xyo")) != "xo" {
		t.Error("maxLength")
	}
	if _, err := New(Params{}).Filter(context.Background(), nil, fi.FilterArgs{Input: "x", FilterParams: fi.Params{"nope": 1}}); err == nil {
		t.Error("unknown params must be an error")
	}
}
