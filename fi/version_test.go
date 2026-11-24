package fi

import (
	"testing"
	"time"
)

// The versions of a binary started again are newer than those of the one
// before: the UIs of Vim draw only newer versions than theirs.
func TestVersionsOfANewBinary(t *testing.T) {
	old := newEngine(nil)
	var last int64
	for range 1000 {
		last = old.newVersion()
	}
	time.Sleep(time.Millisecond)
	if v := newEngine(nil).newVersion(); v <= last {
		t.Errorf("version of the new engine: %d, not newer than %d", v, last)
	}
	if last >= 1<<53 {
		t.Errorf("version %d: not exact in a float", last)
	}
}
