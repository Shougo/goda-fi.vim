package filename

import (
	"context"
	"testing"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
)

func TestText(t *testing.T) {
	c := New(DefaultParams())
	items := []fi.Item{
		{Word: "sub/", IsTree: true, Action: file.ActionData{Path: "/t/sub", IsDirectory: true}},
		{Word: "日本.txt", Action: file.ActionData{Path: "/t/日本.txt"}},
	}
	a := fi.ColumnArgs{Items: items}
	n, err := c.Length(context.Background(), nil, a)
	if err != nil || n != 10 {
		t.Fatalf("length %d %v", n, err)
	}
	got, _ := c.Text(context.Background(), nil, a, items[0], 3, 3+n)
	if got.Text != "+ sub/    " || len(got.Highlights) != 2 ||
		got.Highlights[0] != (fi.Highlight{Name: "column-filename-icon", HlGroup: "Special", Col: 3, Width: 1}) ||
		got.Highlights[1].Col != 5 || got.Highlights[1].Width != 4 {
		t.Errorf("tree: %q %+v", got.Text, got.Highlights)
	}
	got, _ = c.Text(context.Background(), nil, fi.ColumnArgs{Items: items, ColumnParams: fi.Params{"collapsedIcon": ">"}}, items[1], 1, 1+n)
	if got.Text != "  日本.txt" || len(got.Highlights) != 0 {
		t.Errorf("file: %q %+v", got.Text, got.Highlights)
	}
}
