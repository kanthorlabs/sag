package secret

import "log/slog"

const redacted = "[REDACTED]"

type Credential struct {
	name  string
	value []byte
}

func NewCredential(name string, value []byte) Credential {
	return Credential{name: name, value: value}
}

func (c Credential) Name() string {
	return c.name
}

func (c Credential) Reveal() []byte {
	return c.value
}

func (c Credential) String() string {
	return redacted
}

func (c Credential) GoString() string {
	return redacted
}

func (c Credential) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

func (c Credential) LogValue() slog.Value {
	return slog.StringValue(redacted)
}
