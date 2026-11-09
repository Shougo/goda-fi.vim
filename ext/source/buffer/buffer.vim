" The buffers of the source "buffer" (ext/source/buffer), as seen from the
" window the session was started in: the alternate one first.
function! goda_fi#source#buffer#_list(winid, bufnr) abort
  let alt = -1
  if win_id2win(a:winid) > 0
    let alt = win_execute(a:winid, 'echon bufnr("#")')->str2nr()
  endif
  let list = []
  for info in getbufinfo({'buflisted': 1})
    let path = fnamemodify(info.name, ':p')
    if info.name ==# '' || !filereadable(path)
      continue
    endif
    let b = {
          \ 'bufnr': info.bufnr,
          \ 'path': path,
          \ 'current': info.bufnr == a:bufnr ? v:true : v:false,
          \ 'alternate': info.bufnr == alt ? v:true : v:false,
          \ 'modified': info.changed ? v:true : v:false,
          \ }
    if b.alternate
      call insert(list, b)
    else
      call add(list, b)
    endif
  endfor
  return list
endfunction
