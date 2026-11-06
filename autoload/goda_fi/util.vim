function! goda_fi#util#print_error(msg) abort
  " What a request failed with: "goda: goda-fi: ..." in Vim, "Vim(...):Invoking
  " ...: goda-fi: ..." in Neovim.
  let msg = substitute(a:msg, '^.\{-}\zegoda-fi: ', '', '')
  let msg = substitute(msg, '^goda: ', '', '')
  echohl ErrorMsg
  for line in split(msg, "\n")
    echomsg line
  endfor
  echohl None
endfunction

" base with patch over it: dictionaries are merged, recursively; the rest is
" replaced. Neither is modified.
function! goda_fi#util#merge(base, patch) abort
  let out = copy(a:base)
  for [key, value] in items(a:patch)
    if type(value) == v:t_dict && type(get(out, key, v:null)) == v:t_dict
      let out[key] = goda_fi#util#merge(out[key], value)
    else
      let out[key] = deepcopy(value)
    endif
  endfor
  return out
endfunction

" doautocmd User {name}, if there is one: without one, Vim reports "No
" matching autocommands" and, in insert mode, misses TextChangedI.
function! goda_fi#util#doautocmd(name) abort
  if exists('#User#' . a:name)
    execute 'doautocmd <nomodeline> User' a:name
  endif
endfunction

" goda#request() without g:goda#request_timeout: for the actions, which may
" wait for the user (input(), confirm()), as long as they take. (An older
" goda.vim has no goda#request_wait(): goda#request() then.)
function! goda_fi#util#request_wait(...) abort
  return call(exists('*goda#request_wait') ? 'goda#request_wait' : 'goda#request', a:000)
endfunction
