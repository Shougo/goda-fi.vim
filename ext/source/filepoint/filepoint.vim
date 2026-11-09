" The source "filepoint" (ext/source/filepoint): the cursor of the window the
" session was started in.
function! goda_fi#source#filepoint#_point(winid) abort
  if win_id2win(a:winid) <= 0
    return {}
  endif
  let pos = getcurpos(a:winid)
  let bufnr = winbufnr(a:winid)
  let line = get(getbufline(bufnr, pos[1]), 0, '')
  let terminal = getbufvar(bufnr, '&buftype') ==# 'terminal' ? v:true : v:false
  let point = {'line': line, 'col': pos[2], 'terminal': terminal, 'prevLine': '',
        \ 'cwd': getcwd(a:winid)}
  if terminal
    " Lines wrapped by the terminal.
    if strlen(line) >= winwidth(a:winid)
      let point.line .= get(getbufline(bufnr, pos[1] + 1), 0, '')
    endif
    let point.prevLine = get(getbufline(bufnr, pos[1] - 1), 0, '')
  endif
  return point
endfunction

" findfile() from that window (its 'path', its file).
function! goda_fi#source#filepoint#_find(winid, path) abort
  let found = ''
  call win_execute(a:winid, 'let found = findfile(a:path, ".**5")')
  return found
endfunction
