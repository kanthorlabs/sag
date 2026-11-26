package dxh_test

import (
	"context"
	"fmt"
	"strings"
)

type fakeRunner struct {
	outputs map[string][]byte
	errors  map[string]error
	calls   [][]string
}

func newFake() *fakeRunner {
	return &fakeRunner{
		outputs: map[string][]byte{},
		errors:  map[string]error{},
	}
}

func (f *fakeRunner) set(cmd string, out []byte, err error) {
	f.outputs[cmd] = out
	if err != nil {
		f.errors[cmd] = err
	} else {
		delete(f.errors, cmd)
	}
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	key := strings.Join(call, " ")
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	out, ok := f.outputs[key]
	if !ok {
		return nil, fmt.Errorf("fakeRunner: unregistered call: %s", key)
	}
	return out, nil
}
