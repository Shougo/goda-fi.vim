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
	return sourcePath(a.Context, a.SourceOptions)
}

// Abs is path made absolute: "~" expanded, relative to Vim's current
// directory.
func (a GatherArgs) Abs(path string) string { return abs(a.Context, path) }

// SourcePath is that of the source of the items (GatherArgs.SourcePath).
func (a FilterArgs) SourcePath() string {
	return sourcePath(a.Context, a.SourceOptions)
}

func sourcePath(c *Context, so SourceOptions) string {
	p := so.Path
	if p == "" && c != nil {
		p = c.Path
	}
	return abs(c, p)
}

func abs(c *Context, path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	if !filepath.IsAbs(path) && c != nil && c.Cwd != "" {
		path = filepath.Join(c.Cwd, path)
	}
	return filepath.Clean(path)
}
