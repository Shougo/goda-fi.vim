" The files of the source "fileold" (ext/source/fileold).
function! goda_fi#source#fileold#_list() abort
  " Those of the other Vims too (not without a viminfo or ShaDa file).
  try
    if has('nvim')
      if &shada !=# '' && &shadafile !=# 'NONE'
        wshada
        rshada!
      endif
    elseif &viminfo !=# '' && &viminfofile !=# 'NONE'
      wviminfo
      rviminfo!
    endif
  catch
    " Unreadable: v:oldfiles as it is.
  endtry
  let list = []
  for path in v:oldfiles
    let path = path =~# '^\~' ? fnamemodify(path, ':p') : path
    if filereadable(path) && fnamemodify(path, ':t') !=# 'COMMIT_EDITMSG'
      call add(list, path)
    endif
  endfor
  return list
endfunction
