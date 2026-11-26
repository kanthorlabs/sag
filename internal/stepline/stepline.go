package stepline

import (
	"fmt"
	"io"
)

const (
	StatusOK      = "ok"
	StatusWarn    = "warn"
	StatusInstall = "install"
	StatusSkip    = "skip"
	StatusFail    = "fail"
)

type Writer struct {
	w io.Writer
}

func New(w io.Writer) Writer {
	return Writer{w: w}
}

func (wr Writer) Line(status, item, detail string) {
	if detail != "" {
		fmt.Fprintf(wr.w, "%s %s %s\n", status, item, detail)
		return
	}
	fmt.Fprintf(wr.w, "%s %s\n", status, item)
}
