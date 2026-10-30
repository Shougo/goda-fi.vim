// Package sortermtime is the filter "sortermtime" of goda-fi.vim, as
// ddu-filter-sorter_mtime: the items of the files changed last first, by
// their status time, or that of their path.
package sortermtime

import (
	"context"
	"os"
	"slices"
	"sync"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type filter struct {
	mu sync.Mutex
	// times of the paths without a status, by the ID of their item (an
	// item is gathered once: its file is looked at once).
	times map[uint64]int64
}

// New returns the filter.
func New() fi.Filter { return &filter{times: map[uint64]int64{}} }

func (*filter) Name() string { return "sortermtime" }

func (f *filter) time(it *fi.Item) int64 {
	if it.Status != nil && it.Status.Time != 0 {
		return it.Status.Time
	}
	id := it.ID() // 0: not gathered by the engine (tests)
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.times[id]; ok && id != 0 {
		return t
	}
	var t int64
	if p := file.PathOf(it); p != "" {
		if st, err := os.Stat(p); err == nil {
			t = st.ModTime().UnixMilli()
		}
	}
	if len(f.times) > 1000000 {
		clear(f.times)
	}
	if id != 0 {
		f.times[id] = t
	}
	return t
}

func (f *filter) Filter(_ context.Context, _ sdk.Host, a fi.FilterArgs) ([]fi.Item, error) {
	type timed struct {
		t  int64
		it fi.Item
	}
	ts := make([]timed, len(a.Items))
	for i := range a.Items {
		ts[i] = timed{f.time(&a.Items[i]), a.Items[i]}
	}
	slices.SortStableFunc(ts, func(x, y timed) int {
		switch {
		case x.t > y.t:
			return -1
		case x.t < y.t:
			return 1
		}
		return 0
	})
	out := make([]fi.Item, len(ts))
	for i, t := range ts {
		out[i] = t.it
	}
	return out, nil
}
