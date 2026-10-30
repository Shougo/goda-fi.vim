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
  `fileexternal`, `rg`, `line`, `register`
- filters: `matchersubstring`, `matcherhidden`, `matcherrelative`,
  `matcherfiles`, `sorteralpha`, `sortermtime`, `converterhldir`
- kinds: `file`, `word`

Trees and the filer come later.

What it does about the drawing: one goroutine per session takes every change
in order, every redraw carries a version and is applied by Vim in one call
(an older one is dropped), the cursor stays on its item while items come,
and a redraw waits for Vim to have drawn the previous one, so a slow Vim
skips redraws instead of queueing them.

## Try it

You need Go 1.26 or newer, and Vim 9.1 or newer (with `+job +channel`) or
Neovim 0.10 or newer. Install `Shougo/goda.vim` and `Shougo/goda-fi.vim` with
your plugin manager, then create your configuration, a Go module of your own
(keep it in your dotfiles; `examples/config` is only a template to copy):

```sh
mkdir -p ~/.vim/goda-fi && cd ~/.vim/goda-fi        # Neovim: ~/.config/nvim/goda-fi
cp /path/to/goda-fi.vim/examples/config/main.go .
go mod init goda-fi-config
go get github.com/Shougo/goda-fi.vim@latest
go mod tidy
```

The configuration, as a ddu.vim one (ddu.vim's extension names lose their
`_`: `file_rec` is `filerec`):

```go
package main

import (
	kindfile "github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/ext/filter/matchersubstring"
	"github.com/Shougo/goda-fi.vim/ext/source/filerec"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda-fi.vim/ui/ff"
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
go test ./...        # unit tests; also real Vim and Neovim if present
go test -short ./... # unit tests only
```

## License

MIT; see [LICENSE](LICENSE).
