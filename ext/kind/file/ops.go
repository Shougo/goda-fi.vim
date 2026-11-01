package file

// The actions of the filer that change files, as ddu-kind-file's: the
// clipboard (copy, move, link, then paste), rename, delete, trash, new
// files and directories, and undo. The files are changed here; Vim asks
// (input(), confirm()) and keeps its buffers in step
// (autoload/goda_fi/kind/file.vim).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Params are the kind params (kindParams.file), as ddu-kind-file's.
type Params struct {
	// TrashCommand moves a file to the trash, the path after it.
	TrashCommand []string `json:"trashCommand"`
}

// DefaultParams are ddu-kind-file's.
func DefaultParams() Params {
	return Params{TrashCommand: []string{"gio", "trash"}}
}

// state is what the actions keep between them, shared by the sessions (as
// ddu.vim's clipboard and action history): the clipboard and what undo
// takes back.
type state struct {
	mu      sync.Mutex
	clip    clipboard
	history []done
}

type clipboard struct {
	// action: "copy", "move" or "link" (mode "absolute", "relative",
	// "hard").
	action string
	mode   string
	paths  []string
}

// done is an action undo can take back: src (moved, renamed) to dest
// (made: copied, linked, new).
type done struct {
	name string
	src  string
	dest string
}

var defaultState = &state{}

func (k Kind) state() *state {
	if k.st == nil {
		return defaultState
	}
	return k.st
}

func (s *state) record(actions []done) {
	s.mu.Lock()
	s.history = actions
	s.mu.Unlock()
}

// fileActions are the actions that change files.
func (k Kind) fileActions() map[string]fi.Action {
	s := k.state()
	return map[string]fi.Action{
		"copy": {Description: "Copy the items to the clipboard (paste copies them).",
			Run: s.toClipboard("copy")},
		"move": {Description: "Move the items to the clipboard (paste moves them).",
			Run: s.toClipboard("move")},
		"link": {Description: "Link the items with paste (params: mode \"absolute\", \"relative\" or \"hard\").",
			Run: s.toClipboard("link")},
		"paste": {Description: "Do what the clipboard holds in the directory of the item.",
			Run: s.paste},
		"clipYank": {Description: "Yank the paths to the clipboard register (\"+), for clipCopy and clipMove.",
			Run: clipYank},
		"clipCopy": {Description: "Copy the files of the clipboard register (\"+) to the directory of the item.",
			Run: s.clipPaste("copy")},
		"clipMove": {Description: "Move the files of the clipboard register (\"+) to the directory of the item.",
			Run: s.clipPaste("move")},
		"delete": {Description: "Delete the files and directories.",
			Run: s.delete},
		"trash": {Description: "Move the files and directories to the trash (kindParams trashCommand).",
			Run: s.trash},
		"rename": {Description: "Rename the item; several in a buffer (write it to rename).",
			Run: s.rename},
		"edit": {Description: "The same as rename.",
			Run: s.rename},
		"newFile": {Description: "Make files in the directory of the item (comma separated; \"name/\" a directory).",
			Run: s.newFiles(false)},
		"newDirectory": {Description: "Make directories in the directory of the item (comma separated).",
			Run: s.newFiles(true)},
		"undo": {Description: "Take back the last copy, move, link, rename or new file.",
			Run: s.undo},
		"executeSystem": {Description: "Open the file with the program of the system (params: method).",
			Run: executeSystem},
	}
}

// pathOf is the path of an item: that of its action, or its word.
func pathOf(it fi.Item) string {
	var t target
	if it.Action != nil {
		_ = sdk.Decode(it.Action, &t.ActionData)
	}
	if t.Path == "" {
		return it.Word
	}
	return t.Path
}

// targetDirectory is where new files go: the tree expanded under the
// cursor, the directory of the item, or the path of the source.
func targetDirectory(a fi.ActionArgs) string {
	dir := a.SourcePath()
	for _, it := range a.Items {
		path := pathOf(it)
		if it.Expanded() {
			dir = path
		} else {
			dir = filepath.Dir(path)
		}
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func call[T any](ctx context.Context, h sdk.Host, fn string, args ...any) (T, error) {
	var out T
	r, err := h.Call(ctx, "goda_fi#kind#file#"+fn, args...)
	if err != nil {
		return out, err
	}
	err = sdk.Decode(r, &out)
	return out, err
}

func message(ctx context.Context, h sdk.Host, msg string) {
	_, _ = h.Call(ctx, "goda_fi#kind#file#print", msg)
}

// confirm asks with confirm(): the choice (1-based), def when it fails.
func confirm(ctx context.Context, h sdk.Host, msg, choices string, def int) int {
	n, err := call[int](ctx, h, "confirm", msg, choices, def)
	if err != nil {
		return def
	}
	return n
}

// input asks with input() in the directory cwd ("" when cancelled).
func input(ctx context.Context, h sdk.Host, cwd, prompt, text, completion string) (string, error) {
	return call[string](ctx, h, "cwd_input", cwd, prompt, text, completion)
}

func describe(items []fi.Item) string {
	if len(items) > 1 {
		return fmt.Sprintf("%d files", len(items))
	}
	return pathOf(items[0])
}

func (s *state) toClipboard(action string) func(context.Context, sdk.Host, fi.ActionArgs) (fi.ActionResult, error) {
	return func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
		mode := ""
		if action == "link" {
			var p struct {
				Mode string `json:"mode"`
			}
			p.Mode = "absolute"
			if err := fi.Decode(a.ActionParams, &p); err != nil {
				return fi.ActionResult{}, err
			}
			mode = p.Mode
		}
		paths := make([]string, len(a.Items))
		for i, it := range a.Items {
			paths[i] = pathOf(it)
		}
		s.mu.Lock()
		s.clip = clipboard{action: action, mode: mode, paths: paths}
		s.mu.Unlock()
		verb := map[string]string{"copy": "Copy", "move": "Move", "link": "Link"}[action]
		message(ctx, h, verb+" to the clipboard: "+describe(a.Items))
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
}

// overwrite is where src goes when dest is there already, as
// ddu-kind-file's checkOverwrite asks: dest (Force), src when dest is
// newer (Time: nothing), dest + "_", nothing, or a name typed.
func overwrite(ctx context.Context, h sdk.Host, src, dest string) (string, error) {
	sst, err := os.Lstat(src)
	if err != nil {
		return "", nil
	}
	dst, err := os.Lstat(dest)
	if err != nil {
		return dest, nil
	}
	msg := fmt.Sprintf(" src: %s %d bytes\n      %s\ndest: %s %d bytes\n      %s\n%s already exists.  Overwrite?",
		src, sst.Size(), sst.ModTime().Format(time.RFC3339), dest, dst.Size(), dst.ModTime().Format(time.RFC3339), dest)
	switch confirm(ctx, h, msg, "&Force\n&Time\n&Underbar\n&No\n&Rename", 4) {
	case 1:
		return dest, nil
	case 2:
		if dst.ModTime().Before(sst.ModTime()) {
			return dest, nil
		}
		return "", nil
	case 3:
		return dest + "_", nil
	case 5:
		completion := "file"
		if sst.IsDir() {
			completion = "dir"
		}
		name, err := input(ctx, h, "", "Please input a new name: "+dest+" -> ", dest, completion)
		if err != nil || name == dest {
			return "", err
		}
		return name, nil
	}
	return "", nil
}

func (s *state) paste(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	s.mu.Lock()
	clip := s.clip
	s.mu.Unlock()
	if clip.action == "" {
		message(ctx, h, "The clipboard is empty.")
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	return s.pasteFiles(ctx, h, targetDirectory(a), clip)
}

func (s *state) pasteFiles(ctx context.Context, h sdk.Host, cwd string, clip clipboard) (fi.ActionResult, error) {
	var history []done
	search := ""
	for _, src := range clip.paths {
		dest, err := overwrite(ctx, h, src, filepath.Join(cwd, filepath.Base(src)))
		if err != nil {
			return fi.ActionResult{}, err
		}
		if dest == "" {
			continue
		}
		switch clip.action {
		case "link":
			if exists(dest) {
				message(ctx, h, dest+" already exists.")
				continue
			}
			if err := link(clip.mode, src, dest); err != nil {
				return fi.ActionResult{}, err
			}
		case "move":
			modified, _ := call[bool](ctx, h, "modified", src)
			if modified {
				message(ctx, h, src+" is modified.")
				continue
			}
			if err := safeAction("move", src, dest); err != nil {
				return fi.ActionResult{}, err
			}
			_, _ = h.Call(ctx, "goda_fi#kind#file#buffer_rename", src, dest)
		default:
			if err := safeAction("copy", src, dest); err != nil {
				return fi.ActionResult{}, err
			}
		}
		history = append(history, done{name: clip.action, src: src, dest: dest})
		search = dest
	}
	s.record(history)
	if search == "" {
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	return fi.ActionResult{Flags: fi.RefreshItems, SearchPath: search}, nil
}

func link(mode, src, dest string) error {
	switch mode {
	case "hard":
		return os.Link(src, dest)
	case "absolute":
		return os.Symlink(src, dest)
	case "relative":
		rel, err := filepath.Rel(filepath.Dir(dest), src)
		if err != nil {
			return err
		}
		return os.Symlink(rel, dest)
	}
	return fmt.Errorf("invalid mode: %q", mode)
}

func clipYank(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	paths := make([]string, len(a.Items))
	for i, it := range a.Items {
		paths[i] = pathOf(it)
	}
	_, err := h.Call(ctx, "setreg", "+", strings.Join(paths, "\n"), "v")
	return fi.ActionResult{Flags: fi.Persist}, err
}

// clipPaste copies or moves the files of the clipboard register (another
// Vim's clipYank, say). Undo cannot take it back.
func (s *state) clipPaste(action string) func(context.Context, sdk.Host, fi.ActionArgs) (fi.ActionResult, error) {
	return func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
		reg, err := call[string](ctx, h, "getreg", "+")
		if err != nil {
			return fi.ActionResult{}, err
		}
		var paths []string
		for _, p := range strings.Split(reg, "\n") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		res, err := s.pasteFiles(ctx, h, targetDirectory(a), clipboard{action: action, paths: paths})
		s.record(nil)
		return res, err
	}
}

func (s *state) delete(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	if confirm(ctx, h, "Are you sure you want to delete "+describe(a.Items)+"?", "&Yes\n&No\n&Cancel", 2) != 1 {
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	search := ""
	for _, it := range a.Items {
		path := pathOf(it)
		search = filepath.Dir(path)
		if err := os.RemoveAll(path); err != nil {
			return fi.ActionResult{}, err
		}
		_, _ = h.Call(ctx, "goda_fi#kind#file#buffer_delete", path)
	}
	s.record(nil)
	return fi.ActionResult{Flags: fi.RefreshItems, SearchPath: search}, nil
}

func (s *state) trash(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	if confirm(ctx, h, "Are you sure you want to move to the trash "+describe(a.Items)+"?", "&Yes\n&No\n&Cancel", 2) != 1 {
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	p := DefaultParams()
	if err := fi.Decode(a.KindParams, &p); err != nil {
		return fi.ActionResult{}, err
	}
	if len(p.TrashCommand) == 0 {
		return fi.ActionResult{}, errors.New("no trashCommand")
	}
	if _, err := exec.LookPath(p.TrashCommand[0]); err != nil {
		return fi.ActionResult{}, fmt.Errorf("%s is not found", p.TrashCommand[0])
	}
	for _, it := range a.Items {
		path := pathOf(it)
		args := append(append([]string(nil), p.TrashCommand[1:]...), path)
		out, err := exec.CommandContext(ctx, p.TrashCommand[0], args...).CombinedOutput()
		if err != nil {
			return fi.ActionResult{}, fmt.Errorf("%s: %v: %s", strings.Join(append(p.TrashCommand, path), " "), err, strings.TrimSpace(string(out)))
		}
		_, _ = h.Call(ctx, "goda_fi#kind#file#buffer_delete", path)
	}
	s.record(nil)
	return fi.ActionResult{Flags: fi.RefreshItems}, nil
}

func (s *state) rename(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	if len(a.Items) > 1 {
		// A buffer of their names: written, the files are renamed.
		paths := make([]string, len(a.Items))
		for i, it := range a.Items {
			paths[i] = pathOf(it)
		}
		_, err := h.Call(ctx, "goda_fi#kind#file#exrename#create_buffer", paths,
			map[string]any{"cwd": a.SourcePath(), "name": a.Context.Name})
		s.record(nil)
		return fi.ActionResult{Flags: fi.Persist}, err
	}
	cwd := a.SourcePath()
	var history []done
	search := ""
	for _, it := range a.Items {
		path := pathOf(it)
		modified, _ := call[bool](ctx, h, "modified", path)
		if modified {
			message(ctx, h, path+" is modified.")
			continue
		}
		completion := "file"
		if isDir(path) {
			completion = "dir"
		}
		dest, err := input(ctx, h, cwd, "Please input a new name: "+path+" -> ", path, completion)
		if err != nil {
			return fi.ActionResult{}, err
		}
		if dest == "" || dest == path {
			continue
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(cwd, dest)
		}
		if err := safeAction("rename", path, dest); err != nil {
			return fi.ActionResult{}, err
		}
		_, _ = h.Call(ctx, "goda_fi#kind#file#buffer_rename", path, dest)
		history = append(history, done{name: "rename", src: path, dest: dest})
		search = dest
	}
	if search == "" {
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	s.record(history)
	return fi.ActionResult{Flags: fi.RefreshItems, SearchPath: search}, nil
}

func (s *state) newFiles(dirs bool) func(context.Context, sdk.Host, fi.ActionArgs) (fi.ActionResult, error) {
	return func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
		cwd := targetDirectory(a)
		prompt, completion := "Please input names(comma separated): ", "file"
		if dirs {
			prompt, completion = "Please input directory names(comma separated): ", "dir"
		}
		in, err := input(ctx, h, cwd, prompt, "", completion)
		if err != nil {
			return fi.ActionResult{}, err
		}
		var history []done
		search := ""
		for _, name := range strings.Split(in, ",") {
			if name = strings.TrimSpace(name); name == "" {
				continue
			}
			path := name
			if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, name)
			}
			if exists(path) {
				message(ctx, h, path+" already exists.")
				continue
			}
			if dirs || strings.HasSuffix(name, "/") {
				err = os.MkdirAll(path, 0o755)
			} else if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				var f *os.File
				if f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err == nil {
					err = f.Close()
				}
			}
			if err != nil {
				return fi.ActionResult{}, err
			}
			name := "newFile"
			if dirs {
				name = "newDirectory"
			}
			history = append(history, done{name: name, dest: path})
			search = path
		}
		if search == "" {
			return fi.ActionResult{Flags: fi.Persist}, nil
		}
		s.record(history)
		return fi.ActionResult{Flags: fi.RefreshItems, SearchPath: search}, nil
	}
}

func (s *state) undo(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	s.mu.Lock()
	history := append([]done(nil), s.history...)
	s.mu.Unlock()
	if len(history) == 0 {
		message(ctx, h, "Nothing to undo.")
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	var names []string
	for _, d := range history {
		names = append(names, d.name+":"+d.dest)
	}
	what := "action"
	if len(history) > 1 {
		what = "actions"
	}
	if confirm(ctx, h, fmt.Sprintf("Are you sure you want to undo %s %s?", strings.Join(names, ","), what), "&Yes\n&No\n&Cancel", 2) != 1 {
		return fi.ActionResult{Flags: fi.Persist}, nil
	}
	var redo []done
	search := ""
	for i := len(history) - 1; i >= 0; i-- {
		d := history[i]
		switch d.name {
		case "copy", "link", "newFile", "newDirectory":
			if err := os.RemoveAll(d.dest); err != nil {
				return fi.ActionResult{}, err
			}
			search = filepath.Dir(d.dest)
		case "move", "rename":
			if err := move(d.dest, d.src); err != nil {
				return fi.ActionResult{}, err
			}
			_, _ = h.Call(ctx, "goda_fi#kind#file#buffer_rename", d.dest, d.src)
			// Undone again, it is moved back.
			redo = append(redo, done{name: d.name, src: d.dest, dest: d.src})
			search = d.src
		}
	}
	s.record(redo)
	return fi.ActionResult{Flags: fi.RefreshItems, SearchPath: search}, nil
}

// safeAction renames, moves or copies src to dest, as ddu-kind-file's: a
// dest there is replaced (src may be dest: renamed aside first).
func safeAction(action, src, dest string) error {
	if !isDir(filepath.Dir(dest)) {
		return fmt.Errorf("%s is not a directory", filepath.Dir(dest))
	}
	if action != "copy" && exists(dest) {
		temp := src + "___"
		if err := os.Rename(src, temp); err != nil {
			return err
		}
		if exists(dest) {
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
		}
		src = temp
	}
	switch action {
	case "rename":
		return os.Rename(src, dest)
	case "move":
		return move(src, dest)
	}
	return copyPath(src, dest)
}

// move renames src to dest, or copies it and removes it (another device).
func move(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyPath(src, dest); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// copyPath copies a file, a link or a directory (recursively) over dest.
func copyPath(src, dest string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case st.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dest)
		return os.Symlink(target, dest)
	case st.IsDir():
		if dest == src || strings.HasPrefix(dest, src+string(filepath.Separator)) {
			return fmt.Errorf("cannot copy %s into itself", src)
		}
		if err := os.MkdirAll(dest, st.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dest, st.ModTime(), st.ModTime())
}

// executeSystem opens the files with the program of the system, as
// ddu-kind-file's: params.method "nvim-open" (vim.ui.open()), an
// executable ("xdg-open", "open", "wslview", ...), "windows-rundll32", or
// "" (the one of the system).
func executeSystem(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
	var p struct {
		Method string `json:"method"`
	}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.ActionResult{}, err
	}
	for _, it := range a.Items {
		path := pathOf(it)
		if !filepath.IsAbs(path) {
			path = a.Abs(path)
		}
		var err error
		switch p.Method {
		case "nvim-open":
			_, err = h.Call(ctx, "luaeval", "vim.ui.open(_A)", path)
		case "":
			err = start(systemOpener(runtime.GOOS, path))
		case "windows-rundll32":
			err = start("rundll32", "url.dll,FileProtocolHandler", path)
		case "kioclient":
			err = start("kioclient", "exec", path)
		default:
			err = start(p.Method, path)
		}
		if err != nil {
			return fi.ActionResult{}, err
		}
	}
	return fi.ActionResult{Flags: fi.None}, nil
}

func systemOpener(goos, path string) (string, string, string) {
	switch goos {
	case "windows":
		return "rundll32", "url.dll,FileProtocolHandler", path
	case "darwin":
		return "open", path, ""
	}
	if _, err := exec.LookPath("wslview"); err == nil && isWSL() {
		return "wslview", path, ""
	}
	return "xdg-open", path, ""
}

func isWSL() bool {
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// start runs a program without waiting for it.
func start(name string, args ...string) error {
	var list []string
	for _, a := range args {
		if a != "" {
			list = append(list, a)
		}
	}
	cmd := exec.Command(name, list...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
