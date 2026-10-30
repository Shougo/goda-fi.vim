// Package filepoint is the source "filepoint" of goda-fi.vim, as
// ddu-source-file_point: the file under the cursor of the window the session
// was started in, with the line and column that follow it ("path:10:3",
// "path", line 10, ...): a compiler's message, a stack trace, a diff.
package filepoint

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type source struct{}

// New returns the source.
func New() fi.Source { return source{} }

func (source) Name() string { return "filepoint" }
func (source) Kind() string { return "file" }

// patterns are ddu-source-file_point's: the path, the line, the column.
var patterns = []*regexp.Regexp{
	// {path}:{line}:{col}: messages
	regexp.MustCompile(`((?:file://)?[~./\\][./\\\w~@-]*(?:\.[a-zA-Z0-9]+)?)(?::(\d+))?(?::(\d+))?`),
	// "{path}", line {line}
	regexp.MustCompile(`["']([A-Za-z]:[\\/][^"]*|[~./\\a-zA-Z_][^"]*)["'],?\s+line:?\s+(\d+)`),
	// {path}({line},{col})
	regexp.MustCompile(`([A-Za-z]:[\\/][\w./\\@~-]+|[~./\\a-zA-Z_][\w./\\@~-]+)\s*\((\d+),\s*(\d+)(?:-(\d+))?\)`),
	// {path} line {line}:
	regexp.MustCompile(`([A-Za-z]:[\\/]\S*|[~./\\a-zA-Z_@-]\S*[/.\\]\S*)\s+line\s+(\d+):`),
	// {path} {line}:{col}
	regexp.MustCompile(`([A-Za-z]:[\\/]\S*|[~./\\a-zA-Z_@-]\S*)\s+(\d+):(\d+)`),
	// {line}:{col}: messages
	regexp.MustCompile(`^()\s+(\d+):(\d+).*$`),
	// @@ -{line},{col}, +{line},{col} @@
	regexp.MustCompile(`^()@@\s+[-+](\d+),(\d+)\s+[-+](\d+),(\d+)\s+@@(.*$)`),
}

var arrow = regexp.MustCompile(`.*\s+=>\s+`)

// point is the cursor of the window: its line, the column (1-based bytes),
// and for a terminal the line before.
type point struct {
	Line     string `json:"line"`
	Col      int    `json:"col"`
	PrevLine string `json:"prevLine"`
	Terminal bool   `json:"terminal"`
	Cwd      string `json:"cwd"`
}

type check struct {
	line string
	col  int
}

func (source) Gather(ctx context.Context, h sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	raw, err := h.Call(ctx, "goda_fi#source#filepoint#_point", a.Context.WinID)
	if err != nil {
		return err
	}
	var pt point
	if err := sdk.Decode(raw, &pt); err != nil {
		return err
	}
	if pt.Cwd == "" {
		pt.Cwd = a.Context.Cwd
	}
	find := func(path string) string {
		if exists(path) || exists(abs(path, pt.Cwd)) {
			return path
		}
		r, err := h.Call(ctx, "goda_fi#source#filepoint#_find", a.Context.WinID, strings.TrimPrefix(path, "./"))
		s, _ := r.(string)
		if err != nil {
			return ""
		}
		return s
	}

	cfile := Cfile(pt.Line, pt.Col)
	if strings.HasPrefix(cfile, "http://") || strings.HasPrefix(cfile, "https://") {
		// URLs are for a kind "url" (not yet).
		return nil
	}
	var checks []check
	if pt.Terminal && pt.PrevLine != "" {
		checks = append(checks, check{pt.PrevLine + pt.Line, pt.Col + len(pt.PrevLine)})
	}
	checks = append(checks, check{arrow.ReplaceAllString(pt.Line, ""), pt.Col})

	for _, re := range patterns {
		for _, c := range checks {
			m := re.FindStringSubmatch(c.line)
			if m == nil {
				continue
			}
			f := Cfile(c.line, c.col)
			if strings.HasPrefix(f, "a/") || strings.HasPrefix(f, "b/") {
				f = f[2:]
			}
			found := find(f)
			if found == "" {
				continue
			}
			word := found
			ad := file.ActionData{Path: abs(found, pt.Cwd)}
			if len(m) > 2 && m[2] != "" {
				word += ":" + m[2]
				ad.LineNr, _ = strconv.Atoi(m[2])
			}
			if len(m) > 3 && m[3] != "" {
				word += ":" + m[3]
				ad.Col, _ = strconv.Atoi(m[3])
			}
			return emit([]fi.Item{{Word: word, Action: ad}})
		}
	}
	if cfile != "" && strings.Trim(cfile, "/") != "" &&
		(strings.Contains(cfile, "/") || filepath.Ext(cfile) != "") {
		if found := find(cfile); found != "" {
			return emit([]fi.Item{{Word: found, Action: file.ActionData{Path: abs(found, pt.Cwd)}}})
		}
	}
	return nil
}

// Cfile is the file name around the byte column col (1-based) of line, as
// ddu#source#file_point#cfile(): "~" expanded, "file://" and a ":{line}"
// suffix removed.
func Cfile(line string, col int) string {
	if col < 1 {
		return ""
	}
	col = min(col, len(line))
	start := col
	for start > 0 && isFileChar(line[start-1]) {
		start--
	}
	end := col
	for end < len(line) && isFileChar(line[end]) {
		end++
	}
	f := line[start:end]
	if strings.HasPrefix(f, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			f = filepath.ToSlash(home) + f[1:]
		}
	}
	f = strings.ReplaceAll(f, `\`, "/")
	f = strings.TrimPrefix(f, "file://")
	if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
		return f
	}
	return lineSuffix.ReplaceAllString(f, "")
}

var lineSuffix = regexp.MustCompile(`(?::\d+)+:?$`)

func isFileChar(c byte) bool {
	switch {
	case '0' <= c && c <= '9', 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		return true
	}
	return strings.IndexByte(`_./#+*$%@,{}[]!~=:\?&-`, c) >= 0
}

func abs(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(cwd, path)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
