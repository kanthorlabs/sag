package audit

import "os"

type FileWriter struct {
	dir string
}

func NewFileWriter(dir string) (*FileWriter, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &FileWriter{dir: dir}, nil
}

func (w *FileWriter) Append(Record) error {
	return nil
}
