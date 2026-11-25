package keys

import "errors"

func JWTKey(master []byte, version int) ([]byte, error) {
	return nil, errors.ErrUnsupported
}

func ApprovalKey(master []byte) ([]byte, error) {
	return nil, errors.ErrUnsupported
}
