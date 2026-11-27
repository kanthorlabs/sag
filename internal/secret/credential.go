package secret

import (
	"fmt"
	"io"
	"log/slog"
)

const redacted = "[REDACTED]"

type sealed struct {
	value []byte
}

type Credential struct {
	name   string
	sealed *sealed
}

func NewCredential(name string, value []byte) Credential {
	return Credential{name: name, sealed: &sealed{value: value}}
}

func (c Credential) Name() string {
	return c.name
}

func (c Credential) Reveal() []byte {
	if c.sealed == nil {
		return nil
	}
	return c.sealed.value
}

func (c Credential) String() string {
	return redacted
}

func (c Credential) GoString() string {
	return redacted
}

func (c Credential) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, redacted)
}

func (c Credential) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

func (c Credential) LogValue() slog.Value {
	return slog.StringValue(redacted)
}
