package agentsetup

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/kanthorlabs/sag/internal/prompt"
)

func TestMain(m *testing.M) {
	newAsker = func() prompt.Asker {
		return prompt.Asker{Open: func() (io.ReadWriteCloser, error) { return nil, errors.New("no terminal") }}
	}
	os.Exit(m.Run())
}
