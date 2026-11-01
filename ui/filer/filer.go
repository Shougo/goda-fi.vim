// Package filer is the filer UI of goda-fi.vim, as ddu-ui-filer: the items
// of trees in a buffer (goda-fi-filer-{name}, filetype "goda-fi-filer"),
// with the path of each source at the top (displayRoot), sorted.
//
// It is the UI of ui/ff with these defaults (splitDirection "aboveleft",
// displayRoot): the same params, the same actions, and its own buffer.
package filer

import "github.com/Shougo/goda-fi.vim/ui/ff"

// New returns the UI "filer", to add to the builder.
func New() *ff.UI { return ff.NewFiler() }
