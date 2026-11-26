package prompt

import (
	"errors"
	"fmt"
	"io"
)

var ErrNoTerminal = errors.New("no terminal")

type RefusalError struct {
	Flag string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("no terminal for a needed answer; add %s", e.Flag)
}

type Asker struct {
	Open func() (io.ReadWriteCloser, error)
}

func NewAsker() Asker {
	return Asker{Open: func() (io.ReadWriteCloser, error) { return nil, errors.New("not implemented") }}
}

func (a Asker) YesNo(question string) (bool, error) {
	return false, errors.New("not implemented")
}

func ResolveYesNo(a Asker, flagValue, stored, question, flagName string) (string, error) {
	return "", errors.New("not implemented")
}
