package pipeline

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/kanthorlabs/sag/internal/config"
	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/inventory"
)

const (
	idleTimeoutMargin = 5 * time.Second

	keyMaxRows        = "max_rows"
	keyMaxResultBytes = "max_result_bytes"
	keyTimeoutSeconds = "timeout_seconds"

	messageArgumentsNotObject = "arguments must be a JSON object"
	messageUnknownOperation   = "unknown operation"
	messageUnknownKey         = "arguments hold an unknown key"
	messageNULByte            = "arguments hold a NUL byte"
)

func resolveLimits(entry inventory.Entry, req Request) (core.Limits, error) {
	args, err := decodeArguments(req.Input)
	if err != nil {
		return core.Limits{}, err
	}
	known, err := operationKeys(entry.Kind, req.Capability, req.Operation)
	if err != nil {
		return core.Limits{}, err
	}
	for key := range args {
		if _, ok := known[key]; !ok {
			return core.Limits{}, core.NewError(core.CodeInvalidArgument, messageUnknownKey)
		}
	}
	hard := config.HardMaximums()
	lim := entry.Limits
	if err := requestValue(args, keyMaxRows, hard.MaxRows, &lim.MaxRows); err != nil {
		return core.Limits{}, err
	}
	if err := requestValue(args, keyMaxResultBytes, hard.MaxResultBytes, &lim.MaxResultBytes); err != nil {
		return core.Limits{}, err
	}
	seconds := 0
	if err := requestValue(args, keyTimeoutSeconds, int(hard.StatementTimeout/time.Second), &seconds); err != nil {
		return core.Limits{}, err
	}
	if seconds > 0 {
		lim.Timeout = time.Duration(seconds) * time.Second
	}
	lim.IdleTimeout = min(max(entry.Limits.IdleTimeout, lim.Timeout+idleTimeoutMargin), hard.IdleInTransactionSessionTimeout)
	return lim, nil
}

func decodeArguments(input json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var args map[string]any
	if err := dec.Decode(&args); err != nil || args == nil {
		return nil, core.NewError(core.CodeInvalidArgument, messageArgumentsNotObject)
	}
	if holdsNUL(args) {
		return nil, core.NewError(core.CodeInvalidArgument, messageNULByte)
	}
	return args, nil
}

func holdsNUL(v any) bool {
	switch v := v.(type) {
	case string:
		return strings.IndexByte(v, 0) >= 0
	case map[string]any:
		for k, item := range v {
			if holdsNUL(k) || holdsNUL(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if holdsNUL(item) {
				return true
			}
		}
	}
	return false
}

func operationKeys(kind, capability, operation string) (map[string]json.RawMessage, error) {
	if adapter, ok := core.Lookup(kind); ok {
		for _, c := range adapter.Capabilities() {
			if c.Name != capability {
				continue
			}
			for _, op := range c.Operations {
				if op.Name != operation {
					continue
				}
				var schema struct {
					Properties map[string]json.RawMessage `json:"properties"`
				}
				if err := json.Unmarshal(op.InputSchema, &schema); err != nil {
					return nil, err
				}
				return schema.Properties, nil
			}
		}
	}
	return nil, core.NewError(core.CodeInvalidArgument, messageUnknownOperation)
}

func requestValue(args map[string]any, key string, hardMax int, dst *int) error {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	invalid := core.NewError(core.CodeInvalidArgument, key+" must be an integer from 1 to "+strconv.Itoa(hardMax))
	n, ok := raw.(json.Number)
	if !ok {
		return invalid
	}
	v, err := n.Int64()
	if err != nil || v < 1 || v > int64(hardMax) {
		return invalid
	}
	*dst = int(v)
	return nil
}
