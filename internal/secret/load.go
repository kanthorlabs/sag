package secret

import "errors"

const (
	Dir           = "/run/secrets"
	ContainerUID  = 65532
	MasterKeyName = "sag-master-key"
	MasterKeySize = 32
)

func Load(dir string, uid int, names []string) (map[string]Credential, error) {
	return nil, errors.ErrUnsupported
}

func CheckFile(path string, uid int) error {
	return errors.ErrUnsupported
}

func DecodeMasterKey(c Credential) ([]byte, error) {
	return nil, errors.ErrUnsupported
}
