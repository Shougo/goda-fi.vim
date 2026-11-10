# goda-fi.vim

A fuzzy finder for Vim and Neovim, built on
[goda.vim](https://github.com/Shougo/goda.vim). The successor of
[ddu.vim](https://github.com/Shougo/ddu.vim): the same options, the same
extensions, written in Go and configured in Go — a small program you own.

Status: early. The core (sessions, gathering as items come, filters, item
actions, resume, push/pop, aliases) and the UI `ff` (ddu-ui-ff: a buffer in
a split or floating window, filtered from the command line, previews,
autoAction) work, with:

- sources: `file`, `filerec`, `fileold`, `buffer`, `filepoint`,
  `fileexternal`, `rg`, `line`, `register`, `pathhistory`
- filters: `matchersubstring`, `matcherhidden`, `matcherrelative`,
  `matcherfiles`, `sorteralpha`, `sortermtime`, `converterhldir`
- kinds: `file`, `word`, `url`
- columns: `filename`

The UIs and the extensions are in repositories of their own:
[goda-fi-ui-ff.vim](https://github.com/Shougo/goda-fi-ui-ff.vim),
[goda-fi-ui-filer.vim](https://github.com/Shougo/goda-fi-ui-filer.vim) (made
of ff: it needs goda-fi-ui-ff.vim too), and
[goda-fi-ext](https://github.com/Shougo/goda-fi-ext) (the sources, filters,
kinds and columns: Go packages, not a Vim plugin).

Trees are shown by `ff` (`displayTree`, `expandItem`) and by the filer `filer`
(ddu-ui-filer, with the column `filename`), with the file operations of
ddu-kind-file (copy, move, paste, rename, delete, trash, undo...).

What it does about the drawing: one goroutine per session takes every change
in order, every redraw carries a version and is applied by Vim in one call
(an older one is dropped), the cursor stays on its item while items come,
and a redraw waits for Vim to have drawn the previous one, so a slow Vim
skips redraws instead of queueing them.

## Try it

You need Go 1.26 or newer, and Vim 9.1 or newer (with `+job +channel`) or
Neovim 0.11 or newer. Install `Shougo/goda.vim`, `Shougo/goda-fi.vim` and the
UIs you use (`Shougo/goda-fi-ui-ff.vim`) with your plugin manager, then create
your configuration, a Go module of your own (keep it in your dotfiles;
`examples/config` is only a template to copy):

```sh
mkdir -p ~/.vim/goda-fi && cd ~/.vim/goda-fi        # Neovim: ~/.config/nvim/goda-fi
cp /path/to/goda-fi.vim/examples/config/main.go .
go mod init goda-fi-config
go get github.com/Shougo/goda-fi.vim@latest
go get github.com/Shougo/goda-fi-ui-ff.vim@latest
go get github.com/Shougo/goda-fi-ext@latest
go mod tidy
```

The configuration, as a ddu.vim one (ddu.vim's extension names lose their
`_`: `file_rec` is `filerec`):

```go
package main

import (
	"github.com/Shougo/goda-fi-ext/ext/filter/matchersubstring"
	kindfile "github.com/Shougo/goda-fi-ext/ext/kind/file"
	"github.com/Shougo/goda-fi-ext/ext/source/filerec"
	"github.com/Shougo/goda-fi-ui-ff.vim/ui/ff"
	"github.com/Shougo/goda-fi.vim/fi"
)

func main() {
	b := fi.New()
	b.AddUI(ff.New())
	b.AddSource(filerec.New(filerec.Params{}))
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
```

Build it once (and after every change of the configuration):

```vim
:call goda_fi#build()
```

Then, with mappings in the buffer of the UI as with ddu-ui-ff:

```vim
nnoremap <Space>f <Cmd>call goda_fi#start({'sources': ['filerec']})<CR>

autocmd FileType goda-fi-ff call s:ff()
function! s:ff() abort
  nnoremap <buffer> <CR> <Cmd>call goda_fi#ui#do_action('itemAction')<CR>
  nnoremap <buffer> i <Cmd>call goda_fi#ui#do_action('openFilterWindow')<CR>
  nnoremap <buffer> <Space> <Cmd>call goda_fi#ui#do_action('toggleSelectItem')<CR>
  nnoremap <buffer> p <Cmd>call goda_fi#ui#do_action('preview')<CR>
  nnoremap <buffer> q <Cmd>call goda_fi#ui#do_action('quit')<CR>
endfunction
```

If nothing happens, `:echo goda#log()` shows what the plugin said.

Everything else is in `:help goda_fi` ([doc/goda_fi.txt](doc/goda_fi.txt)).

## Tests

```sh
go test ./...                      # unit tests
cd e2e && go test ./...            # real Vim and Neovim (a module of its own)
```

## License

MIT; see [LICENSE](LICENSE).
