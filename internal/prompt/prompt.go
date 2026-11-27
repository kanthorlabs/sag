package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
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
	return Asker{Open: func() (io.ReadWriteCloser, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}}
}

func (a Asker) YesNo(question string) (bool, error) {
	rwc, err := a.Open()
	if err != nil {
		return false, ErrNoTerminal
	}
	defer rwc.Close()
	fmt.Fprint(rwc, question)
	buf := bufio.NewReader(rwc)
	line, err := buf.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return answer == "y" || answer == "yes", nil
}

func ResolveYesNo(a Asker, flagValue, stored, question, flagName string) (string, error) {
	if flagValue == "yes" || flagValue == "no" {
		return flagValue, nil
	}
	if stored == "yes" || stored == "no" {
		return stored, nil
	}
	ok, err := a.YesNo(question)
	if errors.Is(err, ErrNoTerminal) {
		return "", &RefusalError{Flag: flagName}
	}
	if err != nil {
		return "", err
	}
	if ok {
		return "yes", nil
	}
	return "no", nil
}
