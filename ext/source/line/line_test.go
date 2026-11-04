package line

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// host is a buffer of lines, as goda_fi#source#line#_lines() and _get().
type host struct {
	sdk.Host
	lines  []string
	cursor int
}

func (h host) get(begin, end int) []any {
	var out []any
	for i := begin; i <= end && i <= len(h.lines); i++ {
		out = append(out, h.lines[i-1])
	}
	return out
}

func (h host) Call(_ context.Context, fn string, args ...any) (any, error) {
	switch fn {
	case "goda_fi#source#line#_lines":
		size := args[3].(int)
		return map[string]any{
			"bufnr": 1, "begin": 1, "end": len(h.lines), "cursor": h.cursor,
			"lines": h.get(h.cursor, h.cursor+size-1),
		}, nil
	case "goda_fi#source#line#_get":
		return h.get(args[1].(int), args[2].(int)), nil
	}
	return nil, fmt.Errorf("unknown %s", fn)
}

func TestChunks(t *testing.T) {
	defer func(n int) { chunkSize = n }(chunkSize)
	chunkSize = 2
	var lines []string
	for i := range 12 {
		lines = append(lines, string(rune('a'+i)))
	}
	var emits []string
	err := New(Params{}).Gather(context.Background(), host{lines: lines, cursor: 5},
		fi.GatherArgs{Context: &fi.Context{}}, func(items []fi.Item) error {
			var d []string
			for _, it := range items {
				d = append(d, it.Display)
			}
			emits = append(emits, strings.Join(d, ","))
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	want := "05: e,06: f|07: g,08: h|09: i,10: j|11: k,12: l|01: a,02: b|03: c,04: d"
	if got := strings.Join(emits, "|"); got != want {
		t.Fatalf("emits %q, want %q", got, want)
	}
}
