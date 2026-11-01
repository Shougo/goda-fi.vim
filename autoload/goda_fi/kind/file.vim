" The Vim side of the kind "file" (ext/kind/file): one function per action,
" on targets [{word, path, bufNr, lineNr, col, isDirectory, ...}].

function! goda_fi#kind#file#print(msg) abort
  echomsg '[goda-fi-kind-file] ' . a:msg
endfunction

" confirm(): with {msg} only, Yes or not; with {choices} and {default}, the
" choice ({default} when it fails).
function! goda_fi#kind#file#confirm(msg, ...) abort
  let [choices, default] = a:0 ? [a:1, get(a:000, 1, 1)] : ["&Yes\n&No\n&Cancel", 2]
  try
    let n = confirm(a:msg, choices, default)
  catch
    let n = a:0 ? default : 0
  endtry
  return a:0 ? n : n == 1
endfunction

" input() in the directory {cwd} (for its completion): "" when cancelled.
function! goda_fi#kind#file#cwd_input(cwd, prompt, text, completion) abort
  echo '' | redraw
  let prev = getcwd()
  try
    if a:cwd !=# '' && isdirectory(a:cwd)
      call chdir(a:cwd)
    endif
    return input(a:prompt, a:text, a:completion)
  catch /^Vim:Interrupt/
  finally
    call chdir(prev)
  endtry
  return ''
endfunction

function! goda_fi#kind#file#getreg(reg) abort
  return getreg(a:reg)
endfunction

" The buffer of the file {path} (-1: none).
function! s:buffer_of(path) abort
  " bufnr() matches parts of names: only a buffer of this very name.
  return bufexists(a:path) ? bufnr(a:path) : -1
endfunction

" Is the buffer of the file {path} modified?
function! goda_fi#kind#file#modified(path) abort
  let bufnr = s:buffer_of(a:path)
  return bufnr > 0 && getbufvar(bufnr, '&modified') ? v:true : v:false
endfunction

" The file {old} was renamed {new}: so is its buffer.
function! goda_fi#kind#file#buffer_rename(old, new) abort
  let bufnr = s:buffer_of(a:old)
  if bufnr < 0 || !bufloaded(bufnr)
    return
  endif
  let hidden = &hidden
  set hidden
  let save = bufnr('%')
  try
    noautocmd silent! execute 'buffer' bufnr
    silent! execute (&l:buftype ==# '' ? 'saveas!' : 'file') fnameescape(a:new)
    if &l:buftype ==# ''
      " The buffer of the old name.
      silent! bdelete! #
    endif
    noautocmd silent! execute 'buffer' save
  finally
    let &hidden = hidden
  endtry
endfunction

" The file {path} was deleted: its buffer too.
function! goda_fi#kind#file#buffer_delete(path) abort
  let bufnr = s:buffer_of(a:path)
  if bufnr < 0
    return
  endif
  let winid = get(win_findbuf(bufnr), 0, -1)
  if winid > 0
    let save = win_getid()
    call win_gotoid(winid)
    noautocmd silent enew
    execute 'silent! bdelete!' bufnr
    call win_gotoid(save)
  else
    execute 'silent! bdelete!' bufnr
  endif
endfunction

function! s:path(t) abort
  return get(a:t, 'path', '') !=# '' ? a:t.path
        \ : get(a:t, 'bufNr', 0) > 0 ? fnamemodify(bufname(a:t.bufNr), ':p') : a:t.word
endfunction

function! s:bufnr(t) abort
  if get(a:t, 'bufNr', 0) > 0 && bufexists(a:t.bufNr)
    return a:t.bufNr
  endif
  " bufnr() matches parts of names: only a buffer of this very name.
  let path = get(a:t, 'path', '')
  return path !=# '' && bufexists(path) ? bufnr(path) : -1
endfunction

" :{command} {path}, creating its directory when asked to.
function! s:execute_path(command, path) abort
  let path = fnamemodify(a:path, ':p')
  let dir = fnamemodify(path, ':h')
  if !isdirectory(dir) && dir !~# '^\a\+://'
        \ && confirm(printf('"%s" does not exist. Create?', dir), "&Yes\n&No", 2) == 1
    call mkdir(dir, 'p')
  endif
  try
    execute a:command fnameescape(fnamemodify(path, ':.'))
  catch /^Vim\%((\a\+)\)\=:E325\|^Vim:Interrupt/
    " The swap file: the user answered.
  endtry
endfunction

function! goda_fi#kind#file#open(targets, params, context) abort
  let command = get(a:params, 'command', 'edit')
  normal! m`
  for t in a:targets
    let bufnr = s:bufnr(t)
    if bufnr > 0
      if command !=# 'edit'
        call s:execute_path(command, s:path(t))
      endif
      if !bufloaded(bufnr)
        call bufload(bufnr)
        call setbufvar(bufnr, '&buflisted', 1)
      endif
      execute 'buffer' bufnr
    else
      call s:execute_path(command, s:path(t))
    endif
    if get(t, 'lineNr', 0) > 0
      call cursor(t.lineNr, 1)
      let input = get(a:context, 'input', '')
      if input !=# '' && get(t, 'col', 0) <= 0
        let col = stridx(tolower(getline('.')), tolower(input))
        if col >= 0
          call cursor(0, col + 1)
        endif
      endif
    endif
    if get(t, 'col', 0) > 0
      call cursor(0, t.col)
    endif
    normal! zvzz
  endfor
endfunction

function! goda_fi#kind#file#execute(targets, params, context) abort
  for t in a:targets
    call s:execute_path(get(a:params, 'command', 'edit'), s:path(t))
  endfor
endfunction

function! goda_fi#kind#file#cd(targets, params, context) abort
  for t in a:targets
    let path = s:path(t)
    let dir = isdirectory(path) ? path : fnamemodify(path, ':h')
    if !isdirectory(dir)
      call goda_fi#util#print_error('goda-fi: ' . dir . ' is not found.')
      continue
    endif
    call chdir(dir)
  endfor
endfunction

function! goda_fi#kind#file#yank(targets, params, context) abort
  for t in a:targets
    call setreg('"', s:path(t), 'v')
    if v:register !=# '"'
      call setreg(v:register, s:path(t), 'v')
    endif
  endfor
endfunction

function! s:paste(targets, key) abort
  if !&l:modifiable
    return
  endif
  let save = getreginfo('"')
  try
    for t in a:targets
      call setreg('"', s:path(t), 'v')
      execute 'normal! ""' . a:key
    endfor
  finally
    call setreg('"', save)
  endtry
  normal! zv
endfunction

function! goda_fi#kind#file#append(targets, params, context) abort
  call s:paste(a:targets, 'p')
endfunction

function! goda_fi#kind#file#insert(targets, params, context) abort
  call s:paste(a:targets, 'P')
endfunction

function! goda_fi#kind#file#feedkeys(targets, params, context) abort
  for t in a:targets
    call feedkeys(s:path(t), 'n')
  endfor
endfunction

function! s:qflist(targets) abort
  let list = []
  for t in a:targets
    let e = {'text': get(t, 'text', '') !=# '' ? t.text : t.word}
    if get(t, 'lineNr', 0) > 0
      let e.lnum = t.lineNr
    endif
    if get(t, 'col', 0) > 0
      let e.col = t.col
    endif
    if get(t, 'bufNr', 0) > 0
      let e.bufnr = t.bufNr
    endif
    if get(t, 'path', '') !=# ''
      let e.filename = t.path
    endif
    call add(list, e)
  endfor
  return list
endfunction

function! goda_fi#kind#file#quickfix(targets, params, context) abort
  call setqflist(s:qflist(a:targets), ' ')
  copen
endfunction

function! goda_fi#kind#file#loclist(targets, params, context) abort
  call setloclist(0, s:qflist(a:targets), ' ')
  lopen
endfunction
