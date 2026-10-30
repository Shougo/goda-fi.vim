package fi

import (
	"os"
	"path/filepath"
	"strings"
)

// SourcePath is the directory a source works in: SourceOptions.Path, else
// Context.Path (Vim's current directory at the start), with "~" expanded and
// relative to Vim's current directory (not that of this process).
func (a GatherArgs) SourcePath() string {
	p := a.SourceOptions.Path
	if p == "" && a.Context != nil {
		p = a.Context.Path
	}
	return a.Abs(p)
}

// Abs is path made absolute: "~" expanded, relative to Vim's current
// directory.
func (a GatherArgs) Abs(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	if !filepath.IsAbs(path) && a.Context != nil && a.Context.Cwd != "" {
		path = filepath.Join(a.Context.Cwd, path)
	}
	return filepath.Clean(path)
}
