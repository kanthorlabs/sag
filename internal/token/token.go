package token

import (
	"errors"
	"time"
)

type Cause string

const (
	CauseUnauthenticated Cause = "unauthenticated"
	CauseUnknownClaim    Cause = "unknown_claim"
	CauseInvalidType     Cause = "invalid_type"
	CauseMissingClaim    Cause = "missing_claim"
	CauseExpired         Cause = "expired"
	CauseNotYetIssued    Cause = "not_yet_issued"
)

const Kind = "agent"

type Claims struct {
	Sub       string
	Name      string
	Kind      string
	IssuedAt  time.Time
	ExpiresAt time.Time
	JTI       string
}

type Error struct {
	Cause Cause
}

func (e *Error) Error() string {
	return "token: " + string(e.Cause)
}

func Issue(key []byte, sub, name string, now time.Time, lifetime time.Duration) (string, error) {
	return "", errors.ErrUnsupported
}

func Verify(key []byte, raw string, now time.Time) (Claims, error) {
	return Claims{}, errors.ErrUnsupported
}
