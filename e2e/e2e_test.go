// Package e2e runs goda-fi in a real Vim and Neovim: the scenario
// testdata/test.vim, with the binary of testbin.
//
// It is a module of its own, with the UIs and the exts (goda-fi-ui-ff.vim,
// goda-fi-ui-filer.vim, goda-fi-ext), which require goda-fi.vim: this one
// replaces it.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// editor is the executable of Vim or Neovim: $GODA_VIM or $GODA_NVIM, or
// the one in PATH.
func editor(t *testing.T, env, name string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s", name)
	}
	return p
}

// setup builds the binary and returns the environment of the scenario.
func setup(t *testing.T) []string {
	t.Helper()
	return setupBinary(t, "./testbin")
}

// resultFile is where the scenario writes its result.
func resultFile(env []string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "GODA_FI_RESULT="); ok {
			return v
		}
	}
	return ""
}

// setupBinary builds the binary of pkg and returns the environment of a
// scenario.
func setupBinary(t *testing.T, pkg string) []string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "goda-fi")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := filepath.Abs("testdata/tree")
	if err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(),
		// The Vim script of goda.vim and of the UIs: the modules this module
		// requires.
		"GODA_VIM_DIR="+moduleDir(t, "github.com/Shougo/goda.vim"),
		"GODA_FI_DIR="+root,
		"GODA_FI_UI_FF_DIR="+moduleDir(t, "github.com/Shougo/goda-fi-ui-ff.vim"),
		"GODA_FI_UI_FILER_DIR="+moduleDir(t, "github.com/Shougo/goda-fi-ui-filer.vim"),
		"GODA_FI_BINARY="+bin,
		"GODA_FI_TREE="+tree,
		"GODA_FI_RESULT="+filepath.Join(dir, "result"),
	)
}

// moduleDir is the directory of the module path this module requires.
func moduleDir(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", path).Output()
	if err != nil {
		t.Fatalf("go list %s: %v", path, err)
	}
	return strings.TrimSpace(string(out))
}

func run(t *testing.T, env []string, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	b, rerr := os.ReadFile(resultFile(env))
	if rerr != nil {
		t.Fatalf("no result (%v): %v\n%s", err, rerr, out)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	ok := false
	for _, l := range lines {
		if l == "ok" {
			ok = true
		}
	}
	if !ok || strings.Contains(string(b), "[notify]") {
		t.Errorf("%s", strings.Join(lines, "\n"))
	}
}

func script(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("testdata/test.vim")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVim(t *testing.T) {
	vim := editor(t, "GODA_VIM", "vim")
	env := setup(t)
	run(t, env, vim, "-N", "-u", "NONE", "-i", "NONE", "-n", "--not-a-term",
		"-S", script(t))
}

func TestNeovim(t *testing.T) {
	nvim := editor(t, "GODA_NVIM", "nvim")
	env := setup(t)
	run(t, env, nvim, "--headless", "--clean", "-n", "-S", script(t))
}
