package postexec

import "github.com/kanthorlabs/sag/internal/core"

func Apply(res core.Result, _ core.Limits, _ []string) core.Result {
	return res
}
