package dxh

import (
	"syscall"
	"time"
)

func ctimeOf(st *syscall.Stat_t) time.Time {
	return time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec)
}
