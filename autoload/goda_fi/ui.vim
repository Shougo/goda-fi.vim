" The UI API, as ddu#ui#*(). The session is that of the current buffer (the
" UI's), unless {name} is given.

" The UI of each session shown: {name: ui}. The UIs register here.
let s:uis = {}

function! goda_fi#ui#_register(name, ui) abort
  let s:uis[a:name] = a:ui
endfunction

function! s:name(name) abort
  return a:name !=# '' ? a:name : get(b:, 'goda_fi_name', '')
endfunction

" Run the UI action {action} ("itemAction", "toggleSelectItem", ...).
function! goda_fi#ui#do_action(action, params = {}, name = '') abort
  let name = s:name(a:name)
  if name ==# ''
    call goda_fi#util#print_error('goda-fi: not in a goda-fi buffer')
    return
  endif
  let ui = get(s:uis, name, '')
  try
    if ui !=# '' && goda_fi#ui#{ui}#_local_action(name, a:action, a:params)
      return
    endif
    let pos = ui !=# '' ? goda_fi#ui#{ui}#_pos(name) : {}
    call goda#request('fi', 'uiAction', name, a:action, a:params, pos)
  catch
    call goda_fi#util#print_error(v:exception)
  endtry
endfunction

" Run the UI actions, in order: each [action, params] or action.
function! goda_fi#ui#multi_actions(actions, name = '') abort
  for action in a:actions
    let [act, params] = type(action) == v:t_list
          \ ? [action[0], get(action, 1, {})] : [action, {}]
    call goda_fi#ui#do_action(act, params, a:name)
  endfor
endfunction

" The item under the cursor ({} if none).
function! goda_fi#ui#get_item(name = '') abort
  let name = s:name(a:name)
  let ui = get(s:uis, name, '')
  if ui ==# '' || !goda#is_running('fi')
    return {}
  endif
  let pos = goda_fi#ui#{ui}#_pos(name)
  return get(pos, 'id', 0) == 0 ? {} : goda#request('fi', 'getItem', name, pos.id)
endfunction

" The items shown.
function! goda_fi#ui#get_items(name = '') abort
  return goda#is_running('fi') ? goda#request('fi', 'getItems', s:name(a:name)) : []
endfunction

" Is the UI of the session shown?
function! goda_fi#ui#visible(name = '') abort
  let name = s:name(a:name)
  let ui = get(s:uis, name, '')
  return ui !=# '' && goda_fi#ui#{ui}#_visible(name)
endfunction
