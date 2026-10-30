" The Vim side of the kind "word" (ext/kind/word): items [{text, regType}].

function! s:paste(items, mode, key) abort
  if !&l:modifiable
    return
  endif
  let save = getreginfo('"')
  try
    for item in a:items
      if item.text ==# ''
        continue
      endif
      call setreg('"', item.text, get(item, 'regType', '') ==# '' ? 'v' : item.regType)
      execute 'normal! ""' . a:key
      if a:mode ==# 'i'
        call cursor(0, col('.') + strlen(item.text))
      endif
    endfor
  finally
    call setreg('"', save)
  endtry
  normal! zv
endfunction

function! goda_fi#kind#word#append(items, mode) abort
  call s:paste(a:items, a:mode, 'p')
endfunction

function! goda_fi#kind#word#insert(items, mode) abort
  call s:paste(a:items, a:mode, 'P')
endfunction

function! goda_fi#kind#word#feedkeys(items, mode) abort
  for item in a:items
    call feedkeys(item.text, 'n')
  endfor
endfunction

function! goda_fi#kind#word#yank(items, mode) abort
  for item in a:items
    call setreg('"', item.text, 'v')
    if v:register !=# '"'
      call setreg(v:register, item.text, 'v')
    endif
  endfor
endfunction
