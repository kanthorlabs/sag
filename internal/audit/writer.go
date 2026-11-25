package audit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const fileDateFormat = "2006-01-02"

type FileWriter struct {
	dir string
	mu  sync.Mutex
}

func NewFileWriter(dir string) (*FileWriter, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileWriter{dir: dir}, nil
}

func (w *FileWriter) Append(rec Record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	name := filepath.Join(w.dir, "audit-"+time.Time(rec.TS).UTC().Format(fileDateFormat)+".jsonl")

	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(line)
	return errors.Join(err, f.Close())
}
