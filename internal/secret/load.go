package secret

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const (
	Dir           = "/run/secrets"
	ContainerUID  = 65532
	MasterKeyName = "sag-master-key"
	MasterKeySize = 32
)

const requiredMode fs.FileMode = 0o400

const modeBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

func Load(dir string, uid int, names []string) (map[string]Credential, error) {
	creds := make(map[string]Credential, len(names))
	for _, name := range names {
		if _, done := creds[name]; done {
			continue
		}
		value, err := read(dir, name, uid)
		if err != nil {
			return nil, err
		}
		creds[name] = NewCredential(name, value)
	}
	return creds, nil
}

func CheckFile(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("secret %s: %w", path, err)
	}
	return checkInfo(path, info, uid)
}

func DecodeMasterKey(c Credential) ([]byte, error) {
	text := bytes.TrimSpace(c.Reveal())
	key := make([]byte, base64.StdEncoding.DecodedLen(len(text)))
	n, err := base64.StdEncoding.Strict().Decode(key, text)
	if err != nil {
		return nil, fmt.Errorf("secret %s: value is not valid base64", c.Name())
	}
	if n != MasterKeySize {
		return nil, fmt.Errorf("secret %s: value decodes to %d bytes, want %d", c.Name(), n, MasterKeySize)
	}
	return key[:n], nil
}

func read(dir, name string, uid int) ([]byte, error) {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) {
		return nil, fmt.Errorf("secret name %q is not a file name", name)
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("secret %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("secret %s: %w", path, err)
	}
	if err := checkInfo(path, info, uid); err != nil {
		return nil, err
	}
	value, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("secret %s: %w", path, err)
	}
	return value, nil
}

func checkInfo(path string, info fs.FileInfo, uid int) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("secret %s: not a regular file", path)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("secret %s: owner unknown", path)
	}
	if uid < 0 || st.Uid != uint32(uid) {
		return fmt.Errorf("secret %s: owner uid %d, want %d", path, st.Uid, uid)
	}
	if mode := info.Mode() & modeBits; mode != requiredMode {
		return fmt.Errorf("secret %s: mode %v, want %v", path, mode, requiredMode)
	}
	return nil
}
