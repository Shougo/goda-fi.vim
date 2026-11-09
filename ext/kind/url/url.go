// Package url is the kind "url" of goda-fi.vim, as ddu-kind-url: items of
// URLs (ActionData), browsed, opened, yanked or fetched.
package url

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/Shougo/goda-fi.vim/ext/internal/vimscript"
	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

// The Vim side of the kind (url.vim): goda_fi#kind#url#*().
//
//go:embed url.vim
var vimSrc []byte

var script = vimscript.New("autoload/goda_fi/kind/url.vim", vimSrc)

// ActionData is the action of an item of this kind, as ddu-kind-url's.
type ActionData struct {
	// URL of the item; its word when empty.
	URL string `json:"url,omitempty"`
}

// Params are the kind params (kindParams.url), as ddu-kind-url's.
type Params struct {
	// ExternalOpener is how browse opens a URL:
	//   - "systemopen" (default): the program of the system (xdg-open,
	//     open, the shell of MS-Windows).
	//   - "openbrowser": openbrowser#open() of open-browser.vim.
	//   - "external": external#browser() of vim-external.
	//   - "uiopen": vim.ui.open() (Neovim).
	ExternalOpener string `json:"externalOpener"`
}

// Kind is the kind "url".
type Kind struct{}

// New returns the kind, to add to the builder.
func New() Kind { return Kind{} }

func (Kind) Name() string { return "url" }

// URLs are the URLs of the items.
func URLs(items []fi.Item) ([]string, error) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		var ad ActionData
		if it.Action != nil {
			if err := sdk.Decode(it.Action, &ad); err != nil {
				return nil, fmt.Errorf("%s: %w", it.Word, err)
			}
		}
		if ad.URL == "" {
			ad.URL = it.Word
		}
		out = append(out, ad.URL)
	}
	return out, nil
}

func action(desc string, run func(ctx context.Context, h sdk.Host, a fi.ActionArgs, urls []string) (fi.ActionFlags, error)) fi.Action {
	return fi.Action{
		Description: desc,
		Run: func(ctx context.Context, h sdk.Host, a fi.ActionArgs) (fi.ActionResult, error) {
			urls, err := URLs(a.Items)
			if err != nil {
				return fi.ActionResult{}, err
			}
			flags, err := run(ctx, h, a, urls)
			return fi.ActionResult{Flags: flags}, err
		},
	}
}

// Actions are ddu-kind-url's.
func (Kind) Actions() map[string]fi.Action {
	return map[string]fi.Action{
		"browse": action("Browse the URLs with an external browser (kindParams externalOpener).", browse),
		"open":   action("Open the URLs as buffers (params: command \"edit\"), for netrw and the like.", open),
		"yank":   action("Yank the URLs, one per line (params: register, v:register).", yank),
		"fetch":  action("Request the URLs and show the responses in a buffer (params: method, headers, body, showStatus, showHeader).", fetch),
	}
}

func browse(ctx context.Context, h sdk.Host, a fi.ActionArgs, urls []string) (fi.ActionFlags, error) {
	p := Params{ExternalOpener: "systemopen"}
	if err := fi.Decode(a.KindParams, &p); err != nil {
		return fi.None, err
	}
	for _, u := range urls {
		var err error
		switch p.ExternalOpener {
		case "systemopen":
			err = SystemOpen(u)
		case "openbrowser":
			_, err = h.Call(ctx, "openbrowser#open", u)
		case "external":
			_, err = h.Call(ctx, "external#browser", u)
		case "uiopen":
			_, err = h.Call(ctx, "luaeval", "vim.ui.open(_A)", u)
		default:
			return fi.None, fmt.Errorf("invalid externalOpener: %q", p.ExternalOpener)
		}
		if err != nil {
			return fi.None, err
		}
	}
	return fi.None, nil
}

// openCommand is the program of the system that opens a URL.
func openCommand(goos, u string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{u}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", u}
	}
	return "xdg-open", []string{u}
}

// SystemOpen opens u with the program of the system, without waiting for
// it.
func SystemOpen(u string) error {
	name, args := openCommand(runtime.GOOS, u)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

type openParams struct {
	Command string `json:"command"`
}

func open(ctx context.Context, h sdk.Host, a fi.ActionArgs, urls []string) (fi.ActionFlags, error) {
	p := openParams{Command: "edit"}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	_, err := script.Call(ctx, h, "goda_fi#kind#url#open", urls, p.Command)
	return fi.None, err
}

type yankParams struct {
	Register string `json:"register"`
}

func yank(ctx context.Context, h sdk.Host, a fi.ActionArgs, urls []string) (fi.ActionFlags, error) {
	var p yankParams
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	_, err := script.Call(ctx, h, "goda_fi#kind#url#yank", urls, p.Register)
	return fi.Persist, err
}

// FetchParams are the params of fetch, as ddu-kind-url's.
type FetchParams struct {
	Body       *string           `json:"body"`
	Headers    map[string]string `json:"headers"`
	Method     string            `json:"method"`
	ShowHeader bool              `json:"showHeader"`
	ShowStatus bool              `json:"showStatus"`
}

// maxFetch is the most fetch reads of a response.
const maxFetch = 10 << 20

func fetch(ctx context.Context, h sdk.Host, a fi.ActionArgs, urls []string) (fi.ActionFlags, error) {
	p := FetchParams{Method: "GET", ShowHeader: true, ShowStatus: true}
	if err := fi.Decode(a.ActionParams, &p); err != nil {
		return fi.None, err
	}
	for _, u := range urls {
		lines, err := Fetch(ctx, http.DefaultClient, u, p)
		if err != nil {
			return fi.None, err
		}
		if _, err := script.Call(ctx, h, "goda_fi#kind#url#show", lines); err != nil {
			return fi.None, err
		}
	}
	return fi.None, nil
}

// Fetch requests u and returns the lines of the response: its status and
// headers (as p asks), an empty line after each, and its body.
func Fetch(ctx context.Context, c *http.Client, u string, p FetchParams) ([]string, error) {
	var body io.Reader
	if p.Body != nil {
		body = strings.NewReader(*p.Body)
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, u, body)
	if err != nil {
		return nil, err
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	lines := []string{}
	if p.ShowStatus {
		lines = append(lines, resp.Status, "")
	}
	if p.ShowHeader {
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			for _, v := range resp.Header[k] {
				lines = append(lines, k+": "+v)
			}
		}
		lines = append(lines, "")
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxFetch))
	sc.Buffer(make([]byte, 64*1024), maxFetch)
	for sc.Scan() {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	return lines, sc.Err()
}
