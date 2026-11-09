" The Vim side of the kind "url" (ext/kind/url).

function! goda_fi#kind#url#open(urls, command) abort
  for url in a:urls
    execute 'silent' a:command fnameescape(url)
  endfor
endfunction

function! goda_fi#kind#url#yank(urls, register) abort
  let register = a:register ==# '' ? v:register : a:register
  call setreg(register, a:urls, 'V')
endfunction

" The response of fetch, in a new scratch buffer.
function! goda_fi#kind#url#show(lines) abort
  new
  setlocal buftype=nofile bufhidden=wipe noswapfile
  silent! file goda-fi-kind-url-fetch
  call setline(1, a:lines)
  setlocal nomodified
endfunction
