package dxh

import (
	"context"
	"errors"
)

var ErrUserRunNotImplemented = errors.New("dxh: user run guard not implemented")

type UserSnapshot struct {
	Dir    string
	Absent map[string]bool
}

func CheckNoClaude(ctx context.Context) error {
	return ErrUserRunNotImplemented
}

func Snapshot(dir string) (UserSnapshot, error) {
	return UserSnapshot{}, ErrUserRunNotImplemented
}

func CompareSAGEntries(s UserSnapshot) error {
	return ErrUserRunNotImplemented
}
