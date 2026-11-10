// The binary of the end-to-end tests: the ff UI, and the sources, filters and
// kinds of this repository.
package main

import (
	"github.com/Shougo/goda-fi-ext/ext/column/filename"
	"github.com/Shougo/goda-fi-ext/ext/filter/converterhldir"
	"github.com/Shougo/goda-fi-ext/ext/filter/matcherfiles"
	"github.com/Shougo/goda-fi-ext/ext/filter/matcherhidden"
	"github.com/Shougo/goda-fi-ext/ext/filter/matcherrelative"
	"github.com/Shougo/goda-fi-ext/ext/filter/matchersubstring"
	"github.com/Shougo/goda-fi-ext/ext/filter/sorteralpha"
	"github.com/Shougo/goda-fi-ext/ext/filter/sortermtime"
	kindfile "github.com/Shougo/goda-fi-ext/ext/kind/file"
	"github.com/Shougo/goda-fi-ext/ext/kind/url"
	"github.com/Shougo/goda-fi-ext/ext/kind/word"
	"github.com/Shougo/goda-fi-ext/ext/source/buffer"
	"github.com/Shougo/goda-fi-ext/ext/source/file"
	"github.com/Shougo/goda-fi-ext/ext/source/fileexternal"
	"github.com/Shougo/goda-fi-ext/ext/source/fileold"
	"github.com/Shougo/goda-fi-ext/ext/source/filepoint"
	"github.com/Shougo/goda-fi-ext/ext/source/filerec"
	"github.com/Shougo/goda-fi-ext/ext/source/line"
	"github.com/Shougo/goda-fi-ext/ext/source/pathhistory"
	"github.com/Shougo/goda-fi-ext/ext/source/register"
	"github.com/Shougo/goda-fi-ext/ext/source/rg"
	"github.com/Shougo/goda-fi-ui-ff.vim/ui/ff"
	"github.com/Shougo/goda-fi-ui-filer.vim/ui/filer"
	"github.com/Shougo/goda-fi.vim/fi"
)

func main() {
	b := fi.New()
	b.AddUI(ff.New())
	b.AddUI(filer.New())
	b.AddColumn(filename.New(filename.DefaultParams()))
	b.AddSource(file.New(file.Params{}))
	b.AddSource(filerec.New(filerec.Params{}))
	b.AddSource(buffer.New())
	b.AddSource(filepoint.New())
	b.AddSource(fileexternal.New(fileexternal.Params{}))
	b.AddSource(fileold.New())
	b.AddSource(line.New(line.Params{}))
	b.AddSource(register.New())
	b.AddSource(pathhistory.New())
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
	b.AddKind(url.New())
	b.AddActionAlias("tabopen", "open")
	b.PatchGlobal(fi.Options{
		UI: "ff",
		SourceOptions: map[string]fi.SourceOptions{
			"_": {Matchers: []string{"matchersubstring"}},
		},
		KindOptions: map[string]fi.KindOptions{
			"file": {DefaultAction: "open"},
			"word": {DefaultAction: "append"},
			"url":  {DefaultAction: "yank"},
		},
		ActionParams: map[string]fi.Params{
			"tabopen": {"command": "tabedit"},
		},
		ActionOptions: map[string]fi.ActionOptions{
			"narrow": {Quit: new(false)},
		},
	})
	b.Main()
}
