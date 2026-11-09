" The lines of the source "line" (ext/source/line): of the buffer bufnr, or
" those shown in the window winid. The first size lines from the cursor line
" come with the range; goda_fi#source#line#_get() gives the others.
function! goda_fi#source#line#_lines(winid, bufnr, window, size) abort
  let valid = win_id2win(a:winid) > 0
  call bufload(a:bufnr)
  if a:window && valid
    let begin = line('w0', a:winid)
    let end = line('w$', a:winid)
  else
    let begin = 1
    let end = getbufinfo(a:bufnr)[0].linecount
  endif
  let cursor = valid && winbufnr(a:winid) == a:bufnr ? line('.', a:winid) : 1
  let cursor = min([max([cursor, begin]), end])
  return {'bufnr': a:bufnr, 'begin': begin, 'end': end, 'cursor': cursor,
        \ 'lines': getbufline(a:bufnr, cursor, min([end, cursor + a:size - 1]))}
endfunction

function! goda_fi#source#line#_get(bufnr, begin, end) abort
  return getbufline(a:bufnr, a:begin, a:end)
endfunction
