package pty_test

import (
	"testing"

	"github.com/dhamidi/dmux/internal/pty"
)

// Compile-time assertion: FakePTY must implement PTY.
var _ pty.PTY = (*pty.FakePTY)(nil)

func TestFakePTY_Write_captured_by_Input(t *testing.T) {
	f := &pty.FakePTY{}
	want := []byte("hello pty")
	n, err := f.Write(want)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(want) {
		t.Fatalf("Write returned %d, want %d", n, len(want))
	}
	if got := f.Input(); string(got) != string(want) {
		t.Fatalf("Input: got %q, want %q", got, want)
	}
}

func TestFakePTY_InjectOutput_returned_by_Read(t *testing.T) {
	f := &pty.FakePTY{}
	f.InjectOutput([]byte("child output"))
	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := string(buf[:n]); got != "child output" {
		t.Fatalf("Read: got %q, want %q", got, "child output")
	}
}

func TestFakePTY_Resize_recorded_in_order(t *testing.T) {
	f := &pty.FakePTY{}
	sizes := []pty.Size{
		{Rows: 24, Cols: 80},
		{Rows: 48, Cols: 132},
		{Rows: 10, Cols: 40},
	}
	for _, s := range sizes {
		if err := f.Resize(s.Rows, s.Cols); err != nil {
			t.Fatalf("Resize(%d,%d): %v", s.Rows, s.Cols, err)
		}
	}
	if len(f.Resizes) != len(sizes) {
		t.Fatalf("got %d Resizes, want %d", len(f.Resizes), len(sizes))
	}
	for i, s := range sizes {
		if f.Resizes[i] != s {
			t.Errorf("Resizes[%d] = %v, want %v", i, f.Resizes[i], s)
		}
	}
}

func TestFakePTY_Close_prevents_Write(t *testing.T) {
	f := &pty.FakePTY{}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := f.Write([]byte("x")); err == nil {
		t.Fatal("Write after Close: expected error, got nil")
	}
}

func TestFakePTY_Close_prevents_Resize(t *testing.T) {
	f := &pty.FakePTY{}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := f.Resize(24, 80); err == nil {
		t.Fatal("Resize after Close: expected error, got nil")
	}
}

func TestFakePTY_Close_drains_remaining_output(t *testing.T) {
	f := &pty.FakePTY{}
	f.InjectOutput([]byte("buffered"))
	f.Close()

	buf := make([]byte, 64)
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("Read after Close (non-empty buffer): %v", err)
	}
	if got := string(buf[:n]); got != "buffered" {
		t.Fatalf("Read: got %q, want %q", got, "buffered")
	}
}

func TestFakePTY_Multiple_writes_accumulate(t *testing.T) {
	f := &pty.FakePTY{}
	f.Write([]byte("foo"))
	f.Write([]byte("bar"))
	if got := string(f.Input()); got != "foobar" {
		t.Fatalf("Input: got %q, want %q", got, "foobar")
	}
}

// TestOpen_smoke exercises the real OS PTY path.
// It is skipped in short mode to allow unit test runs without OS PTY support.
func TestOpen_smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping OS PTY smoke test in short mode")
	}
	p, err := pty.Open("echo", []string{"hello"}, pty.Size{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p.Close()
}
