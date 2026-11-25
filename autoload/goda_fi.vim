" goda-fi.vim - a fuzzy finder for Vim/Neovim on goda.vim, the successor of
" ddu.vim.
"
" The Go side (the user's own build of a configuration, see the README)
" gathers, filters and acts; this file starts it and passes on what is asked.
" The UIs have their own Vim script (autoload/goda_fi/ui/*.vim).

" The version of this Vim script that the binary goes with (ScriptVersion of
" its Go side): goda.vim refuses a binary of another one, and says to build
" it again. Bumped with it when they no longer work together.
let s:version = #{version: 3, module: 'github.com/Shougo/goda-fi.vim'}

" ---------------------------------------------------------------------------
" Setup

let s:goda = goda#configured#new('fi', s:version)

" Build the user's configuration (a Go module in g:goda_fi#config_dir) with
" goda#build(): with the plugins installed (g:goda#workspace). The running
" binary is stopped, and the new one starts with the next request. When the
" build fails, the previous binary keeps running.
function! goda_fi#build() abort
  call s:goda.build()
  call s:goda.register()
endfunction

call s:goda.register()

" ---------------------------------------------------------------------------
" API (that of ddu.vim)

" Start a session with these options (merged over the configuration); a
" session of the same name is replaced, or resumed with {'resume': v:true}.
function! goda_fi#start(options = {}) abort
  try
    let context = {
          \ 'bufName': bufname('%'),
          \ 'bufNr': bufnr('%'),
          \ 'winId': win_getid(),
          \ 'cwd': getcwd(),
          \ 'mode': mode(),
          \ }
    call goda#request('fi', 'start', a:options, context)
  catch
    call goda_fi#util#print_error(v:exception)
  endtry
endfunction

" Filter the session with {'input': ...}, or gather again with
" {'refresh': v:true}.
function! goda_fi#redraw(name, options = {}) abort
  if goda#is_running('fi')
    call goda#notify('fi', 'redraw', a:name, a:options)
  endif
endfunction

" End the session started with {'push': v:true} and show the one it replaced.
function! goda_fi#pop(name = get(b:, 'goda_fi_name', 'default')) abort
  if goda#is_running('fi')
    call goda#request('fi', 'pop', a:name)
  endif
endfunction

" Run the item action on items (from goda_fi#ui#get_item(), say).
function! goda_fi#item_action(name, action, items, params = {}) abort
  call goda_fi#util#request_wait('fi', 'itemAction', a:name, a:action,
        \ map(copy(a:items), 'get(v:val, "__id", 0)'), a:params)
endfunction

function! goda_fi#get_context(name) abort
  return goda#is_running('fi') ? goda#request('fi', 'getContext', a:name) : {}
endfunction

" The names of the sessions.
function! goda_fi#sessions() abort
  return goda#is_running('fi') ? goda#request('fi', 'sessions') : []
endfunction

" The options of the session {name} (that of the current buffer), as set
" ({} without the session), as ddu#custom#get_current().
function! goda_fi#get_current(name = '') abort
  let name = a:name !=# '' ? a:name : get(b:, 'goda_fi_name', '')
  if name ==# '' || !goda#is_running('fi')
    return {}
  endif
  return goda#request('fi', 'getCurrent', name)
endfunction
