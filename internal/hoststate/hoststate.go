package hoststate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type Lock struct {
	file *os.File
}

func ResolvePaths() (Paths, error) {
	var base string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		base = xdg
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		base = filepath.Join(home, ".config")
	}
	cfg := filepath.Join(base, "sag")
	return Paths{
		Cfg:         cfg,
		SAGYaml:     filepath.Join(cfg, "sag.yaml"),
		CAPem:       filepath.Join(cfg, "ca.pem"),
		AnswersJSON: filepath.Join(cfg, "answers.json"),
		Lock:        filepath.Join(cfg, "lock"),
	}, nil
}

func EnsureCfg(p Paths) (created bool, err error) {
	_, err = os.Stat(p.Cfg)
	if err == nil {
		if err := os.Chmod(p.Cfg, 0o700); err != nil {
			return false, err
		}
		return false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(p.Cfg, 0o700); err != nil {
		return false, err
	}
	return true, nil
}

func AcquireLock(p Paths) (*Lock, error) {
	f, err := os.OpenFile(p.Lock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another process holds the lock; try again later")
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Release() error {
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		l.file.Close()
		return err
	}
	return l.file.Close()
}

func LoadAnswers(p Paths) (Answers, error) {
	raw, err := os.ReadFile(p.AnswersJSON)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Answers{}, nil
		}
		return Answers{}, err
	}
	var a Answers
	if err := json.Unmarshal(raw, &a); err != nil {
		return Answers{}, err
	}
	return a, nil
}

func SaveAnswers(p Paths, a Answers) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tmp := p.AnswersJSON + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.AnswersJSON)
}

var podmanFixedPaths = []string{
	"/opt/podman/bin/podman",
	"/opt/homebrew/bin/podman",
}

func FindPodman(stored string) (string, error) {
	if stored != "" {
		if _, err := os.Stat(stored); err == nil {
			return stored, nil
		}
	}
	if p, err := exec.LookPath("podman"); err == nil {
		return p, nil
	}
	for _, candidate := range podmanFixedPaths {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("podman not found")
}
