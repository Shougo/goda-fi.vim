" goda-fi.vim - a fuzzy finder for Vim/Neovim on goda.vim, the successor of
" ddu.vim.
"
" The Go side (the user's own build of a configuration, see the README)
" gathers, filters and acts; this file starts it and passes on what is asked.
" The UIs have their own Vim script (autoload/goda_fi/ui/*.vim).

" The version of this Vim script that the binary goes with (ScriptVersion of
" its Go side): goda.vim refuses a binary of another one, and says to build
" it again. Bumped with it when they no longer work together.
let s:version = #{version: 1, module: 'github.com/Shougo/goda-fi.vim'}

" Patches of the options made at run time, as ddu#custom#patch_*(). They live
" here, not in the binary: they survive a restart of it, and they can be made
" before it is started. Every start carries them.
let s:global_patch = {}
let s:local_patch = {}
" The actions of goda_fi#custom#action(): {type: {name: {action: Funcref}}}.
" Every start carries their names; the binary calls them back.
let s:custom_actions = {'ui': {}, 'source': {}, 'kind': {}}

" ---------------------------------------------------------------------------
" Setup

" The binary is machine specific, so it is not built into the configuration
" directory (which belongs in your dotfiles) but into a cache directory.
function! s:cache_dir() abort
  if has('nvim')
    let base = stdpath('cache')
  elseif !empty($XDG_CACHE_HOME)
    let base = $XDG_CACHE_HOME . '/vim'
  elseif has('win32') && !empty($LOCALAPPDATA)
    let base = $LOCALAPPDATA . '/vim'
  else
    let base = expand('~/.cache/vim')
  endif
  return base . '/goda-fi'
endfunction

function! s:default_bin() abort
  if exists('g:goda_fi#binary')
    return expand(g:goda_fi#binary)
  endif
  return s:cache_dir() . '/goda-fi' . (has('win32') ? '.exe' : '')
endfunction

function! s:config_dir() abort
  if exists('g:goda_fi#config_dir')
    return expand(g:goda_fi#config_dir)
  endif
  return (has('nvim') ? stdpath('config') : expand('~/.vim')) . '/goda-fi'
endfunction

" Build the user's configuration (a Go module in g:goda_fi#config_dir) with
" goda#build(): with the plugins installed (g:goda#workspace). The running
" binary is stopped, and the new one starts with the next request. When the
" build fails, the previous binary keeps running.
function! goda_fi#build() abort
  call goda#build(#{name: 'fi', dir: s:config_dir(), bin: s:default_bin()})
  call goda#register('fi', function('s:start'), s:version)
endfunction

" Start the binary: goda.vim calls it when a request needs the plugin "fi"
" and no binary serves it (not started yet, stopped, or exited).
function! s:start() abort
  let bin = s:default_bin()
  if !executable(bin)
    throw 'goda-fi: ' . bin . ' does not exist yet; build it with: call goda_fi#build()'
  endif
  let plugins = goda#start(bin)
  if index(plugins, 'fi') < 0
    if !empty(plugins)
      call goda#stop(plugins[0])
    endif
    throw 'goda-fi: ' . bin . ' does not contain the "fi" plugin'
  endif
  " A binary built from an invalid configuration starts anyway and says why.
  let status = goda#request('fi', 'status')
  if type(status) == v:t_string && status !=# ''
    call goda#stop('fi')
    throw 'goda-fi: fix the configuration in ' . s:config_dir()
          \ . ' and run goda_fi#build(): ' . status
  endif
endfunction

" (Again after a build: a start that failed is tried again.)
call goda#register('fi', function('s:start'), s:version)

" ---------------------------------------------------------------------------
" API (that of ddu.vim)

" Start a session with these options (merged over the configuration and the
" patches); a session of the same name is replaced, or resumed with
" {'resume': v:true}.
function! goda_fi#start(options = {}) abort
  try
    let context = {
          \ 'bufName': bufname('%'),
          \ 'bufNr': bufnr('%'),
          \ 'winId': win_getid(),
          \ 'cwd': getcwd(),
          \ 'mode': mode(),
          \ }
    call goda#request('fi', 'start', a:options, context, s:patches())
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
  call goda#request('fi', 'itemAction', a:name, a:action,
        \ map(copy(a:items), 'get(v:val, "__id", 0)'), a:params)
endfunction

function! goda_fi#get_context(name) abort
  return goda#is_running('fi') ? goda#request('fi', 'getContext', a:name) : {}
endfunction

" The names of the sessions.
function! goda_fi#sessions() abort
  return goda#is_running('fi') ? goda#request('fi', 'sessions') : []
endfunction

" ---------------------------------------------------------------------------
" Patches: goda_fi#custom#*() keep them here.

function! goda_fi#_patch_global(patch) abort
  let s:global_patch = goda_fi#util#merge(s:global_patch, a:patch)
endfunction

function! goda_fi#_patch_local(name, patch) abort
  let s:local_patch[a:name] = goda_fi#util#merge(get(s:local_patch, a:name, {}), a:patch)
endfunction

function! goda_fi#_get_global() abort
  return deepcopy(s:global_patch)
endfunction

function! goda_fi#_get_local() abort
  return deepcopy(s:local_patch)
endfunction

function! s:patches() abort
  let actions = {}
  for [type, names] in items(s:custom_actions)
    let actions[type] = map(copy(names), 'keys(v:val)')
  endfor
  return {'global': s:global_patch, 'local': s:local_patch, 'actions': actions}
endfunction

" ---------------------------------------------------------------------------
" Actions as Vim functions: goda_fi#custom#action() keeps them here.

function! goda_fi#_custom_action(type, names, action, func) abort
  if !has_key(s:custom_actions, a:type)
    throw 'goda-fi: invalid custom action type: ' . a:type
  endif
  let l:Func = type(a:func) == v:t_func ? a:func : function(a:func)
  for name in split(a:names, '\s*,\s*')
    let s:custom_actions[a:type][name] = extend(
          \ get(s:custom_actions[a:type], name, {}), {a:action : l:Func})
  endfor
endfunction

" The binary runs an action of goda_fi#custom#action(): the action flags (a
" Number, 0 for anything else, as ddu.vim).
function! goda_fi#_call_action(type, name, action, args) abort
  let l:Func = s:custom_actions[a:type][a:name][a:action]
  let flags = call(l:Func, [a:args])
  return type(flags) == v:t_number ? flags : 0
endfunction
