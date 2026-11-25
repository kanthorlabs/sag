package approval

import (
	"errors"
	"time"
)

const DirName = "approvals"

var (
	ErrNotApproved         = errors.New("approval: no approval record")
	ErrInvalidMAC          = errors.New("approval: record mac is not valid")
	ErrFingerprintMismatch = errors.New("approval: fingerprint does not match the current objects")
)

type Record struct {
	Resource    string    `json:"resource"`
	Fingerprint string    `json:"fingerprint"`
	Objects     Objects   `json:"objects"`
	ApprovedAt  time.Time `json:"approved_at"`
	MAC         string    `json:"mac"`
}

type Store struct {
	dir string
	key []byte
}

func NewStore(dir string, key []byte) *Store {
	return &Store{dir: dir, key: key}
}

func (s *Store) Save(resource, fingerprint string, o Objects, now time.Time) (Record, error) {
	return Record{}, errors.ErrUnsupported
}

func (s *Store) Verify(resource, fingerprint string) error {
	return errors.ErrUnsupported
}

func (s *Store) Revoke(resource string) error {
	return errors.ErrUnsupported
}
