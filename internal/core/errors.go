package core

import "fmt"

type Code string

const (
	CodeUnauthenticated     Code = "unauthenticated"
	CodeNotFound            Code = "not_found"
	CodeInvalidArgument     Code = "invalid_argument"
	CodeQueryRejected       Code = "query_rejected"
	CodeTimeout             Code = "timeout"
	CodeRateLimited         Code = "rate_limited"
	CodeResourceUnavailable Code = "resource_unavailable"
	CodeUpstreamError       Code = "upstream_error"
	CodeInternal            Code = "internal"
)

type Error struct {
	Code    Code
	Message string
	Trace   Trace
	Cause   error
}

func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}
