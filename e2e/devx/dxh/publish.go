package dxh

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
)

func PublishAddr(ctx context.Context, r Runner) (string, error) {
	if runtime.GOOS != "linux" {
		return "127.0.0.1", nil
	}
	out, err := r.Output(ctx, "ip", "-4", "route", "get", "1.1.1.1")
	if err != nil {
		return "", errors.New("publish address: ip -4 route get failed")
	}
	return parseRouteSrc(string(out))
}

func parseRouteSrc(out string) (string, error) {
	fields := strings.Fields(out)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "src" {
			continue
		}
		ip := net.ParseIP(fields[i+1])
		if ip == nil || ip.To4() == nil || ip.IsLoopback() {
			return "", errors.New("publish address: route source is not a non-loopback IPv4 address")
		}
		return ip.String(), nil
	}
	return "", errors.New("publish address: no route source")
}
