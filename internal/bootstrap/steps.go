package bootstrap

import "errors"

var errNotImplemented = errors.New("not implemented")

type Step struct {
	Name string
	Run  func() error
}

var Steps = []Step{
	{Name: "platform-check", Run: func() error { return errNotImplemented }},
	{Name: "select-pins", Run: func() error { return errNotImplemented }},
	{Name: "plan", Run: func() error { return errNotImplemented }},
	{Name: "lock", Run: func() error { return errNotImplemented }},
	{Name: "plan-again", Run: func() error { return errNotImplemented }},
	{Name: "deps", Run: func() error { return errNotImplemented }},
	{Name: "image", Run: func() error { return errNotImplemented }},
	{Name: "master-key", Run: func() error { return errNotImplemented }},
	{Name: "cert", Run: func() error { return errNotImplemented }},
	{Name: "sag-yaml", Run: func() error { return errNotImplemented }},
	{Name: "autostart", Run: func() error { return errNotImplemented }},
	{Name: "service-start", Run: func() error { return errNotImplemented }},
	{Name: "tailscale", Run: func() error { return errNotImplemented }},
	{Name: "agent-setup", Run: func() error { return errNotImplemented }},
	{Name: "print-endpoint", Run: func() error { return errNotImplemented }},
}
