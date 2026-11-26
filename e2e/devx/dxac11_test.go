//go:build devxe2e

package devx_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func init() {
	credentialTests["DXAC11_CreateRejectsForm"] = func(t *testing.T, ctx context.Context) {
		DXAC11_CreateRejectsForm(t, ctx)
	}
}

func DXAC11_CreateRejectsForm(t *testing.T, ctx context.Context) {
	t.Helper()
	dxh.EnsureCanary(t)

	uid := fmt.Sprintf("%d", os.Getuid())
	if err := waitLaunchAgentIdle(ctx, uid); err != nil {
		t.Fatalf("DXAC11: wait launch agent idle: %v", err)
	}

	canary := dxh.ActiveCanaryParts
	r := dxh.NewRunner()

	port, err := canaryPGPort(ctx, r)
	if err != nil {
		t.Fatalf("DXAC11: get canary PG port: %v", err)
	}

	baseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(canary.User, canary.Password),
		Host:   "host.containers.internal:" + port,
		Path:   "/" + canary.Database,
	}

	ac11Case := func(t *testing.T, name, dsn, wantMsg string) {
		t.Helper()
		secretsBefore := ac11SecretNames(ctx, t, r)
		code, combined := runSAGWithInput(t, ctx, []byte(dsn),
			"credential", "create", name,
			"--kind", "postgres",
			"--agent", "pi",
			"--tls", "disable",
			"--stdin",
		)
		if code != 1 {
			t.Fatalf("%s: exit %d; want 1", t.Name(), code)
		}
		if !strings.Contains(combined, wantMsg) {
			t.Fatalf("%s: output does not contain %q", t.Name(), wantMsg)
		}
		for _, part := range []string{canary.Password, canary.Host, canary.User, canary.Database} {
			if strings.Contains(combined, part) {
				t.Fatalf("%s: output contains canary part", t.Name())
			}
		}
		secretsAfter := ac11SecretNames(ctx, t, r)
		for _, sn := range secretsAfter {
			found := false
			for _, sb := range secretsBefore {
				if sb == sn {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s: new secret %s created after failure", t.Name(), sn)
			}
		}
	}

	t.Run("sslmode_query", func(t *testing.T) {
		u := *baseURL
		u.RawQuery = "sslmode=disable"
		ac11Case(t, "dxac11a", u.String(), "query parameters")
	})

	t.Run("two_hosts", func(t *testing.T) {
		u := *baseURL
		twoHostDSN := u.Scheme + "://" + url.UserPassword(canary.User, canary.Password).String() +
			"@host.containers.internal:" + port + ",127.0.0.1:" + port + "/" + canary.Database
		ac11Case(t, "dxac11b", twoHostDSN, "more than one host")
	})

	t.Run("no_database", func(t *testing.T) {
		u := *baseURL
		u.Path = "/"
		ac11Case(t, "dxac11c", u.String(), "postgres URL")
	})

	t.Run("invalid_name", func(t *testing.T) {
		code, combined := runSAGWithInput(t, ctx, []byte(baseURL.String()),
			"credential", "create", "Bad_Name",
			"--kind", "postgres",
			"--agent", "pi",
			"--tls", "disable",
			"--stdin",
		)
		if code != 1 {
			t.Fatalf("invalid_name: exit %d; want 1", code)
		}
		if !strings.Contains(combined, "invalid name") {
			t.Fatalf("invalid_name: output does not contain 'invalid name'")
		}
	})

	t.Run("name_in_use", func(t *testing.T) {
		ac11Case(t, "canary", baseURL.String(), "resource canary exists; delete it first")
	})
}

func ac11SecretNames(ctx context.Context, t *testing.T, r dxh.Runner) []string {
	t.Helper()
	out, err := r.Output(ctx, "podman", "secret", "ls", "--format", "{{.Name}}")
	if err != nil {
		t.Fatalf("ac11SecretNames: podman secret ls: %v", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}
