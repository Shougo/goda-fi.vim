// The binary of the end-to-end tests: the ff UI, and the sources, filters and
// kinds of this repository.
package main

import (
	"github.com/Shougo/goda-fi.vim/ext/filter/converterhldir"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherfiles"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherhidden"
	"github.com/Shougo/goda-fi.vim/ext/filter/matcherrelative"
	"github.com/Shougo/goda-fi.vim/ext/filter/matchersubstring"
	"github.com/Shougo/goda-fi.vim/ext/filter/sorteralpha"
	"github.com/Shougo/goda-fi.vim/ext/filter/sortermtime"
	kindfile "github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/ext/kind/word"
	"github.com/Shougo/goda-fi.vim/ext/source/buffer"
	"github.com/Shougo/goda-fi.vim/ext/source/file"
	"github.com/Shougo/goda-fi.vim/ext/source/fileexternal"
	"github.com/Shougo/goda-fi.vim/ext/source/fileold"
	"github.com/Shougo/goda-fi.vim/ext/source/filepoint"
	"github.com/Shougo/goda-fi.vim/ext/source/filerec"
	"github.com/Shougo/goda-fi.vim/ext/source/line"
	"github.com/Shougo/goda-fi.vim/ext/source/register"
	"github.com/Shougo/goda-fi.vim/ext/source/rg"
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
	b.AddSource(fileold.New())
	b.AddSource(line.New(line.Params{}))
	b.AddSource(register.New())
	b.AddSource(rg.New(rg.Params{}))
	// An alias with params of its own.
	b.AddSourceAs("filels", fileexternal.New(fileexternal.Params{Cmd: []string{"ls"}}))
	b.AddFilter(matchersubstring.New(matchersubstring.Params{}))
	b.AddFilter(matcherhidden.New())
	b.AddFilter(matcherrelative.New())
	b.AddFilter(matcherfiles.New(matcherfiles.Params{}))
	b.AddFilter(sorteralpha.New())
	b.AddFilter(sortermtime.New())
	b.AddFilter(converterhldir.New(converterhldir.Params{}))
	b.AddKind(kindfile.New())
	b.AddKind(word.New())
	b.AddActionAlias("tabopen", "open")
	b.PatchGlobal(fi.Options{
		UI: "ff",
		SourceOptions: map[string]fi.SourceOptions{
			"_": {Matchers: []string{"matchersubstring"}},
		},
		KindOptions: map[string]fi.KindOptions{
			"file": {DefaultAction: "open"},
			"word": {DefaultAction: "append"},
		},
		ActionParams: map[string]fi.Params{
			"tabopen": {"command": "tabedit"},
		},
	})
	b.Main()
}
