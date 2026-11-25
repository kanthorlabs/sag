package core

type TruncatedReason string

const (
	TruncatedNone           TruncatedReason = ""
	TruncatedMaxRows        TruncatedReason = "max_rows"
	TruncatedMaxResultBytes TruncatedReason = "max_result_bytes"
)

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Trace struct {
	Input          string
	InputSHA256    string
	RenderedSHA256 string
}

type Result struct {
	Columns         []Column
	Rows            [][]any
	Truncated       bool
	TruncatedReason TruncatedReason
	Trace           Trace
}
