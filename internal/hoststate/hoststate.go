package hoststate

import (
	"errors"
	"os"
)

type Lock struct {
	file *os.File
}

func ResolvePaths() (Paths, error) {
	return Paths{}, errors.New("not implemented")
}

func EnsureCfg(p Paths) (created bool, err error) {
	return false, errors.New("not implemented")
}

func AcquireLock(p Paths) (*Lock, error) {
	return nil, errors.New("not implemented")
}

func (l *Lock) Release() error {
	return errors.New("not implemented")
}

func LoadAnswers(p Paths) (Answers, error) {
	return Answers{}, errors.New("not implemented")
}

func SaveAnswers(p Paths, a Answers) error {
	return errors.New("not implemented")
}

func FindPodman(stored string) (string, error) {
	return "", errors.New("not implemented")
}
