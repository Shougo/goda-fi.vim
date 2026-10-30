// Command config is a user's goda-fi configuration: an ordinary Go program.
// Build it with :call goda_fi#build() (or go build) and goda-fi.vim runs the
// result.
//
// It is the counterpart of a ddu.vim configuration: extensions are imported
// (and so type checked and completed by your editor), and configured with
// PatchGlobal and PatchLocal, with ddu.vim's option names. ddu.vim's
// extension names lose their "_": "file_rec" is "filerec", "matcher_substring"
// is "matchersubstring".
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

	// Extensions: anything implementing fi.UI / fi.Source / fi.Filter /
	// fi.Kind, from this repository or from your own modules.
	b.AddUI(ff.New())
	b.AddSource(file.New(file.Params{}))
	b.AddSource(filerec.New(filerec.Params{IgnoredDirectories: []string{".git", "node_modules"}}))
	b.AddSource(buffer.New())
	b.AddSource(filepoint.New())
	b.AddSource(fileexternal.New(fileexternal.Params{Cmd: []string{"git", "ls-files"}}))
	b.AddFilter(matchersubstring.New(matchersubstring.Params{HighlightMatched: "Search"}))
	b.AddKind(kindfile.New())

	// ddu#custom#patch_global()
	b.PatchGlobal(fi.Options{
		UI: "ff",
		UIParams: map[string]fi.Params{
			"ff": {"split": "horizontal", "winHeight": 15},
		},
		SourceOptions: map[string]fi.SourceOptions{
			"_": {Matchers: []string{"matchersubstring"}, IgnoreCase: ptr(true)},
		},
		KindOptions: map[string]fi.KindOptions{
			"file": {DefaultAction: "open"},
		},
	})

	// ddu#custom#patch_local('files', ...): goda_fi#start({'name': 'files'})
	b.PatchLocal("files", fi.Options{
		Sources: fi.Sources("filerec"),
	})

	b.Main()
}

func ptr[T any](v T) *T { return &v }
