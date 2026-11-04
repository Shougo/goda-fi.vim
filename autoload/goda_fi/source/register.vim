" The registers of the source "register" (ext/source/register).
function! goda_fi#source#register#_list() abort
  let names = (has('clipboard') ? ['+', '*'] : [])
        \ + ['"'] + map(range(10), 'string(v:val)')
        \ + map(range(char2nr('a'), char2nr('z')), 'nr2char(v:val)')
        \ + ['-', '.', ':', '#', '%', '/', '=']
  let list = []
  for name in names
    try
      let info = getreginfo(name)
    catch
      continue
    endtry
    if empty(get(info, 'regcontents', []))
      continue
    endif
    call add(list, {'name': name,
          \ 'contents': map(copy(info.regcontents), {_, l -> s:strip_special(l)}),
          \ 'type': get(info, 'regtype', 'v')})
  endfor
  return list
endfunction

" {line} without the special keys of a register (a macro): K_SPECIAL (the
" byte 0x80 alone, not that of a character: "、" is E3 80 81) and its two
" bytes.
function! s:strip_special(line) abort
  if stridx(a:line, "\x80") < 0
    return a:line
  endif
  let out = []
  let skip = 0
  for c in split(a:line, '\zs')
    if skip > 0
      let skip -= len(c)
    elseif c ==# "\x80"
      let skip = 2
    else
      call add(out, c)
    endif
  endfor
  return join(out, '')
endfunction
