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
    call add(list, {'name': name, 'contents': info.regcontents,
          \ 'type': get(info, 'regtype', 'v')})
  endfor
  return list
endfunction
