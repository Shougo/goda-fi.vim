" The ff UI: a buffer of items in a split window (ui/ff/ff.go makes what it
" shows).
"
" Every redraw comes as one call, goda_fi#ui#ff#_redraw(), with a version:
" one older than what is shown is dropped, and once the window is closed only
" one that opens it again is applied. The cursor stays on its item: the IDs of
" the lines come with them.

let s:ns = has('nvim') ? nvim_create_namespace('goda-fi-ff') : 0
let s:state = {}

function! s:st(name) abort
  if !has_key(s:state, a:name)
    let s:state[a:name] = {
          \ 'version': 0,
          \ 'closed': 1,
          \ 'bufnr': -1,
          \ 'ids': [],
          \ 'input': '',
          \ 'done': 1,
          \ 'prev_winid': 0,
          \ 'prev_bufnr': -1,
          \ 'params': {},
          \ 'preview_winid': -1,
          \ 'preview_bufnr': -1,
          \ 'preview_id': 0,
          \ 'sent': v:null,
          \ 'keep_row': 0,
          \ }
  endif
  return s:state[a:name]
endfunction

function! s:eval(v) abort
  return type(a:v) == v:t_string ? eval(a:v) : a:v
endfunction

" ---------------------------------------------------------------------------
" Drawing

function! goda_fi#ui#ff#_redraw(name, r) abort
  try
    call s:redraw(a:name, a:r)
  finally
    " The next one is sent once this one is done (fi.UIAcker).
    if goda#is_running('fi')
      call goda#notify('fi', 'event', a:name, 'drawn', a:r.version)
    endif
  endtry
endfunction

function! s:redraw(name, r) abort
  let st = s:st(a:name)
  if a:r.version <= st.version || (st.closed && !a:r.open)
    return
  endif
  let st.version = a:r.version
  let st.params = a:r.params
  call goda_fi#ui#_register(a:name, 'ff')

  let bufnr = s:buffer(a:name)
  let winid = bufwinid(bufnr)
  let opened = 0
  if a:r.open
    let st.closed = 0
    if a:r.prevWinid > 0 && a:r.prevWinid != winid
      let st.prev_winid = a:r.prevWinid
    endif
    if winid < 0
      let winid = s:open_window(a:name, bufnr, a:r.params)
      let opened = 1
    else
      call win_gotoid(winid)
    endif
  endif

  let old_row = winid > 0 ? line('.', winid) : 0
  let old_id = get(st.ids, old_row - 1, 0)
  call s:set_lines(bufnr, a:r.lines)
  let st.ids = a:r.ids
  let st.input = a:r.input
  call setbufvar(bufnr, 'goda_fi_version', a:r.version)
  call s:highlight(bufnr, a:r)

  if winid > 0
    let n = len(a:r.lines)
    if a:r.cursor > 0
      let row = a:r.cursor
    elseif a:r.reset && st.keep_row > 0
      " The cursor was moved while the filter was typed, before its items
      " came: it stays where it was moved.
      let row = st.keep_row
    elseif a:r.reset
      let row = a:r.params.reversed ? n : 1
    else
      let i = old_id == 0 ? -1 : index(a:r.ids, old_id)
      let row = i >= 0 ? i + 1 : old_row
    endif
    let row = max([1, min([row, n])])
    call win_execute(winid, 'call cursor(' . row . ', 1)')
    if type(st.sent) == v:t_string && a:r.input ==# st.sent
      " What was typed is shown.
      let st.sent = v:null
      let st.keep_row = 0
    endif
    if a:r.params.reversed && (a:r.reset || opened)
      call win_execute(winid, 'normal! zb')
    endif
    if a:r.params.overwriteStatusline
      call setwinvar(winid, '&statusline', s:escape(a:r.status[0])
            \ . ' %#LineNr#%{line(".")}/%{line("$")}%*' . s:escape(a:r.status[1]))
    endif
  endif

  if opened
    call goda_fi#util#doautocmd('GodaFi:uiReady')
  endif
  if a:r.done && !st.done
    call goda_fi#util#doautocmd('GodaFi:uiDone')
  endif
  let st.done = a:r.done
  call goda_fi#util#doautocmd('GodaFi:uiRedraw')
  " Needed while the filter is typed (and harmless otherwise).
  redraw
endfunction

function! s:escape(s) abort
  return substitute(a:s, '%', '%%', 'g')
endfunction

function! s:buffer(name) abort
  let st = s:st(a:name)
  if st.bufnr > 0 && bufexists(st.bufnr)
    return st.bufnr
  endif
  let bufnr = bufadd('goda-fi-ff-' . a:name)
  " Before loading: the name is not a file.
  call setbufvar(bufnr, '&buftype', 'nofile')
  call bufload(bufnr)
  call setbufvar(bufnr, '&bufhidden', 'hide')
  call setbufvar(bufnr, '&swapfile', 0)
  call setbufvar(bufnr, '&buflisted', 0)
  call setbufvar(bufnr, '&undolevels', -1)
  call setbufvar(bufnr, '&modifiable', 0)
  call setbufvar(bufnr, 'goda_fi_name', a:name)
  call setbufvar(bufnr, 'goda_fi_ui', 'ff')
  execute 'augroup goda-fi-ff-' . bufnr
    autocmd!
    execute 'autocmd BufWinLeave <buffer=' . bufnr . '> call s:on_close(' . string(a:name) . ')'
    execute 'autocmd BufWipeout <buffer=' . bufnr . '> call s:on_wipe(' . string(a:name) . ')'
  augroup END
  let st.bufnr = bufnr
  return bufnr
endfunction

function! s:open_window(name, bufnr, p) abort
  let st = s:st(a:name)
  if a:p.split ==# 'tab'
    tabnew
    let st.prev_bufnr = bufnr('%')
    execute 'silent keepalt buffer' a:bufnr
  elseif a:p.split ==# 'no'
    let st.prev_bufnr = bufnr('%')
    execute 'silent keepalt buffer' a:bufnr
  elseif a:p.split ==# 'vertical'
    execute 'silent' a:p.splitDirection 'vertical split'
    execute 'silent keepalt buffer' a:bufnr
    execute 'vertical resize' s:eval(a:p.winWidth)
    setlocal winfixwidth
  else
    execute 'silent' a:p.splitDirection 'split'
    execute 'silent keepalt buffer' a:bufnr
    execute 'resize' s:eval(a:p.winHeight)
    setlocal winfixheight
  endif
  setlocal nowrap nolist nonumber norelativenumber nospell
  setlocal nofoldenable foldcolumn=0 signcolumn=no cursorline
  if &filetype !=# 'goda-fi-ff'
    " Your mappings: autocmd FileType goda-fi-ff
    setlocal filetype=goda-fi-ff
  endif
  return win_getid()
endfunction

function! s:set_lines(bufnr, lines) abort
  call setbufvar(a:bufnr, '&modifiable', 1)
  try
    if empty(a:lines)
      silent call deletebufline(a:bufnr, 1, '$')
    else
      call setbufline(a:bufnr, 1, a:lines)
      let total = get(get(getbufinfo(a:bufnr), 0, {}), 'linecount', 0)
      if total > len(a:lines)
        silent call deletebufline(a:bufnr, len(a:lines) + 1, '$')
      endif
    endif
  finally
    call setbufvar(a:bufnr, '&modifiable', 0)
    call setbufvar(a:bufnr, '&modified', 0)
  endtry
endfunction

let s:prop_types = {}

function! s:prop_type(group, priority) abort
  let name = 'goda_fi_' . a:group . '_' . a:priority
  if !has_key(s:prop_types, name)
    if empty(prop_type_get(name))
      let opts = {'priority': a:priority, 'override': v:true}
      if a:group !=# ''
        let opts.highlight = a:group
      endif
      call prop_type_add(name, opts)
    endif
    let s:prop_types[name] = 1
  endif
  return name
endfunction

function! s:highlight(bufnr, r) abort
  if has('nvim')
    call nvim_buf_clear_namespace(a:bufnr, s:ns, 0, -1)
  else
    call prop_clear(1, max([1, len(a:r.lines)]), {'bufnr': a:bufnr})
  endif
  for [row, col, length, group, priority] in a:r.highlights
    let text = get(a:r.lines, row - 1, '')
    if !hlexists(group) || col > strlen(text)
      continue
    endif
    let length = length <= 0 || col + length - 1 > strlen(text)
          \ ? strlen(text) - col + 1 : length
    if has('nvim')
      call nvim_buf_set_extmark(a:bufnr, s:ns, row - 1, col - 1, {
            \ 'end_col': col - 1 + length, 'hl_group': group, 'priority': priority})
    else
      call prop_add(row, col, {'length': length, 'bufnr': a:bufnr,
            \ 'type': s:prop_type(group, priority)})
    endif
  endfor
  for info in a:r.info
    if has('nvim')
      call nvim_buf_set_extmark(a:bufnr, s:ns, info.row - 1, 0, {
            \ 'virt_lines': map(copy(info.lines),
            \   {_, l -> l[1] ==# '' ? [[l[0]]] : [[l[0], l[1]]]})})
    else
      for [text, group] in info.lines
        call prop_add(info.row, 0, {'type': s:prop_type(hlexists(group) ? group : '', 50),
              \ 'text': text, 'text_align': 'below', 'bufnr': a:bufnr})
      endfor
    endif
  endfor
endfunction

" ---------------------------------------------------------------------------
" Closing

function! goda_fi#ui#ff#_quit(name, version) abort
  let st = s:st(a:name)
  let st.closed = 1
  call s:close_preview(st)
  let winid = bufwinid(st.bufnr)
  if winid > 0
    let split = get(st.params, 'split', 'horizontal')
    call win_gotoid(winid)
    if split ==# 'no' || winnr('$') == 1 && tabpagenr('$') == 1
      if bufexists(st.prev_bufnr) && st.prev_bufnr != st.bufnr
        execute 'silent keepalt buffer' st.prev_bufnr
      else
        enew
      endif
    else
      silent! close!
    endif
  endif
  if win_id2win(st.prev_winid) > 0
    call win_gotoid(st.prev_winid)
  endif
  call goda_fi#util#doautocmd('GodaFi:uiQuit')
endfunction

" The window was closed (or shows another buffer).
function! s:on_close(name) abort
  let st = s:st(a:name)
  if !st.closed
    let st.closed = 1
    if goda#is_running('fi')
      call goda#notify('fi', 'event', a:name, 'close', st.version)
    endif
  endif
  " Not while the window closes.
  call timer_start(0, {-> s:close_preview(st)})
endfunction

function! s:on_wipe(name) abort
  call s:on_close(a:name)
  let st = s:st(a:name)
  let st.bufnr = -1
  let st.ids = []
endfunction

function! goda_fi#ui#ff#_visible(name) abort
  let st = s:st(a:name)
  return !st.closed && bufwinid(st.bufnr) > 0
endfunction

" ---------------------------------------------------------------------------
" Actions

" Where the cursor is, for an action: {version, cursor, id}.
function! goda_fi#ui#ff#_pos(name) abort
  let st = s:st(a:name)
  let winid = bufwinid(st.bufnr)
  let row = winid > 0 ? line('.', winid) : 0
  return {'version': st.version, 'cursor': row, 'id': get(st.ids, row - 1, 0)}
endfunction

" The actions done here: 1 if it was one.
function! goda_fi#ui#ff#_local_action(name, action, params) abort
  if a:action ==# 'cursorNext'
    call s:move(a:name, get(a:params, 'count', 1), get(a:params, 'loop', v:false))
  elseif a:action ==# 'cursorPrevious'
    call s:move(a:name, -get(a:params, 'count', 1), get(a:params, 'loop', v:false))
  elseif a:action ==# 'openFilterWindow'
    call s:open_filter(a:name, a:params)
  else
    return 0
  endif
  return 1
endfunction

function! s:move(name, count, loop) abort
  let st = s:st(a:name)
  let winid = bufwinid(st.bufnr)
  if winid < 0
    return
  endif
  let last = line('$', winid)
  let row = line('.', winid) + a:count
  if row > last
    let row = a:loop ? 1 : last
  elseif row < 1
    let row = a:loop ? last : 1
  endif
  call win_execute(winid, 'call cursor(' . row . ', 1)')
  if type(st.sent) == v:t_string && st.sent !=# st.input
    " The items of what was typed are still to come.
    let st.keep_row = row
  endif
  " The cursor of another window moved, or the screen of the filter.
  redraw
endfunction

" The filter: input() in the command line, every change filtering (unless
" the items are more than filterUpdateMax: then when it is done).
function! s:open_filter(name, params) abort
  let st = s:st(a:name)
  let s:filter_name = a:name
  let s:filter_prev = st.input
  let max = get(st.params, 'filterUpdateMax', 0)
  augroup goda-fi-filter
    autocmd!
    if max <= 0 || get(st.params, 'itemCount', 0) <= max
      autocmd CmdlineChanged @ call s:filter_update(getcmdline())
    endif
  augroup END
  call goda_fi#util#doautocmd('GodaFi:uiOpenFilterWindow')
  redraw
  let prompt = get(a:params, 'prompt', get(st.params, 'filterPrompt', ''))
  try
    if has('nvim')
      let input = input({'prompt': prompt, 'default': st.input, 'cancelreturn': st.input})
    else
      let input = input(prompt, st.input)
    endif
  finally
    augroup goda-fi-filter
      autocmd!
    augroup END
    call goda_fi#util#doautocmd('GodaFi:uiCloseFilterWindow')
  endtry
  call s:filter_update(input)
endfunction

function! s:filter_update(input) abort
  if a:input ==# s:filter_prev
    return
  endif
  let s:filter_prev = a:input
  let st = s:st(s:filter_name)
  let st.sent = a:input
  let st.keep_row = 0
  call goda_fi#redraw(s:filter_name, {'input': a:input})
endfunction

" ---------------------------------------------------------------------------
" Preview

function! goda_fi#ui#ff#_close_preview(name) abort
  call s:close_preview(s:st(a:name))
endfunction

function! s:close_preview(st) abort
  let nr = win_id2win(a:st.preview_winid)
  if nr > 0
    if winnr('$') > 1
      execute 'silent!' nr . 'close!'
    endif
  endif
  let a:st.preview_winid = -1
  let a:st.preview_id = 0
endfunction

" Show the previewer pv of the item id.
function! goda_fi#ui#ff#_preview(name, id, pv, p) abort
  let st = s:st(a:name)
  if a:p.toggle && st.preview_id == a:id && win_id2win(st.preview_winid) > 0
    call s:close_preview(st)
    return
  endif
  let ffwin = bufwinid(st.bufnr)
  if a:p.previewSplit ==# 'no' || ffwin < 0
    return
  endif
  let [lines, path] = s:preview_lines(a:pv, a:p)

  let cur = win_getid()
  if win_id2win(st.preview_winid) <= 0
    call win_gotoid(ffwin)
    if a:p.previewSplit ==# 'vertical'
      execute 'silent rightbelow' s:eval(a:p.previewWidth) . 'vsplit'
    else
      execute 'silent aboveleft' s:eval(a:p.previewHeight) . 'split'
    endif
    let st.preview_winid = win_getid()
  endif
  call win_gotoid(st.preview_winid)
  if !bufexists(st.preview_bufnr)
    let st.preview_bufnr = bufadd('goda-fi-preview-' . a:name)
    call setbufvar(st.preview_bufnr, '&buftype', 'nofile')
    call bufload(st.preview_bufnr)
    call setbufvar(st.preview_bufnr, '&bufhidden', 'hide')
    call setbufvar(st.preview_bufnr, '&swapfile', 0)
    call setbufvar(st.preview_bufnr, '&buflisted', 0)
    call setbufvar(st.preview_bufnr, '&undolevels', -1)
  endif
  execute 'silent keepalt buffer' st.preview_bufnr
  setlocal modifiable
  silent %delete _
  call setline(1, lines)
  setlocal nomodifiable nomodified
  setlocal previewwindow nowrap nonumber norelativenumber nolist nofoldenable
  setlocal signcolumn=no foldcolumn=0
  " Highlighted as what it shows.
  let &l:syntax = get(a:pv, 'syntax', '')
  if get(a:pv, 'filetype', '') !=# ''
    let &l:filetype = a:pv.filetype
  elseif path !=# ''
    let &l:filetype = ''
    execute 'silent! doautocmd filetypedetect BufRead' fnameescape(path)
  else
    let &l:filetype = ''
  endif
  let lnum = get(a:pv, 'lineNr', 0)
  if lnum > 0
    call cursor(lnum, 1)
    normal! zz
  elseif get(a:pv, 'pattern', '') !=# ''
    call search(a:pv.pattern, 'cw')
    normal! zz
  else
    call cursor(1, 1)
  endif
  let st.preview_id = a:id
  call win_gotoid(cur)
  redraw
endfunction

" The lines of a previewer, and the path they are of.
function! s:preview_lines(pv, p) abort
  if a:pv.kind ==# 'nofile'
    return [get(a:pv, 'contents', []), '']
  endif
  if a:pv.kind !=# 'buffer'
    return [['(no preview of ' . a:pv.kind . ')'], '']
  endif
  let expr = get(a:pv, 'expr', v:null)
  if expr isnot v:null && bufexists(expr)
    let bufnr = type(expr) == v:t_number ? expr : bufnr(expr)
    call bufload(bufnr)
    return [getbufline(bufnr, 1, '$'), bufname(bufnr)]
  endif
  let path = get(a:pv, 'path', '')
  if bufloaded(path)
    return [getbufline(bufnr(path), 1, '$'), path]
  endif
  if isdirectory(path)
    return [readdir(path), '']
  endif
  if !filereadable(path)
    return [['(cannot read ' . path . ')'], '']
  endif
  if getfsize(path) > a:p.maxSize
    return [['(too large: ' . path . ')'], '']
  endif
  let lines = readfile(path, '', 10000)
  " A NUL is a newline in what readfile() reads.
  if match(lines[: 100], "\n") >= 0
    return [['(binary: ' . path . ')'], '']
  endif
  return [lines, path]
endfunction
