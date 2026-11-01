package url

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Shougo/goda-fi.vim/fi"
	"github.com/Shougo/goda.vim/sdk"
)

type host struct{ calls []string }

func (h *host) Meta() sdk.Meta { return sdk.Meta{Host: sdk.HostNone} }
func (h *host) Call(_ context.Context, fn string, args ...any) (any, error) {
	h.calls = append(h.calls, fmt.Sprint(fn, args))
	return nil, nil
}
func (h *host) Notify(context.Context, string, ...any) error { return nil }
func (h *host) Eval(context.Context, string) (any, error)    { return nil, nil }
func (h *host) Cmd(context.Context, string) error            { return nil }

func run(t *testing.T, name string, a fi.ActionArgs) (*host, fi.ActionResult) {
	t.Helper()
	h := &host{}
	res, err := New().Actions()[name].Run(context.Background(), h, a)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return h, res
}

func TestActions(t *testing.T) {
	items := []fi.Item{
		{Word: "https://a.example", Action: ActionData{URL: "https://a.example/x"}},
		{Word: "https://b.example"},
	}
	h, res := run(t, "yank", fi.ActionArgs{Items: items})
	if res.Flags != fi.Persist || !slices.Equal(h.calls, []string{"goda_fi#kind#url#yank[[https://a.example/x https://b.example] ]"}) {
		t.Errorf("yank %v %q", res.Flags, h.calls)
	}
	h, _ = run(t, "open", fi.ActionArgs{Items: items[1:], ActionParams: fi.Params{"command": "tabedit"}})
	if !slices.Equal(h.calls, []string{"goda_fi#kind#url#open[[https://b.example] tabedit]"}) {
		t.Errorf("open %q", h.calls)
	}
	h, _ = run(t, "browse", fi.ActionArgs{Items: items[1:], KindParams: fi.Params{"externalOpener": "openbrowser"}})
	if !slices.Equal(h.calls, []string{"openbrowser#open[https://b.example]"}) {
		t.Errorf("browse %q", h.calls)
	}
	if _, err := New().Actions()["browse"].Run(context.Background(), &host{},
		fi.ActionArgs{Items: items, KindParams: fi.Params{"externalOpener": "nope"}}); err == nil {
		t.Error("an invalid externalOpener was accepted")
	}
}

func TestOpenCommand(t *testing.T) {
	for goos, want := range map[string]string{
		"linux":   "xdg-open [u]",
		"darwin":  "open [u]",
		"windows": "rundll32 [url.dll,FileProtocolHandler u]",
	} {
		name, args := openCommand(goos, "u")
		if got := fmt.Sprint(name, " ", args); got != want {
			t.Errorf("%s: %s", goos, got)
		}
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method+" "+r.Header.Get("X-Test"))
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "one\r\ntwo\n")
	}))
	defer srv.Close()
	body := "b"
	lines, err := Fetch(context.Background(), srv.Client(), srv.URL, FetchParams{
		Method: "POST", Body: &body, Headers: map[string]string{"X-Test": "t"},
		ShowHeader: true, ShowStatus: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if lines[0] != "200 OK" || lines[1] != "" || !slices.Contains(lines, "X-Method: POST t") ||
		!slices.Equal(lines[len(lines)-3:], []string{"", "one", "two"}) {
		t.Errorf("lines %q", lines)
	}
	lines, err = Fetch(context.Background(), srv.Client(), srv.URL, FetchParams{Method: "GET"})
	if err != nil || !slices.Equal(lines, []string{"one", "two"}) {
		t.Errorf("body only: %q %v", lines, err)
	}
}
