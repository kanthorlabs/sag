package token

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
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

const (
	keySize       = 32
	maxNameLength = 64
)

const (
	claimSub  = "sub"
	claimName = "name"
	claimKind = "kind"
	claimIat  = "iat"
	claimExp  = "exp"
	claimJTI  = "jti"
)

var requiredClaims = []string{claimSub, claimName, claimKind, claimIat, claimExp, claimJTI}

var subPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

var (
	errInvalidKey      = errors.New("token: key must be 32 bytes")
	errInvalidSub      = errors.New("token: sub must match ^[a-z][a-z0-9-]{0,62}$")
	errInvalidName     = errors.New("token: name must be a nonblank string of 1 to 64 characters")
	errInvalidLifetime = errors.New("token: lifetime must be at least one second")
)

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

func validSub(sub string) bool {
	return subPattern.MatchString(sub)
}

func ValidSub(sub string) bool {
	return validSub(sub)
}

func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= maxNameLength
}

func Issue(key []byte, sub, name string, now time.Time, lifetime time.Duration) (string, error) {
	if len(key) != keySize {
		return "", errInvalidKey
	}
	if !validSub(sub) {
		return "", errInvalidSub
	}
	if !validName(name) {
		return "", errInvalidName
	}
	seconds := int64(lifetime / time.Second)
	if seconds < 1 {
		return "", errInvalidLifetime
	}
	jti, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return "", err
	}
	iat := now.Unix()
	claims := jwt.MapClaims{
		claimSub:  sub,
		claimName: name,
		claimKind: Kind,
		claimIat:  iat,
		claimExp:  iat + seconds,
		claimJTI:  jti.String(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
}

func Verify(key []byte, raw string, now time.Time) (Claims, error) {
	if len(key) != keySize {
		return Claims{}, refuse(CauseUnauthenticated)
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithoutClaimsValidation(),
		jwt.WithJSONNumber(),
		jwt.WithStrictDecoding(),
	)
	rawClaims := jwt.MapClaims{}
	if _, err := parser.ParseWithClaims(raw, rawClaims, func(*jwt.Token) (any, error) { return key, nil }); err != nil {
		return Claims{}, refuse(CauseUnauthenticated)
	}
	for name := range rawClaims {
		if !slices.Contains(requiredClaims, name) {
			return Claims{}, refuse(CauseUnknownClaim)
		}
	}
	claims, ok := decodeClaims(rawClaims)
	if !ok {
		return Claims{}, refuse(CauseInvalidType)
	}
	for _, name := range requiredClaims {
		if _, present := rawClaims[name]; !present {
			return Claims{}, refuse(CauseMissingClaim)
		}
	}
	if !claims.ExpiresAt.After(now) {
		return Claims{}, refuse(CauseExpired)
	}
	if claims.IssuedAt.After(now) {
		return Claims{}, refuse(CauseNotYetIssued)
	}
	return claims, nil
}

func refuse(cause Cause) *Error {
	return &Error{Cause: cause}
}

func decodeClaims(raw jwt.MapClaims) (Claims, bool) {
	var claims Claims
	for name, value := range raw {
		var ok bool
		switch name {
		case claimSub:
			claims.Sub, ok = value.(string)
			ok = ok && validSub(claims.Sub)
		case claimName:
			claims.Name, ok = value.(string)
			ok = ok && validName(claims.Name)
		case claimKind:
			claims.Kind, ok = value.(string)
			ok = ok && claims.Kind == Kind
		case claimIat:
			claims.IssuedAt, ok = unixSeconds(value)
		case claimExp:
			claims.ExpiresAt, ok = unixSeconds(value)
		case claimJTI:
			claims.JTI, ok = value.(string)
			ok = ok && validJTI(claims.JTI)
		}
		if !ok {
			return Claims{}, false
		}
	}
	return claims, true
}

func unixSeconds(value any) (time.Time, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := number.Int64()
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

func validJTI(jti string) bool {
	_, err := ulid.ParseStrict(jti)
	return err == nil
}
