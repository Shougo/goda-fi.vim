package linestream

import (
	"io"
	"testing"
	"time"
)

// The lines are handed over while the writer is still writing.
func TestTickBeforeEnd(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	go func() { _, _ = io.WriteString(w, "a\nb\n") }()
	var pending []string
	got := make(chan []string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Read(r, 1024, func(l []byte) bool {
			pending = append(pending, string(l))
			return true
		}, func() bool {
			if len(pending) == 2 {
				got <- pending
				return false
			}
			return true
		})
	}()
	select {
	case lines := <-got:
		if lines[0] != "a" || lines[1] != "b" {
			t.Fatalf("lines %q", lines)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no tick before the end of the output")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// line returning false stops Read even when the writer goes on.
func TestStop(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	go func() {
		for {
			if _, err := io.WriteString(w, "x\n"); err != nil {
				return
			}
		}
	}()
	n := 0
	err := Read(r, 1024, func([]byte) bool {
		n++
		return n < 1000
	}, func() bool { return true })
	if err != nil || n != 1000 {
		t.Fatalf("n %d err %v", n, err)
	}
	r.Close()
}

func TestAll(t *testing.T) {
	r, w := io.Pipe()
	go func() {
		for range 10000 {
			_, _ = io.WriteString(w, "line\n")
		}
		w.Close()
	}()
	n := 0
	if err := Read(r, 1024, func(l []byte) bool {
		if string(l) == "line" {
			n++
		}
		return true
	}, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if n != 10000 {
		t.Fatalf("n %d", n)
	}
}
