package sql

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestValueEncoding(t *testing.T) {
	const maxSafe = int64(1)<<53 - 1
	ts := time.Date(2026, 11, 25, 12, 30, 45, 123456000, time.FixedZone("x", 7*3600))
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"null", nil, `null`},
		{"bool", true, `true`},
		{"small int", int64(42), `42`},
		{"negative int", int64(-7), `-7`},
		{"max safe int", maxSafe, `9007199254740991`},
		{"min safe int", -maxSafe, `-9007199254740991`},
		{"above safe int", maxSafe + 1, `"9007199254740992"`},
		{"below safe int", -maxSafe - 1, `"-9007199254740992"`},
		{"max int64", int64(math.MaxInt64), `"9223372036854775807"`},
		{"float", 1.5, `1.5`},
		{"NaN", math.NaN(), `"NaN"`},
		{"infinity", math.Inf(1), `"Infinity"`},
		{"negative infinity", math.Inf(-1), `"-Infinity"`},
		{"numeric text", "12345678901234567890.123", `"12345678901234567890.123"`},
		{"text", "a\"b", `"a\"b"`},
		{"binary", []byte{1, 2, 3}, `"<binary 3 bytes>"`},
		{"empty binary", []byte{}, `"<binary 0 bytes>"`},
		{"time", ts, `"2026-11-25T12:30:45.123456+07:00"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(encodeValue(tc.in)); err != nil {
				t.Fatal(err)
			}
			raw := strings.TrimSuffix(buf.String(), "\n")
			if raw != tc.want {
				t.Fatalf("encode(%v) = %s, want %s", tc.in, raw, tc.want)
			}
		})
	}
}
