package filepoint

import "testing"

func TestCfile(t *testing.T) {
	for _, c := range []struct {
		line string
		col  int
		want string
	}{
		{"sub/c.txt:2:3: error", 3, "sub/c.txt"},
		{"see file:///tmp/x.go:10 here", 8, "/tmp/x.go"},
		{"at https://example.com/a:1", 10, "https://example.com/a:1"},
		{"  foo/bar.go  ", 1, ""},
		{"  foo/bar.go  ", 5, "foo/bar.go"},
		{"x", 0, ""},
	} {
		if got := Cfile(c.line, c.col); got != c.want {
			t.Errorf("Cfile(%q, %d) = %q, want %q", c.line, c.col, got, c.want)
		}
	}
}

func TestPatterns(t *testing.T) {
	for _, c := range []struct {
		i    int
		line string
		ok   bool
	}{
		{0, "foo/bar.txt:123:45", true},
		{0, "~/.gitconfig:4", true},
		{1, `"foo/bar.txt", line 123`, true},
		{1, "foo.txt line 1", false},
		{2, "./foo.txt(1, 2-5)", true},
		{3, "foo/bar.txt line 123:", true},
		{4, "foo/bar.txt 123:45", true},
		{4, "foo.txt:1:2", false},
		{5, "  123:45: error message", true},
		{6, "@@ -1,2 +3,4 @@ x", true},
	} {
		if got := patterns[c.i].MatchString(c.line); got != c.ok {
			t.Errorf("pattern %d on %q: %v", c.i, c.line, got)
		}
	}
	// The path is Cfile's; the pattern gives the line and the column.
	m := patterns[0].FindStringSubmatch("sub/c.txt:2:3: error")
	if m == nil || m[2] != "2" || m[3] != "3" {
		t.Errorf("submatches %q", m)
	}
}
