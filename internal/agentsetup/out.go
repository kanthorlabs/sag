package agentsetup

import (
	"io"
	"os"
)

var agentOut io.Writer = os.Stdout

func SetOut(w io.Writer) {
	agentOut = w
}
