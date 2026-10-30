" The Vim side of the kind "file" (ext/kind/file): one function per action,
" on targets [{word, path, bufNr, lineNr, col, isDirectory, ...}].

function! goda_fi#kind#file#print(msg) abort
  echomsg '[goda-fi-kind-file] ' . a:msg
endfunction

function! goda_fi#kind#file#confirm(msg) abort
  try
    return confirm(a:msg, "&Yes\n&No\n&Cancel", 2) == 1
  catch
    return v:false
  endtry
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
