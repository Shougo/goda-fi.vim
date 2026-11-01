package ff

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/Shougo/goda-fi.vim/fi"
)

// filerItems are the lines of the filer: for each source, its root item
// (displayRoot) and its items, those that are not trees matching
// fileFilter, sorted (sort, sortTreesFirst) among those of the same tree.
func filerItems(v *fi.View, p Params) ([]fi.Item, error) {
	var re *regexp.Regexp
	if p.FileFilter != "" {
		var err error
		if re, err = regexp.Compile(p.FileFilter); err != nil {
			return nil, err
		}
	}
	bySource := map[int][]fi.Item{}
	for _, it := range v.Items {
		if re != nil && !it.IsTree && !re.MatchString(it.Word) {
			continue
		}
		bySource[it.SourceIndex()] = append(bySource[it.SourceIndex()], it)
	}
	less := sortFunc(p.Sort)
	var out []fi.Item
	for i := range v.Sources {
		if p.DisplayRoot && i < len(v.Roots) {
			out = append(out, rootLine(v.Roots[i], p))
		}
		out = append(out, sortTree(bySource[i], treeLevel, less, p.SortTreesFirst)...)
	}
	return out, nil
}

// rootLine is the root item as shown: "{source}:{path}", "~" for the home
// directory.
func rootLine(root fi.Item, p Params) fi.Item {
	path := root.Word
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path == home {
			path = "~"
		} else if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
			path = "~/" + filepath.ToSlash(rest)
		}
	}
	name := root.SourceName()
	root.Display = name + ":" + path
	root.Highlights = []fi.Highlight{
		{Name: "root-source-name", HlGroup: or(p.Highlights["sourceName"], "Type"), Col: 1, Width: len(name)},
		{Name: "root-source-path", HlGroup: or(p.Highlights["sourcePath"], "String"), Col: len(name) + 2, Width: len(path)},
	}
	return root
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// sortTree sorts the items of each tree among them, keeping the items of a
// tree under it.
func treeLevel(it *fi.Item) int { return it.TreeLevel() }

func sortTree(items []fi.Item, level func(*fi.Item) int, less func(a, b *fi.Item) int, treesFirst bool) []fi.Item {
	if (less == nil && !treesFirst) || len(items) < 2 {
		return items
	}
	type block struct{ items []fi.Item }
	// The blocks of the items of the first level: an item and those under
	// it.
	var blocks []block
	for i := 0; i < len(items); {
		j := i + 1
		for j < len(items) && level(&items[j]) > level(&items[i]) {
			j++
		}
		blocks = append(blocks, block{items[i:j]})
		i = j
	}
	slices.SortStableFunc(blocks, func(a, b block) int {
		x, y := &a.items[0], &b.items[0]
		if treesFirst && x.IsTree != y.IsTree {
			if x.IsTree {
				return -1
			}
			return 1
		}
		if less == nil {
			return 0
		}
		return less(x, y)
	})
	out := make([]fi.Item, 0, len(items))
	for _, b := range blocks {
		out = append(out, b.items[0])
		out = append(out, sortTree(b.items[1:], level, less, treesFirst)...)
	}
	return out
}

func pathOf(it *fi.Item) string {
	if it.TreePath != "" {
		return it.TreePath
	}
	return it.Word
}

func nameOf(it *fi.Item) string { return filepath.Base(strings.TrimRight(pathOf(it), `/\`)) }

// sortFunc is the comparison of the sort param (nil: none).
func sortFunc(method string) func(a, b *fi.Item) int {
	var f func(a, b *fi.Item) int
	switch strings.ToLower(method) {
	case "filename":
		f = func(a, b *fi.Item) int { return strings.Compare(pathOf(a), pathOf(b)) }
	case "natural":
		f = func(a, b *fi.Item) int { return naturalCompare(nameOf(a), nameOf(b)) }
	case "extension":
		f = func(a, b *fi.Item) int { return strings.Compare(filepath.Ext(nameOf(a)), filepath.Ext(nameOf(b))) }
	case "size":
		f = func(a, b *fi.Item) int { return cmp.Compare(size(a), size(b)) }
	case "time":
		f = func(a, b *fi.Item) int { return cmp.Compare(mtime(a), mtime(b)) }
	default:
		return nil
	}
	if strings.ToLower(method) != method {
		return func(a, b *fi.Item) int { return f(b, a) }
	}
	return f
}

func size(it *fi.Item) int64 {
	if it.Status == nil {
		return -1
	}
	return it.Status.Size
}

func mtime(it *fi.Item) int64 {
	if it.Status == nil {
		return -1
	}
	return it.Status.Time
}

// naturalCompare compares names case insensitively, numbers by their
// value ("file2" before "file10").
func naturalCompare(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[j]) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if c := cmp.Compare(len(na), len(nb)); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			continue
		}
		if c := cmp.Compare(ra[i], rb[j]); c != 0 {
			return c
		}
		i++
		j++
	}
	return cmp.Compare(len(ra)-i, len(rb)-j)
}
