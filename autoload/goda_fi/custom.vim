" Patches of the options, as ddu#custom#*(): over those of the configuration
" (main.go), under those given to goda_fi#start().

" goda_fi#custom#patch_global({options}) or ({key}, {value})
function! goda_fi#custom#patch_global(dict_or_key, ...) abort
  call goda_fi#_patch_global(s:patch(a:dict_or_key, a:000))
endfunction

" goda_fi#custom#patch_local({name}, {options}) or ({name}, {key}, {value})
function! goda_fi#custom#patch_local(name, dict_or_key, ...) abort
  call goda_fi#_patch_local(a:name, s:patch(a:dict_or_key, a:000))
endfunction

" goda_fi#custom#action({type}, {name}, {action}, {func}): the action
" {action} of the UI, source or kind ({type}) {name} ("a,b" for several) is
" {func}, a Vim function of {context, options, actionParams, items} that
" returns the action flags.
function! goda_fi#custom#action(type, name, action, func) abort
  call goda_fi#_custom_action(a:type, a:name, a:action, a:func)
endfunction

function! goda_fi#custom#get_global() abort
  return goda_fi#_get_global()
endfunction

function! goda_fi#custom#get_local() abort
  return goda_fi#_get_local()
endfunction

function! s:patch(dict_or_key, rest) abort
  if type(a:dict_or_key) == v:t_dict
    return a:dict_or_key
  endif
  return {a:dict_or_key : get(a:rest, 0, v:null)}
endfunction
