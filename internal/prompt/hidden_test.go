package prompt

import (
	"os"
	"testing"
)

func TestReadHiddenNonTTYErrors(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w.Close()
	_, gotErr := ReadHidden(r, "prompt: ")
	if gotErr == nil {
		t.Fatal("ReadHidden on a pipe: want error, got nil")
	}
}
