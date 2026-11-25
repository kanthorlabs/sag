package sql

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

const maxSafeInteger = int64(1)<<53 - 1

func encodeValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int64:
		if x > maxSafeInteger || x < -maxSafeInteger {
			return strconv.FormatInt(x, 10)
		}
		return x
	case float64:
		return encodeFloat(x)
	case string:
		return x
	case []byte:
		return fmt.Sprintf("<binary %d bytes>", len(x))
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(x)
	}
}

func encodeFloat(x float64) any {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	default:
		return x
	}
}
