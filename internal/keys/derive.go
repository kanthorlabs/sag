package keys

import (
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"strconv"
)

const (
	masterSize    = 32
	keySize       = 32
	jwtLabel      = "sag/jwt-hs256/v"
	approvalLabel = "sag/approval-hmac/v1"
)

func JWTKey(master []byte, version int) ([]byte, error) {
	if version < 1 {
		return nil, errors.New("keys: auth.version must be a positive integer")
	}
	return derive(master, jwtLabel+strconv.Itoa(version))
}

func ApprovalKey(master []byte) ([]byte, error) {
	return derive(master, approvalLabel)
}

func derive(master []byte, info string) ([]byte, error) {
	if len(master) != masterSize {
		return nil, errors.New("keys: master key must be 32 bytes")
	}
	return hkdf.Key(sha256.New, master, nil, info, keySize)
}
