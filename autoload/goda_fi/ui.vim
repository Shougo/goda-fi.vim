" The UI API, as ddu#ui#*(). The session is that of the current buffer (the
" UI's), unless {name} is given.

" The UI of each session shown: {name: ui}. The UIs register here.
let s:uis = {}

function! goda_fi#ui#_register(name, ui) abort
  let s:uis[a:name] = a:ui
  let s:last = a:name
endfunction

" The session: {name}, that of the current buffer, or the last one shown
" (from another buffer: checkItems in autocommands, say).
function! s:name(name) abort
  if a:name !=# ''
    return a:name
  endif
  let name = get(b:, 'goda_fi_name', '')
  if name ==# '' && get(s:, 'last', '') !=# '' && goda_fi#ui#visible(s:last)
    let name = s:last
  endif
  return name
endfunction

" Run the UI action {action} ("itemAction", "toggleSelectItem", ...).
function! goda_fi#ui#do_action(action, params = {}, name = '') abort
  let name = s:name(a:name)
  if name ==# ''
    " No session shown.
    return
  endif
  let ui = get(s:uis, name, '')
  try
    if ui !=# '' && goda_fi#ui#{ui}#_local_action(name, a:action, a:params)
      return
    endif
    let pos = ui !=# '' ? goda_fi#ui#{ui}#_pos(name) : {}
    call goda_fi#util#request_wait('fi', 'uiAction', name, a:action, a:params, pos)
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

" The action chosen (the kind action): once its action is done, the session
" of the actions goes away and the action runs on the items.
function! goda_fi#ui#_do_chosen(name, action, ids) abort
  call timer_start(0, {-> s:do_chosen(a:name, a:action, a:ids)})
endfunction

function! s:do_chosen(name, action, ids) abort
  try
    call goda_fi#pop(a:name)
    call goda_fi#item_action(a:name, a:action, map(copy(a:ids), {_, id -> {'__id': id}}))
  catch
    call goda_fi#util#print_error(v:exception)
  endtry
endfunction

" Save the command line maps of {keys}, as ddu#ui#save_cmaps(): to map them
" in GodaFi:uiOpenFilterWindow, and restore them with
" goda_fi#ui#restore_cmaps() in GodaFi:uiCloseFilterWindow.
function! goda_fi#ui#save_cmaps(keys) abort
  let s:save_maps = {}
  for key in a:keys
    let s:save_maps[key] = maparg(key, 'c', v:false, v:true)
  endfor
endfunction

" Restore the command line maps saved by goda_fi#ui#save_cmaps(): those
" made since are removed.
function! goda_fi#ui#restore_cmaps() abort
  if !exists('s:save_maps')
    return
  endif
  for [key, map] in items(s:save_maps)
    let current = maparg(key, 'c', v:false, v:true)
    if !empty(current)
      execute 'cunmap' (current.buffer ? '<buffer>' : '') key
    endif
    if !empty(map)
      call mapset('c', 0, map)
    endif
  endfor
  let s:save_maps = {}
endfunction
