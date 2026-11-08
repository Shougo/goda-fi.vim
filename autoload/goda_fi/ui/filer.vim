" The filer UI (ui/filer): a UI of ff (ff.NewUI) with a buffer of its own
" (autoload/goda_fi/ui/ff.vim does it all).

function! goda_fi#ui#filer#_pos(name) abort
  return goda_fi#ui#ff#_pos(a:name)
endfunction

function! goda_fi#ui#filer#_visible(name) abort
  return goda_fi#ui#ff#_visible(a:name)
endfunction

function! goda_fi#ui#filer#_local_action(name, action, params) abort
  return goda_fi#ui#ff#_local_action(a:name, a:action, a:params)
endfunction
