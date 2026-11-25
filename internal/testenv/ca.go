package testenv

import "errors"

type CA struct {
	CertPEM []byte
}

func NewCA() (*CA, error) {
	return nil, errors.ErrUnsupported
}

func (c *CA) Issue(hosts ...string) (certPEM, keyPEM []byte, err error) {
	return nil, nil, errors.ErrUnsupported
}
