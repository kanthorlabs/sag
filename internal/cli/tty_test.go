package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/creack/pty"
)

func TestIsTerminal(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open pseudo-terminal: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()
	if !isTerminal(tty) {
		t.Fatal("isTerminal(pty) = false; want true")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Fatal("isTerminal(pipe) = true; want false")
	}

	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Fatal("isTerminal(file) = true; want false")
	}

	if isTerminal(&bytes.Buffer{}) {
		t.Fatal("isTerminal(buffer) = true; want false")
	}
	if isTerminal(nil) {
		t.Fatal("isTerminal(nil) = true; want false")
	}

	devNullR, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open /dev/null for read: %v", err)
	}
	defer devNullR.Close()
	if isTerminal(devNullR) {
		t.Fatal("isTerminal(/dev/null read) = true; want false")
	}

	devNullW, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open /dev/null for write: %v", err)
	}
	defer devNullW.Close()
	if isTerminal(devNullW) {
		t.Fatal("isTerminal(/dev/null write) = true; want false")
	}
}
