// Package rg is the source "rg" of goda-fi.vim, as ddu-source-rg: the lines
// that ripgrep finds, in the directory SourceOptions.Path (or the current
// one). The pattern is params.input, or the input when the source is
// volatile.
package rg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/kind/file"
	"github.com/Shougo/goda-fi.vim/ext/source/internal/linestream"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// Highlights are the groups of the parts of an item ("": none).
type Highlights struct {
	Path   string `json:"path"`
	LineNr string `json:"lineNr"`
	Word   string `json:"word"`
}

// Params are the source params, as ddu-source-rg's ("inputType" "migemo"
// is not supported).
type Params struct {
	// Args of rg; with "--json" its JSON output is read.
	Args []string `json:"args"`
	// Category: a line of the path before the matches of every file, and
	// the matches without it.
	Category bool `json:"category"`
	// Cmd is the executable.
	Cmd   string   `json:"cmd"`
	Globs []string `json:"globs"`
	// Highlights of the path, the line number and the match.
	Highlights Highlights `json:"highlights"`
	// Input is the pattern (when the source is not volatile).
	Input     string `json:"input"`
	InputType string `json:"inputType"`
	// MaxEnqueSize: at most this many matches.
	MaxEnqueSize int `json:"maxEnqueSize"`
	// MinVolatileInputLength: a volatile source searches for no shorter
	// input.
	MinVolatileInputLength int `json:"minVolatileInputLength"`
	// Paths searched (relative to the directory).
	Paths []string `json:"paths"`
}

// DefaultParams are ddu-source-rg's.
func DefaultParams() Params {
	return Params{
		Args:                   []string{"--column", "--no-heading", "--color", "never"},
		Cmd:                    "rg",
		Globs:                  []string{},
		Highlights:             Highlights{Path: "Normal", LineNr: "Normal", Word: "Search"},
		InputType:              "regex",
		MaxEnqueSize:           10000,
		MinVolatileInputLength: 2,
		Paths:                  []string{},
	}
}

type source struct{ p Params }

// New returns the source with p over DefaultParams (the zero fields of p are
// the defaults); the source params of the options go over them.
func New(p Params) fi.Source {
	d := DefaultParams()
	if p.Args != nil {
		d.Args = p.Args
	}
	d.Category = p.Category
	if p.Cmd != "" {
		d.Cmd = p.Cmd
	}
	if p.Globs != nil {
		d.Globs = p.Globs
	}
	if p.Highlights != (Highlights{}) {
		d.Highlights = p.Highlights
	}
	if p.MaxEnqueSize > 0 {
		d.MaxEnqueSize = p.MaxEnqueSize
	}
	if p.MinVolatileInputLength > 0 {
		d.MinVolatileInputLength = p.MinVolatileInputLength
	}
	if p.Paths != nil {
		d.Paths = p.Paths
	}
	d.Input = p.Input
	return source{d}
}

func (source) Name() string { return "rg" }
func (source) Kind() string { return "file" }

func (s source) Gather(ctx context.Context, _ sdk.Host, a fi.GatherArgs, emit func([]fi.Item) error) error {
	p := s.p
	if err := fi.Decode(a.SourceParams, &p); err != nil {
		return err
	}
	if p.InputType != "" && p.InputType != "regex" {
		return fmt.Errorf("inputType %q is not supported", p.InputType)
	}
	volatile := a.SourceOptions.Volatile != nil && *a.SourceOptions.Volatile
	input := p.Input
	if volatile {
		input = a.Input
	}
	if input == "" || volatile && len([]rune(input)) < p.MinVolatileInputLength {
		return nil
	}
	cwd := a.SourcePath()
	args := slices.Clone(p.Args)
	for _, g := range p.Globs {
		args = append(args, "--glob", g)
	}
	args = append(args, "--", input)
	args = append(args, p.Paths...)

	cmd := exec.CommandContext(ctx, p.Cmd, args...)
	cmd.Dir = cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	useJSON := slices.Contains(p.Args, "--json")
	b := builder{p: p, cwd: cwd, input: input, seen: map[string]bool{}}
	total := 0
	var items []fi.Item
	var emitErr error
	flush := func() bool {
		if len(items) == 0 {
			return true
		}
		emitErr = emit(items)
		items = nil
		return emitErr == nil
	}
	readErr := linestream.Read(out, 16*1024*1024, func(line []byte) bool {
		var it *fi.Item
		if useJSON {
			it = b.parseJSON(line)
		} else {
			it = b.parseLine(string(line))
		}
		if it == nil || it.Word == "" {
			return true
		}
		items = b.push(items, *it)
		total++
		return total < p.MaxEnqueSize
	}, flush)
	if emitErr == nil {
		flush()
	}
	if total >= p.MaxEnqueSize || emitErr != nil || readErr != nil {
		_ = cmd.Process.Kill()
	}
	werr := cmd.Wait()
	if emitErr != nil {
		return emitErr
	}
	var exit *exec.ExitError
	switch {
	case werr == nil, ctx.Err() != nil, total >= p.MaxEnqueSize:
		return nil
	case errors.As(werr, &exit) && exit.ExitCode() == 1:
		return nil // nothing found
	}
	msg := strings.TrimSpace(stderr.String())
	if volatile && strings.Contains(msg, "regex parse error") {
		return nil // typing
	}
	if msg != "" {
		return fmt.Errorf("%s: %s", p.Cmd, msg)
	}
	return fmt.Errorf("%s: %w", p.Cmd, werr)
}

type builder struct {
	p     Params
	cwd   string
	input string
	seen  map[string]bool
}

func (b *builder) hl(items []fi.Highlight, name, group string, col, width int) []fi.Highlight {
	if group == "" || width <= 0 {
		return items
	}
	return append(items, fi.Highlight{Name: name, HlGroup: group, Col: col, Width: width})
}

func (b *builder) abs(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(b.cwd, path)
}

type rgText struct {
	Text string `json:"text"`
}

type rgMatch struct {
	Type string `json:"type"`
	Data struct {
		Path       rgText `json:"path"`
		Lines      rgText `json:"lines"`
		LineNumber int    `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

func (b *builder) parseJSON(line []byte) *fi.Item {
	var m rgMatch
	if json.Unmarshal(line, &m) != nil || m.Type != "match" || len(m.Data.Submatches) == 0 {
		return nil
	}
	path := m.Data.Path.Text
	lnum := m.Data.LineNumber
	sub := m.Data.Submatches[0]
	text := strings.TrimRight(m.Data.Lines.Text, "\r\n")
	header := fmt.Sprintf("%s:%d:%d: ", path, lnum, sub.Start)
	var hls []fi.Highlight
	hls = b.hl(hls, "path", b.p.Highlights.Path, 1, len(path))
	hls = b.hl(hls, "lineNr", b.p.Highlights.LineNr, len(path)+2, len(strconv.Itoa(lnum)))
	hls = b.hl(hls, "word", b.p.Highlights.Word, len(header)+sub.Start+1, sub.End-sub.Start)
	return &fi.Item{
		Word:       header + text,
		Action:     file.ActionData{Path: b.abs(path), LineNr: lnum, Col: sub.Start + 1, Text: text},
		Highlights: hls,
	}
}

var lineRe = regexp.MustCompile(`^(.+?):(\d+):(\d+):(.*)$`)

func (b *builder) parseLine(line string) *fi.Item {
	line = strings.TrimSpace(line)
	m := lineRe.FindStringSubmatch(line)
	var path, text string
	var lnum, col int
	if m != nil {
		path, text = m[1], m[4]
		lnum, _ = strconv.Atoi(m[2])
		col, _ = strconv.Atoi(m[3])
	}
	header := fmt.Sprintf("%s:%d:%d: ", path, lnum, col)
	textStart := len(path) + 2 + len(strconv.Itoa(lnum)) + len(strconv.Itoa(col)) + 2
	var hls []fi.Highlight
	hls = b.hl(hls, "path", b.p.Highlights.Path, 1, len(path))
	hls = b.hl(hls, "lineNr", b.p.Highlights.LineNr, len(path)+2, len(strconv.Itoa(lnum)))
	hls = b.hl(hls, "word", b.p.Highlights.Word, textStart+col, len(b.input))
	return &fi.Item{
		Word:       header + text,
		Action:     file.ActionData{Path: b.abs(path), LineNr: lnum, Col: col, Text: text},
		Highlights: hls,
	}
}

var headerRe = regexp.MustCompile(`^[^:]+:\d+:\d+: `)

// push adds it to items; with Category, after the line of its file.
func (b *builder) push(items []fi.Item, it fi.Item) []fi.Item {
	if !b.p.Category {
		return append(items, it)
	}
	ad := it.Action.(file.ActionData)
	if ad.Path != "" && !b.seen[ad.Path] {
		b.seen[ad.Path] = true
		rel := ad.Path
		if r, err := filepath.Rel(b.cwd, ad.Path); err == nil {
			rel = filepath.ToSlash(r)
		}
		var hls []fi.Highlight
		hls = b.hl(hls, "path", b.p.Highlights.Path, 1, len(rel))
		items = append(items, fi.Item{
			Word:       ad.Path + ":",
			Display:    rel + ":",
			Action:     file.ActionData{Path: ad.Path},
			Highlights: hls,
		})
	}
	it.Display = headerRe.ReplaceAllString(it.Word, "  ")
	it.Highlights = b.hl(nil, "word", b.p.Highlights.Word, max(ad.Col, 1)+2, len(b.input))
	return append(items, it)
}
