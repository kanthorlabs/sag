package harness

import "net/netip"

var tailnetPrefix = netip.MustParsePrefix("100.64.0.0/10")

func PublishHostAllowed(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is4() {
		return false
	}
	return addr == netip.MustParseAddr(PublishHost) || tailnetPrefix.Contains(addr)
}
