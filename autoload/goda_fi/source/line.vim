" The lines of the source "line" (ext/source/line): of the buffer bufnr, or
" those shown in the window winid.
function! goda_fi#source#line#_lines(winid, bufnr, window) abort
  let valid = win_id2win(a:winid) > 0
  let cursor = valid && winbufnr(a:winid) == a:bufnr ? line('.', a:winid) : 1
  if a:window && valid
    let begin = line('w0', a:winid)
    let end = line('w$', a:winid)
  else
    let begin = 1
    let end = '$'
  endif
  call bufload(a:bufnr)
  return {'bufnr': a:bufnr, 'begin': begin, 'cursor': cursor,
        \ 'lines': getbufline(a:bufnr, begin, end)}
endfunction
