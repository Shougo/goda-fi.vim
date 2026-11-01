" The rename of several files of the kind "file" (its action rename), as
" ddu-kind-file's exrename: a buffer of their names; written, the files are
" renamed as the lines say, and the session gathers again when it is left.

let s:PREFIX = has('win32') ? '[exrename]' : '*exrename*'

" {paths}: the files; {options}: {cwd, name} (the directory the names are
" relative to, the session).
function! goda_fi#kind#file#exrename#create_buffer(paths, options = {}) abort
  let options = extend({'cwd': getcwd(), 'name': ''}, a:options)
  if options.cwd !~# '[/\\]$'
    " The directory ends with a separator.
    let options.cwd .= '/'
  endif
  let winid = win_getid()

  let bufnr = bufadd(s:PREFIX . (options.name ==# '' ? '' : ' - ' . options.name))
  call bufload(bufnr)
  execute 'vertical sbuffer' bufnr

  setlocal buftype=acwrite
  setlocal noswapfile
  setfiletype goda_fi_exrename

  syntax match godaFiExrenameModified '^.*$'
  highlight default link godaFiExrenameModified Todo
  highlight default link godaFiExrenameOriginal Normal

  let b:exrename = options
  let b:exrename.prev_winid = winid

  nnoremap <buffer><silent> q <Cmd>call <SID>exit(bufnr('%'))<CR>
  augroup goda-fi-exrename
    autocmd! * <buffer>
    autocmd BufHidden <buffer> call s:exit(expand('<abuf>'))
    autocmd BufWriteCmd <buffer> call s:do_rename()
    autocmd CursorMoved,CursorMovedI <buffer> call s:check_lines()
  augroup END

  silent % delete _
  silent! syntax clear godaFiExrenameOriginal

  let unique = {}
  let b:exrename.paths = []
  let b:exrename.filenames = []
  let cnt = 1
  for path in a:paths
    if !s:is_absolute(path)
      let path = b:exrename.cwd . path
    endif
    if !filewritable(path) && !isdirectory(path)
      redraw
      call goda_fi#util#print_error(path . ' does not exist. Skip.')
      continue
    endif
    if has_key(unique, path)
      redraw
      call goda_fi#util#print_error(path . ' is duplicated. Skip.')
      continue
    endif

    let filename = path
    if stridx(filename, b:exrename.cwd) == 0
      let filename = filename[len(b:exrename.cwd) :]
    endif
    " A directory ends with a separator.
    if isdirectory(path)
      let filename .= '/'
    endif
    let pattern = escape(escape(filename, '~"\.^$[]*'), '/')
    execute 'syntax match godaFiExrenameOriginal'
          \ '/' . printf('^\%%%dl%s$', cnt, pattern) . '/'

    let unique[path] = 1
    call add(b:exrename.paths, path)
    call add(b:exrename.filenames, filename)
    let cnt += 1
  endfor

  let [undolevels, &l:undolevels] = [&l:undolevels, -1]
  try
    call setline(1, b:exrename.filenames)
  finally
    let &l:undolevels = undolevels
  endtry
  setlocal nomodified

  " Back to the UI, as ddu-kind-file's.
  call win_gotoid(winid)
endfunction

function! s:is_absolute(path) abort
  return a:path =~# '^\%(\a\a\+:\)\|^\%(\a:\|/\)'
endfunction

function! s:do_rename() abort
  if line('$') != len(b:exrename.filenames)
    call goda_fi#util#print_error('Invalid rename buffer!')
    return
  endif

  let max = line('$')
  for linenr in range(1, max)
    let filename = b:exrename.filenames[linenr - 1]
    let line = getline(linenr)
    redraw
    echo printf('(%' . len(max) . 'd/%d): %s -> %s', linenr, max, filename, line)
    if filename ==# line
      continue
    endif

    let old = b:exrename.paths[linenr - 1]
    let new = expand(line)
    if !s:is_absolute(new)
      let new = b:exrename.cwd . new
    endif
    if filereadable(new) || isdirectory(new)
      redraw
      call goda_fi#util#print_error(new . ' already exists. Skip.')
      continue
    endif
    call mkdir(fnamemodify(new, ':h'), 'p')
    if rename(old, new)
      redraw
      call goda_fi#util#print_error(new . ' is rename error. Skip.')
      continue
    endif
    call goda_fi#kind#file#buffer_rename(old, new)

    let b:exrename.filenames[linenr - 1] = line
    let b:exrename.paths[linenr - 1] = new
  endfor

  redraw
  echo 'Rename done!'
  setlocal nomodified
endfunction

function! s:exit(bufnr) abort
  let bufnr = str2nr(a:bufnr)
  if !bufexists(bufnr)
    return
  endif
  let exrename = getbufvar(bufnr, 'exrename', {})
  if empty(exrename)
    return
  endif
  call setbufvar(bufnr, 'exrename', {})
  if winnr('$') > 1 && bufwinid(bufnr) > 0
    call win_execute(bufwinid(bufnr), 'close')
  endif
  " Not while the buffer is hidden.
  call timer_start(0, {-> execute('silent! bwipeout! ' . bufnr)})
  call win_gotoid(exrename.prev_winid)
  if exrename.name !=# ''
    call goda_fi#redraw(exrename.name, {'refresh': v:true})
  endif
endfunction

function! s:check_lines() abort
  if !exists('b:exrename') || empty(b:exrename)
    return
  endif
  if line('$') != len(b:exrename.filenames)
    call goda_fi#util#print_error('Invalid rename buffer!')
  endif
endfunction
