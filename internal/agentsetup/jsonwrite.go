package agentsetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func writeJSON(path string, keyPath []string, value any) error {
	return writeJSONFull(path, keyPath, value, 0)
}

func writeJSONMode(path string, keyPath []string, value any, forcedMode fs.FileMode) error {
	return writeJSONFull(path, keyPath, value, forcedMode)
}

func writeJSONFull(path string, keyPath []string, value any, forcedMode fs.FileMode) error {
	target, err := resolveTarget(path)
	if err != nil {
		return err
	}

	mode := fs.FileMode(0o600)
	if forcedMode != 0 {
		mode = forcedMode
	} else if info, err := os.Stat(target); err == nil {
		mode = info.Mode() & 0o777
	}

	top := map[string]json.RawMessage{}
	data, err := os.ReadFile(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &top); err != nil {
			return err
		}
	}

	if err := setNestedRaw(top, keyPath, value); err != nil {
		return err
	}

	out, err := marshalNoEscape(top)
	if err != nil {
		return err
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".jsonwrite-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, target)
}

func resolveTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		return "", err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return target, nil
}

func setNestedRaw(top map[string]json.RawMessage, keyPath []string, value any) error {
	if len(keyPath) == 0 {
		return errors.New("jsonwrite: empty key path")
	}
	if len(keyPath) == 1 {
		raw, err := marshalNoEscape(value)
		if err != nil {
			return err
		}
		top[keyPath[0]] = raw
		return nil
	}

	var nested map[string]json.RawMessage
	if existing, ok := top[keyPath[0]]; ok && len(existing) > 0 {
		if err := json.Unmarshal(existing, &nested); err != nil {
			return err
		}
	}
	if nested == nil {
		nested = map[string]json.RawMessage{}
	}
	if err := setNestedRaw(nested, keyPath[1:], value); err != nil {
		return err
	}
	raw, err := marshalNoEscape(nested)
	if err != nil {
		return err
	}
	top[keyPath[0]] = raw
	return nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}
