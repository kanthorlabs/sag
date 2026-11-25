package approval

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

const (
	DirName        = "approvals"
	recordMode     = fs.FileMode(0o600)
	dirMode        = fs.FileMode(0o700)
	keySize        = 32
	maxRecordBytes = 1 << 16
)

var (
	ErrNotApproved         = errors.New("approval: no approval record")
	ErrInvalidMAC          = errors.New("approval: record mac is not valid")
	ErrFingerprintMismatch = errors.New("approval: fingerprint does not match the current objects")

	errKeySize     = errors.New("approval: the approval key must be 32 bytes")
	errName        = errors.New("approval: resource name is not valid")
	errFingerprint = errors.New("approval: fingerprint must be 64 lowercase hex digits")
	errStoreDir    = errors.New("approval: store path is not a directory")
	errRecordFile  = errors.New("approval: record must be a regular file with mode 0600 that the process user owns")
	errRecordSize  = errors.New("approval: record is too large")
)

var (
	namePattern        = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Record struct {
	Resource    string    `json:"resource"`
	Fingerprint string    `json:"fingerprint"`
	Objects     Objects   `json:"objects"`
	ApprovedAt  time.Time `json:"approved_at"`
	MAC         string    `json:"mac"`
}

type macInput struct {
	Resource    string    `json:"resource"`
	Fingerprint string    `json:"fingerprint"`
	Objects     Objects   `json:"objects"`
	ApprovedAt  time.Time `json:"approved_at"`
}

type Store struct {
	dir string
	key []byte
}

func NewStore(dir string, key []byte) *Store {
	return &Store{dir: dir, key: key}
}

func (s *Store) Save(resource, fingerprint string, o Objects, now time.Time) (Record, error) {
	if len(s.key) != keySize {
		return Record{}, errKeySize
	}
	path, err := s.path(resource)
	if err != nil {
		return Record{}, err
	}
	if !fingerprintPattern.MatchString(fingerprint) {
		return Record{}, errFingerprint
	}
	rec := Record{Resource: resource, Fingerprint: fingerprint, Objects: canonical(o), ApprovedAt: now.UTC().Round(0)}
	mac, err := s.mac(rec)
	if err != nil {
		return Record{}, err
	}
	rec.MAC = hex.EncodeToString(mac)
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return Record{}, err
	}
	if err := s.checkDir(); err != nil {
		return Record{}, err
	}
	if err := writeFile(s.dir, path, append(data, '\n')); err != nil {
		return Record{}, err
	}
	return rec, nil
}

func (s *Store) Verify(resource, fingerprint string) error {
	if len(s.key) != keySize {
		return errKeySize
	}
	path, err := s.path(resource)
	if err != nil {
		return err
	}
	if err := s.checkDir(); err != nil {
		return err
	}
	data, err := readRecord(path)
	if err != nil {
		return err
	}
	rec, err := decodeRecord(data)
	if err != nil {
		return err
	}
	got, err := hex.DecodeString(rec.MAC)
	if err != nil || len(got) != sha256.Size {
		return ErrInvalidMAC
	}
	want, err := s.mac(rec)
	if err != nil {
		return err
	}
	if !hmac.Equal(got, want) {
		return ErrInvalidMAC
	}
	if rec.Resource != resource {
		return fmt.Errorf("%w: record belongs to another resource", ErrFingerprintMismatch)
	}
	if subtle.ConstantTimeCompare([]byte(rec.Fingerprint), []byte(fingerprint)) != 1 {
		return ErrFingerprintMismatch
	}
	return nil
}

func (s *Store) Revoke(resource string) error {
	path, err := s.path(resource)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotApproved
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errRecordFile
	}
	return os.Remove(path)
}

func (s *Store) path(resource string) (string, error) {
	if !namePattern.MatchString(resource) {
		return "", errName
	}
	return filepath.Join(s.dir, resource+".json"), nil
}

func (s *Store) checkDir() error {
	info, err := os.Lstat(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotApproved
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errStoreDir
	}
	return nil
}

func (s *Store) mac(rec Record) ([]byte, error) {
	msg, err := json.Marshal(macInput{
		Resource:    rec.Resource,
		Fingerprint: rec.Fingerprint,
		Objects:     rec.Objects,
		ApprovedAt:  rec.ApprovedAt,
	})
	if err != nil {
		return nil, err
	}
	h := hmac.New(sha256.New, s.key)
	h.Write(msg)
	return h.Sum(nil), nil
}

func decodeRecord(data []byte) (Record, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var rec Record
	if err := dec.Decode(&rec); err != nil {
		return Record{}, fmt.Errorf("%w: record does not decode", ErrInvalidMAC)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Record{}, fmt.Errorf("%w: record has trailing data", ErrInvalidMAC)
	}
	return rec, nil
}

func readRecord(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotApproved
	}
	if errors.Is(err, syscall.ELOOP) {
		return nil, errRecordFile
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !ownRegularFile(info) {
		return nil, errRecordFile
	}
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecordBytes {
		return nil, errRecordSize
	}
	return data, nil
}

func ownRegularFile(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && info.Mode().Perm() == recordMode && int(st.Uid) == os.Geteuid()
}

func writeFile(dir, path string, data []byte) error {
	f, err := os.CreateTemp(dir, ".record-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(recordMode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
