" The end-to-end scenario: the ff UI and the sources in a real Vim or
" Neovim. Failures go to $GODA_FI_RESULT; it says "ok" at the end.

set nocompatible
set encoding=utf-8
set hidden
let s:errors = []
let s:result = $GODA_FI_RESULT

execute 'set runtimepath^=' . fnameescape($GODA_VIM_DIR)
execute 'set runtimepath^=' . fnameescape($GODA_FI_DIR)
let g:goda_fi#binary = $GODA_FI_BINARY
execute 'cd' fnameescape($GODA_FI_TREE)

let s:events = []
augroup test
  autocmd!
  autocmd User GodaFi:uiReady call add(s:events, 'ready')
  autocmd User GodaFi:uiQuit call add(s:events, 'quit')
augroup END

function! s:check(name, got, want) abort
  if a:got !=# a:want
    call add(s:errors, printf('%s: got %s, want %s', a:name, string(a:got), string(a:want)))
  endif
endfunction

" Wait (5 seconds at most) for cond() to be true.
function! s:wait(name, cond) abort
  for _ in range(500)
    if a:cond()
      return 1
    endif
    sleep 10m
  endfor
  call add(s:errors, 'timed out: ' . a:name)
  return 0
endfunction

function! s:buf(name) abort
  return bufnr('goda-fi-ff-' . a:name)
endfunction

function! s:lines(name) abort
  return s:buf(a:name) < 0 ? [] : getbufline(s:buf(a:name), 1, '$')
endfunction

function! s:shown(name) abort
  return bufwinid(s:buf(a:name)) > 0 ? v:true : v:false
endfunction

" The lines once they are these.
function! s:wait_lines(name, want) abort
  call s:wait(a:name . ' ' . string(a:want), {-> s:lines(a:name) ==# a:want})
  return s:lines(a:name)
endfunction

" The name of the buffer, relative, with "/" (MS-Windows).
function! s:rel(bufnr) abort
  return substitute(fnamemodify(bufname(a:bufnr), ':.'), '\\', '/', 'g')
endfunction

function! s:start(options) abort
  call goda_fi#start(a:options)
endfunction

" The lines of the preview: a window, a float or a popup.
function! s:preview_lines() abort
  if has('nvim')
    for w in nvim_list_wins()
      if nvim_win_get_config(w).relative !=# '' || getwinvar(w, '&previewwindow')
        if bufname(winbufnr(w)) =~# '^goda-fi-preview'
          return getbufline(winbufnr(w), 1, '$')
        endif
      endif
    endfor
    return []
  endif
  for w in popup_list() + range(1, winnr('$'))->map('win_getid(v:val)')
    if bufname(winbufnr(w)) =~# '^goda-fi-preview'
      return getbufline(winbufnr(w), 1, '$')
    endif
  endfor
  return []
endfunction

" The filter of filterInputFunc: "@deep" ("@" taken off by the callback).
function! MyInput(prompt, text, completion) abort
  return '@deep'
endfunction

" As cmdline#input(): input() with the completion given, "" when it fails.
function! WrappedInput(prompt, text, completion) abort
  try
    return input(a:prompt, a:text, a:completion)
  catch
    let g:wrapped_error = v:exception
    return ''
  endtry
endfunction

function! MyCallback(input) abort
  return a:input[0] ==# '@' ? a:input[1:] : a:input
endfunction

let s:all = ['a.txt', 'b.go', 'msg.log', 'sub/c.txt', 'sub/deep/d.md']

" Actions as Vim functions (goda_fi#custom#action()).
function! s:grep(args) abort
  " As ddu.vim's example: rg in the directory, in a session pushed.
  call goda_fi#start({'name': a:args.options.name, 'push': v:true,
        \ 'sources': [{'name': 'rg', 'params': {
        \   'paths': [a:args.items[0].action.path], 'input': 'words'}}]})
  return 0
endfunction

function! s:mark(args) abort
  let g:marked = [a:args.context.name, a:args.actionParams,
        \ map(copy(a:args.items), 'v:val.word')]
  " Persist: the UI stays.
  return 4
endfunction

function! s:ui_hello(args) abort
  let g:ui_hello = map(copy(a:args.items), 'v:val.word')
  return 0
endfunction

function! s:run() abort
  " filerec: gathered, in a window of its own, focused.
  call s:start({'name': 'rec', 'sources': ['filerec']})
  call s:check('filerec', s:wait_lines('rec', s:all), s:all)
  call s:check('focus', bufnr('%'), s:buf('rec'))
  call s:check('windows', winnr('$'), 2)
  call s:check('filetype', &filetype, 'goda-fi-ff')
  call s:check('statusline', &l:statusline =~# '^\[goda-fi-rec\]', 1)
  call s:check('ready', s:events, ['ready'])

  " Filtered, the cursor on the first item.
  call goda_fi#redraw('rec', {'input': 'sub'})
  call s:check('filter', s:wait_lines('rec', ['sub/c.txt', 'sub/deep/d.md']),
        \ ['sub/c.txt', 'sub/deep/d.md'])
  call s:check('filter cursor', line('.'), 1)
  call s:check('status input', &l:statusline =~# ' sub$', 1)

  " The cursor moves in Vim; the action is on the item under it.
  call goda_fi#ui#do_action('cursorNext')
  call s:check('cursorNext', line('.'), 2)
  call goda_fi#ui#do_action('cursorNext', {'loop': v:true})
  call s:check('cursorNext loop', line('.'), 1)
  call s:check('get_item', get(goda_fi#ui#get_item(), 'word', ''), 'sub/c.txt')
  call goda_fi#ui#do_action('itemAction')
  call s:check('opened', s:rel('%'), 'sub/c.txt')
  call s:check('closed', [winnr('$'), s:shown('rec')], [1, v:false])
  call s:check('quit event', s:events, ['ready', 'quit'])

  " Resumed: the same input and items.
  call s:start({'name': 'rec', 'resume': v:true})
  call s:wait('resume', {-> s:shown('rec')})
  call s:check('resume', s:lines('rec'), ['sub/c.txt', 'sub/deep/d.md'])
  call s:check('resume focus', bufnr('%'), s:buf('rec'))

  " Two selected: the quickfix list.
  call goda_fi#ui#do_action('toggleSelectItem')
  call goda_fi#ui#do_action('cursorNext')
  call goda_fi#ui#do_action('toggleSelectItem')
  call goda_fi#ui#do_action('itemAction', {'name': 'quickfix'})
  call s:check('quickfix', map(getqflist(), 's:rel(v:val.bufnr)'),
        \ ['sub/c.txt', 'sub/deep/d.md'])
  cclose

  " Closed with :quit, and started again: gathered again.
  call s:start({'name': 'rec', 'sources': ['filerec']})
  call s:wait_lines('rec', s:all)
  quit
  call s:check('quit', s:shown('rec'), v:false)
  sleep 100m
  call s:check('stays closed', s:shown('rec'), v:false)
  call s:start({'name': 'rec', 'sources': ['filerec'], 'input': 'deep'})
  call s:check('started again', s:wait_lines('rec', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call goda_fi#ui#do_action('quit')
  call s:check('quit action', s:shown('rec'), v:false)

  " file: the directory.
  call s:start({'name': 'file', 'sources': ['file']})
  call s:check('file', s:wait_lines('file', ['a.txt', 'b.go', 'msg.log', 'sub/']),
        \ ['a.txt', 'b.go', 'msg.log', 'sub/'])
  " The preview of the item under the cursor.
  call goda_fi#ui#do_action('preview')
  let pwin = filter(range(1, winnr('$')), 'getwinvar(v:val, "&previewwindow")')
  call s:check('preview', len(pwin) == 1 ? getbufline(winbufnr(pwin[0]), 1, '$') : [], ['a'])
  call s:check('preview focus', bufnr('%'), s:buf('file'))
  call goda_fi#ui#do_action('quit')
  call s:check('preview closed', winnr('$'), 1)

  " A tree: the directories expanded under them (displayTree).
  call s:start({'name': 'tree', 'sources': ['file'], 'uiParams': {'ff': {'displayTree': v:true}}})
  call s:check('tree', s:wait_lines('tree', ['  a.txt', '  b.go', '  msg.log', '+ sub/']),
        \ ['  a.txt', '  b.go', '  msg.log', '+ sub/'])
  call cursor(4, 1)
  call goda_fi#ui#do_action('expandItem', {'isInTree': v:true})
  let want = ['  a.txt', '  b.go', '  msg.log', '- sub/', '   c.txt', ' + deep/']
  call s:check('expandItem', s:wait_lines('tree', want), want)
  call s:check('isInTree', line('.'), 5)
  call s:check('tree item', [goda_fi#ui#get_item().__level, goda_fi#ui#get_item().treePath =~# 'sub[\/]c\.txt$'], [1, 1])
  call cursor(6, 1)
  call goda_fi#ui#do_action('expandItem', {'mode': 'toggle'})
  let want = ['  a.txt', '  b.go', '  msg.log', '- sub/', '   c.txt', ' - deep/', '    d.md']
  call s:check('expandItem deeper', s:wait_lines('tree', want), want)
  call cursor(7, 1)
  call goda_fi#ui#do_action('collapseItem')
  let want = ['  a.txt', '  b.go', '  msg.log', '- sub/', '   c.txt', ' + deep/']
  call s:check('collapseItem', s:wait_lines('tree', want), want)
  call s:check('collapseItem cursor', line('.'), 6)
  call cursor(5, 1)
  call goda_fi#ui#do_action('itemAction')
  call s:check('tree open', s:rel('%'), 'sub/c.txt')

  " buffer: the buffers opened.
  edit b.go
  call s:start({'name': 'buf', 'sources': ['buffer']})
  call s:wait('buffer', {-> len(s:lines('buf')) == 2})
  call s:check('buffer', map(s:lines('buf'), 'matchstr(v:val, "\\S\\+$")'),
        \ ['sub/c.txt', 'b.go'])
  call s:check('buffer marks', s:lines('buf')[0] =~# '^\d\+ # ', 1)
  call goda_fi#ui#do_action('itemAction')
  call s:check('buffer open', s:rel('%'), 'sub/c.txt')

  " filepoint: the file under the cursor, at its line and column.
  edit msg.log
  call cursor(2, 3)
  call s:start({'name': 'point', 'sources': ['filepoint']})
  call s:check('filepoint', s:wait_lines('point', ['sub/c.txt:2:3']), ['sub/c.txt:2:3'])
  call goda_fi#ui#do_action('itemAction')
  call s:check('filepoint open', [s:rel('%'), line('.'), col('.')],
        \ ['sub/c.txt', 2, 3])
  " A URL: an item of the kind url.
  enew!
  call setline(1, 'see https://example.com/a?b=1 here')
  call cursor(1, 12)
  call s:start({'name': 'point', 'sources': ['filepoint']})
  call s:check('filepoint url', s:wait_lines('point', ['https://example.com/a?b=1']),
        \ ['https://example.com/a?b=1'])
  call s:check('url kind', goda_fi#ui#get_item().kind, 'url')
  call goda_fi#ui#do_action('itemAction')
  call s:check('url yank', getreg('"'), "https://example.com/a?b=1\n")
  " yank keeps the UI (Persist).
  call s:wait('url persist', {-> bufnr('%') == s:buf('point')})
  call goda_fi#ui#do_action('quit')
  enew!

  " fileexternal: what a command lists.
  let cmd = has('win32') ? ['cmd', '/c', 'echo b.go'] : ['sh', '-c', 'echo b.go; echo a.txt']
  let want = has('win32') ? ['b.go'] : ['b.go', 'a.txt']
  call s:start({'name': 'ext', 'sources': [{'name': 'fileexternal', 'params': {'cmd': cmd}}]})
  call s:check('fileexternal', s:wait_lines('ext', want), want)
  call goda_fi#ui#do_action('quit')

  " The cursor stays on its item while items arrive before it.
  if !has('win32')
    call s:start({'name': 'slow', 'sources': [
          \ {'name': 'fileexternal', 'params': {'cmd': ['sh', '-c', 'sleep 0.5; echo b.go']}},
          \ 'file']})
    call s:wait_lines('slow', ['a.txt', 'b.go', 'msg.log', 'sub/'])
    call cursor(3, 1)
    call s:check('slow', s:wait_lines('slow', ['b.go', 'a.txt', 'b.go', 'msg.log', 'sub/']),
          \ ['b.go', 'a.txt', 'b.go', 'msg.log', 'sub/'])
    call s:check('cursor kept', getline('.'), 'msg.log')
    call goda_fi#ui#do_action('quit')
  endif

  " The filter typed in the command line.
  call s:start({'name': 'rec', 'sources': ['filerec']})
  call s:wait_lines('rec', s:all)
  call feedkeys(":call goda_fi#ui#do_action('openFilterWindow')\<CR>.md\<CR>", 'xt')
  call s:check('filter window', s:wait_lines('rec', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call goda_fi#ui#do_action('quit')

  " push and pop: the same window.
  call s:start({'name': 'pp', 'sources': ['file']})
  call s:wait_lines('pp', ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call s:start({'name': 'pp', 'sources': ['filerec'], 'push': v:true})
  call s:check('push', s:wait_lines('pp', s:all), s:all)
  call goda_fi#pop('pp')
  call s:check('pop', s:wait_lines('pp', ['a.txt', 'b.go', 'msg.log', 'sub/']),
        \ ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call s:check('pop window', [winnr('$'), bufnr('%')], [2, s:buf('pp')])
  call goda_fi#ui#do_action('quit')

  " rg: the matches, opened at their line.
  if executable('rg')
    call s:start({'name': 'rg', 'sources': [{'name': 'rg', 'params': {'input': 'words'}}]})
    call s:check('rg', s:wait_lines('rg', ['sub/c.txt:2:5: two words']), ['sub/c.txt:2:5: two words'])
    call goda_fi#ui#do_action('itemAction')
    call s:check('rg open', [s:rel('%'), line('.'), col('.')], ['sub/c.txt', 2, 5])
    " Volatile: searched again as the input changes.
    call s:start({'name': 'rg', 'sources': ['rg'],
          \ 'sourceOptions': {'rg': {'volatile': v:true}}})
    call goda_fi#redraw('rg', {'input': 'three'})
    call s:check('rg volatile', s:wait_lines('rg', ['sub/c.txt:3:1: three']), ['sub/c.txt:3:1: three'])
    call goda_fi#ui#do_action('quit')
  endif

  " line: from the cursor line, then the lines before.
  edit sub/c.txt
  call cursor(2, 1)
  call s:start({'name': 'line', 'sources': ['line']})
  call s:check('line', s:wait_lines('line', ['2: two words', '3: three', '1: one']),
        \ ['2: two words', '3: three', '1: one'])
  call goda_fi#redraw('line', {'input': 'thr'})
  call s:wait_lines('line', ['3: three'])
  call goda_fi#ui#do_action('itemAction')
  call s:check('line open', [s:rel('%'), line('.')], ['sub/c.txt', 3])

  " register: appended where the session was started.
  enew!
  call setreg('a', 'hello', 'v')
  call s:start({'name': 'reg', 'sources': ['register'], 'input': 'hello'})
  call s:wait('register', {-> index(s:lines('reg'), 'a: hello') >= 0})
  execute index(s:lines('reg'), 'a: hello') + 1
  call goda_fi#ui#do_action('itemAction')
  call s:check('register append', getline(1), 'hello')
  normal! 0
  " uiOptions.defaultAction goes before that of the kind.
  call s:start({'name': 'reg', 'sources': ['register'], 'input': 'hello',
        \ 'uiOptions': {'ff': {'defaultAction': 'insert'}}})
  call s:wait('register 2', {-> s:shown('reg') && bufnr('%') == s:buf('reg')
        \ && index(s:lines('reg'), 'a: hello') >= 0})
  execute index(s:lines('reg'), 'a: hello') + 1
  call goda_fi#ui#do_action('itemAction')
  call s:check('ui defaultAction', getline(1), 'hellohello')
  enew!

  " An alias: its own params (ls), its own options.
  call s:start({'name': 'alias', 'sources': ['filels'],
        \ 'sourceOptions': {'filels': {'matchers': ['matcherhidden'],
        \   'sorters': ['sorteralpha'], 'converters': ['converterhldir']}}})
  if !has('win32')
    call s:check('alias', s:wait_lines('alias', ['a.txt', 'b.go', 'msg.log', 'sub/']),
          \ ['a.txt', 'b.go', 'msg.log', 'sub/'])
    call goda_fi#ui#do_action('itemAction', {'name': 'tabopen'})
    call s:check('action alias', [tabpagenr('$'), s:rel('%')], [2, 'a.txt'])
    tabclose
  endif
  call goda_fi#ui#do_action('quit', {}, 'alias')

  " fileold: v:oldfiles.
  let v:oldfiles = [fnamemodify('b.go', ':p'), '/no/such/file']
  call s:start({'name': 'old', 'sources': ['fileold']})
  call s:check('fileold', s:wait_lines('old', ['b.go']), ['b.go'])
  call goda_fi#ui#do_action('quit')
  " The short names of the sources: what they share left out.
  call s:start({'name': 'old', 'sources': ['fileold', 'file'],
        \ 'uiParams': {'ff': {'displaySourceName': 'short'}}})
  call s:check('short names', s:wait_lines('old', ['[fold] b.go', '[f] a.txt', '[f] b.go', '[f] msg.log', '[f] sub/']),
        \ ['[fold] b.go', '[f] a.txt', '[f] b.go', '[f] msg.log', '[f] sub/'])
  call goda_fi#ui#do_action('quit')

  " autoResize, and autoAction: the preview follows the cursor.
  call s:start({'name': 'auto', 'sources': ['file'], 'uiParams': {'ff': {
        \ 'autoResize': v:true, 'startAutoAction': v:true,
        \ 'autoAction': {'name': 'preview', 'delay': 0}}}})
  call s:wait_lines('auto', ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call s:check('autoResize', winheight(0), 4)
  call s:wait('autoAction', {-> s:preview_lines() ==# ['a']})
  call s:check('autoAction', s:preview_lines(), ['a'])
  call goda_fi#ui#do_action('cursorNext')
  call s:wait('autoAction next', {-> s:preview_lines() ==# ['package b']})
  call s:check('autoAction next', s:preview_lines(), ['package b'])
  call goda_fi#ui#do_action('toggleAutoAction')
  call s:check('toggleAutoAction', s:preview_lines(), [])
  call goda_fi#ui#do_action('quit')

  " The preview floating (a float of Neovim, a popup of Vim).
  call s:start({'name': 'float', 'sources': ['file'],
        \ 'uiParams': {'ff': {'previewFloating': v:true, 'previewFloatingBorder': 'single'}}})
  call s:wait_lines('float', ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call goda_fi#ui#do_action('preview')
  call s:check('preview floating', s:preview_lines(), ['a'])
  call goda_fi#ui#do_action('preview', {'toggle': v:true})
  call s:check('preview toggled', s:preview_lines(), [])
  call goda_fi#ui#do_action('quit')

  " split floating: Neovim only (Vim splits).
  call s:start({'name': 'float', 'sources': ['file'], 'uiParams': {'ff': {'split': 'floating'}}})
  call s:wait('float open', {-> bufnr('%') == s:buf('float')})
  call s:check('floating', has('nvim') ? nvim_win_get_config(0).relative : 'editor', 'editor')
  call s:check('floating focus', bufnr('%'), s:buf('float'))
  call goda_fi#ui#do_action('quit')
  call s:check('floating closed', s:shown('float'), v:false)

  " filterInputFunc and filterUpdateCallback.
  call s:start({'name': 'rec', 'sources': ['filerec'], 'uiOptions': {'ff': {
        \ 'filterInputFunc': 'MyInput', 'filterInputOptsFunc': '',
        \ 'filterUpdateCallback': 'MyCallback'}}})
  call s:wait_lines('rec', s:all)
  call goda_fi#ui#do_action('openFilterWindow')
  call s:check('filterInputFunc', s:wait_lines('rec', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call goda_fi#ui#do_action('quit')

  " A filterInputFunc that calls input() with the completion it is given.
  call s:start({'name': 'rec', 'sources': ['filerec'], 'uiOptions': {'ff': {
        \ 'filterInputFunc': 'WrappedInput', 'filterInputOptsFunc': '',
        \ 'filterUpdateCallback': ''}}})
  call s:wait_lines('rec', s:all)
  call feedkeys(":call goda_fi#ui#do_action('openFilterWindow')\<CR>.md\<CR>", 'xt')
  call s:check('wrapped input', s:wait_lines('rec', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call s:check('wrapped input error', get(g:, 'wrapped_error', ''), '')
  call goda_fi#ui#do_action('quit')

  " expandInput: the input expanded by expand(); the filter shows it as
  " typed.
  let $GODA_FI_PAT = 'deep'
  call s:start({'name': 'exp', 'sources': ['filerec'], 'expandInput': v:true,
        \ 'input': '$GODA_FI_PAT'})
  call s:check('expandInput', s:wait_lines('exp', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call s:check('expandInput status', &l:statusline =~# ' \$GODA_FI_PAT$', 1)
  call feedkeys(":call goda_fi#ui#do_action('openFilterWindow')\<CR>"
        \ . "\<C-u>$GODA_FI_PAT/d\<CR>", 'xt')
  call s:check('expandInput filter', s:wait_lines('exp', ['sub/deep/d.md']), ['sub/deep/d.md'])
  call goda_fi#redraw('exp', {'input': '$GODA_FI_NONE_SUCH_X'})
  call s:check('expandInput none', s:wait_lines('exp', ['']), [''])
  call goda_fi#ui#do_action('quit')

  " Actions as Vim functions, of a kind, a source and a UI.
  call goda_fi#custom#action('kind', 'file', 'mark', function('s:mark'))
  call goda_fi#custom#action('source', 'filerec,file', 'grep', function('s:grep'))
  call goda_fi#custom#action('ui', 'ff', 'hello', function('s:ui_hello'))
  call s:start({'name': 'custom', 'sources': ['file']})
  call s:wait_lines('custom', ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call s:check('custom names', index(goda#request('fi', 'itemActionNames', 'custom',
        \ [goda_fi#ui#get_item().__id]), 'mark') >= 0, 1)
  call goda_fi#ui#do_action('itemAction', {'name': 'mark', 'params': {'x': 1}})
  call s:check('custom kind', get(g:, 'marked', []), ['custom', {'x': 1}, ['a.txt']])
  " Persist: shown again (closed before the action).
  call s:wait('custom persist', {-> bufnr('%') == s:buf('custom')})
  call goda_fi#ui#do_action('hello')
  call s:check('custom ui', get(g:, 'ui_hello', []), ['a.txt'])
  if executable('rg')
    call cursor(4, 1)
    call goda_fi#ui#do_action('itemAction', {'name': 'grep'})
    call s:wait('custom grep', {-> s:shown('custom')
          \ && join(s:lines('custom')) =~# 'two words'})
    call s:check('custom grep', join(s:lines('custom')) =~# 'c\.txt:2:5: two words', 1)
    call goda_fi#pop('custom')
    call s:check('custom pop', s:wait_lines('custom', ['a.txt', 'b.go', 'msg.log', 'sub/']),
          \ ['a.txt', 'b.go', 'msg.log', 'sub/'])
  endif
  call goda_fi#ui#do_action('quit')

  " The binary stopped (or exited): the next start starts it again.
  call goda#stop('fi')
  call s:check('stopped', goda#is_running('fi'), 0)
  call s:start({'name': 'again', 'sources': ['file']})
  call s:check('restarted', s:wait_lines('again', ['a.txt', 'b.go', 'msg.log', 'sub/']),
        \ ['a.txt', 'b.go', 'msg.log', 'sub/'])
  call goda_fi#ui#do_action('quit')

  " Errors are told.
  call goda_fi#start({'name': 'bad', 'sources': ['nosuch']})
  call s:check('unknown source', s:shown('bad'), v:false)
endfunction

try
  call s:run()
catch
  call add(s:errors, v:throwpoint . ': ' . v:exception)
endtry
call writefile(s:errors + (empty(s:errors) ? ['ok'] : []) + goda#log(), s:result)
call goda#stop()
qall!
