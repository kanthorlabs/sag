package daemon

import (
	"os"
	"strings"
)

func removePGEnv() error {
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "PG") {
			if err := os.Unsetenv(key); err != nil {
				return err
			}
		}
	}
	return nil
}
