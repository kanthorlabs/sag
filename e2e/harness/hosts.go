package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

func (s *Suite) postgresHosts(ctx context.Context) ([]string, error) {
	hosts := make([]string, 0, len(s.pgs))
	for _, pg := range s.pgs {
		res, err := podman(ctx, "container", "inspect", pg.Container)
		if err != nil {
			return nil, err
		}
		ip, err := networkIP([]byte(res.Stdout), s.run.Network)
		if err != nil {
			return nil, fmt.Errorf("harness: %s: %w", pg.Container, err)
		}
		hosts = append(hosts, pg.Container+":"+ip)
	}
	sort.Strings(hosts)
	return hosts, nil
}

func networkIP(inspect []byte, network string) (string, error) {
	var items []struct {
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(inspect, &items); err != nil {
		return "", err
	}
	if len(items) != 1 {
		return "", fmt.Errorf("inspect returned %d items", len(items))
	}
	n, ok := items[0].NetworkSettings.Networks[network]
	if !ok || n.IPAddress == "" {
		return "", fmt.Errorf("no IP on network %s", network)
	}
	return n.IPAddress, nil
}
