package preflight

import "time"

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

func SystemClock() Clock {
	return nil
}
