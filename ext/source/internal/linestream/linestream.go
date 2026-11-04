// Package linestream reads the output of a command line by line for the
// sources, so that they can hand over the items they have every little while
// instead of waiting for a chunk to fill (a search that finds a few matches
// in a large tree would show nothing before it ends).
package linestream

import (
	"bufio"
	"io"
	"time"
)

// Interval is how often Read calls tick while lines come.
var Interval = 30 * time.Millisecond

// Read reads r (the lines up to maxLine bytes long) in a goroutine and calls
// line with each of them in the caller's goroutine; line must not keep the
// slice. tick is called every Interval, so that the pending items are
// handed over (the caller hands over the last ones after Read). Read returns when r ends, or when line or
// tick returns false (the caller then stops the writer of r: kills the
// command); it returns the error of the reading.
func Read(r io.Reader, maxLine int, line func([]byte) bool, tick func() bool) error {
	type batch struct {
		lines [][]byte
		err   error
	}
	ch := make(chan batch, 16)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), maxLine)
		// The lines are sent by small batches: a channel operation by
		// line is slow for the large outputs.
		var lines [][]byte
		send := func(b batch) bool {
			select {
			case ch <- b:
				return true
			case <-done:
				return false
			}
		}
		for sc.Scan() {
			lines = append(lines, append([]byte(nil), sc.Bytes()...))
			if len(lines) >= 64 || len(ch) == 0 {
				if !send(batch{lines: lines}) {
					return
				}
				lines = nil
			}
		}
		if len(lines) > 0 && !send(batch{lines: lines}) {
			return
		}
		if err := sc.Err(); err != nil {
			send(batch{err: err})
		}
	}()
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		select {
		case b, ok := <-ch:
			if !ok {
				return nil
			}
			if b.err != nil {
				return b.err
			}
			for _, l := range b.lines {
				if !line(l) {
					return nil
				}
			}
		case <-t.C:
			if !tick() {
				return nil
			}
		}
	}
}
