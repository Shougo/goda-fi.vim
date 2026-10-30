// The binary of the end-to-end tests: the ff UI, the sources and the filter
// of this repository.
package main

import (
	"github.com/Shougo/goda-fi.vim/ext/filter/matchersubstring"
	kindfile "github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/ext/source/buffer"
	"github.com/Shougo/goda-fi.vim/ext/source/file"
	"github.com/Shougo/goda-fi.vim/ext/source/fileexternal"
	"github.com/Shougo/goda-fi.vim/ext/source/filepoint"
	"github.com/Shougo/goda-fi.vim/ext/source/filerec"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda-fi.vim/ui/ff"
)

func main() {
	b := fi.New()
	b.AddUI(ff.New())
	b.AddSource(file.New(file.Params{}))
	b.AddSource(filerec.New(filerec.Params{}))
	b.AddSource(buffer.New())
	b.AddSource(filepoint.New())
	b.AddSource(fileexternal.New(fileexternal.Params{}))
	b.AddFilter(matchersubstring.New(matchersubstring.Params{}))
	b.AddKind(kindfile.New())
	b.PatchGlobal(fi.Options{
		UI: "ff",
		SourceOptions: map[string]fi.SourceOptions{
			"_": {Matchers: []string{"matchersubstring"}},
		},
		KindOptions: map[string]fi.KindOptions{
			"file": {DefaultAction: "open"},
		},
	})
	b.Main()
}
