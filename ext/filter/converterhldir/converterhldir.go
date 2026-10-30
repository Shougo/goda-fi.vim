// Package converterhldir is the filter "converterhldir" of goda-fi.vim, as
// ddu-filter-converter_hl_dir: the directories of the path shown are
// highlighted, by turns in the groups of hlGroup.
package converterhldir

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Groups is a highlight group, or several: in JSON a string or a list.
type Groups []string

func (g *Groups) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*g = Groups{s}
		return nil
	}
	var l []string
	if err := json.Unmarshal(b, &l); err != nil {
		return fmt.Errorf("hlGroup: a group or a list of groups")
	}
	*g = l
	return nil
}

// Params are the filter params, as ddu-filter-converter_hl_dir's.
type Params struct {
	// HlGroup: "Directory" (by default), or ["Directory", "Keyword"].
	HlGroup Groups `json:"hlGroup"`
}

type filter struct{ p Params }

// New returns the filter with these params (the filter params of the options
// go over them).
func New(p Params) fi.Filter {
	if len(p.HlGroup) == 0 {
		p.HlGroup = Groups{"Directory"}
	}
	return filter{p}
}

func (filter) Name() string { return "converterhldir" }

func (f filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	p := f.p
	if err := fi.Decode(a.FilterParams, &p); err != nil {
		return nil, err
	}
	if len(p.HlGroup) == 0 {
		return a.Items, nil
	}
	out := make([]fi.Item, len(a.Items))
	for i, it := range a.Items {
		out[i] = it
		if file.PathOf(&it) == "" {
			continue
		}
		text := it.Text()
		var hls []fi.Highlight
		col, n := 1, 0
		for {
			j := strings.IndexAny(text, `/\`)
			if j < 0 {
				break
			}
			hls = append(hls, fi.Highlight{
				Name:    fmt.Sprintf("goda_fi_dir_%d", n),
				HlGroup: p.HlGroup[n%len(p.HlGroup)],
				Col:     col,
				Width:   j + 1,
			})
			col += j + 1
			text = text[j+1:]
			n++
		}
		if len(hls) > 0 {
			out[i].Highlights = append(hls, it.Highlights...)
		}
	}
	return out, nil
}
