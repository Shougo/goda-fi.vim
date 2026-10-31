" goda-fi.vim - a fuzzy finder for Vim/Neovim on goda.vim, the successor of
" ddu.vim.
"
" The Go side (the user's own build of a configuration, see the README)
" gathers, filters and acts; this file starts it and passes on what is asked.
" The UIs have their own Vim script (autoload/goda_fi/ui/*.vim).

" Patches of the options made at run time, as ddu#custom#patch_*(). They live
" here, not in the binary: they survive a restart of it, and they can be made
" before it is started. Every start carries them.
let s:global_patch = {}
let s:local_patch = {}

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

" Build the user's configuration (a Go module in g:goda_fi#config_dir), after
" "go mod tidy". The running binary is stopped, and the new one starts with
" the next request. When the build fails, the previous binary keeps running.
function! goda_fi#build() abort
  let dir = s:config_dir()
  if !isdirectory(dir)
    throw 'goda-fi: no config directory: ' . dir
  endif
  let bin = s:default_bin()
  call mkdir(fnamemodify(bin, ':h'), 'p')
  let tidy = system('go -C ' . shellescape(dir) . ' mod tidy')
  let tidy_failed = v:shell_error
  let tmp = bin . '.new' . (has('win32') ? '.exe' : '')
  let out = system('go -C ' . shellescape(dir) . ' build -o ' . shellescape(tmp) . ' .')
  if v:shell_error
    throw 'goda-fi: go build failed:' . "\n" . out
          \ . (tidy_failed ? "\n" . 'go mod tidy failed too:' . "\n" . tidy : '')
  endif
  if s:own_binary()
    call goda#stop('fi')
  endif
  if rename(tmp, bin) != 0
    throw 'goda-fi: cannot replace ' . bin
  endif
  call goda#register('fi', function('s:start'))
  echomsg 'goda-fi: built ' . bin
endfunction

" Is the plugin served by the binary of goda-fi (not by another plugin's
" binary that contains it, see the README)?
function! s:own_binary() abort
  let plugins = goda#plugins()
  return has_key(plugins, 'fi')
        \ && plugins.fi ==# fnamemodify(s:default_bin(), ':p')
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
call goda#register('fi', function('s:start'))

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
  return {'global': s:global_patch, 'local': s:local_patch}
endfunction
