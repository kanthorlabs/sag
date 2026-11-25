package audit

import (
	"time"
)

const (
	DirName    = "audit"
	TimeFormat = "2006-01-02T15:04:05.000Z"

	ExtraJTI        = "jti"
	ExtraRemoteAddr = "remote_addr"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type Timestamp time.Time

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format(TimeFormat) + `"`), nil
}

type Record struct {
	TS             Timestamp `json:"ts"`
	RequestID      string    `json:"request_id"`
	RemoteAddr     string    `json:"remote_addr"`
	Sub            *string   `json:"sub"`
	JTI            *string   `json:"jti"`
	AuthCause      *string   `json:"auth_cause"`
	Resource       string    `json:"resource"`
	Capability     string    `json:"capability"`
	Operation      string    `json:"operation"`
	SQL            string    `json:"sql"`
	SQLSHA256      string    `json:"sql_sha256"`
	RenderedSHA256 string    `json:"rendered_sha256"`
	Decision       Decision  `json:"decision"`
	Reason         string    `json:"reason"`
	Rows           int       `json:"rows"`
	Bytes          int       `json:"bytes"`
	Truncated      bool      `json:"truncated"`
	DurationMS     int64     `json:"duration_ms"`
}

type Writer interface {
	Append(rec Record) error
}

func AuthFailure(ts time.Time, requestID, remoteAddr, cause string) Record {
	return Record{
		TS:         Timestamp(ts),
		RequestID:  requestID,
		RemoteAddr: remoteAddr,
		AuthCause:  &cause,
		Decision:   DecisionDeny,
		Reason:     "unauthenticated",
	}
}
