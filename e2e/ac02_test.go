//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/kanthorlabs/sag/e2e/harness"
	"github.com/kanthorlabs/sag/internal/secret"
)

func init() {
	harness.Register(harness.AC{
		ID:    "AC02",
		Phase: harness.PhaseParallel,
		Run:   runAC02,
	})
}

func runAC02(t *testing.T, env harness.Env) {
	shortKey := make([]byte, 16)
	if _, err := rand.Read(shortKey); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		spec  harness.ContainerSpec
		cause string
	}{
		{
			name:  "mode0444",
			spec:  harness.ContainerSpec{Name: "ac02-mode", SecretMode: "0444"},
			cause: "want -r--------",
		},
		{
			name:  "shortMasterKey",
			spec:  harness.ContainerSpec{Name: "ac02-short-key", MasterKey: shortKey},
			cause: "decodes to 16 bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ac02Refused(t, env, tc.spec, tc.cause, shortKey)
		})
	}
}

func ac02Refused(t *testing.T, env harness.Env, spec harness.ContainerSpec, cause string, shortKey []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c, err := env.StartSAG(ctx, spec)
	if err != nil {
		t.Fatalf("start %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = c.Remove(context.Background())
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWait()
	code, err := c.Wait(waitCtx)
	logs, _ := c.Logs(ctx)
	if err != nil {
		t.Fatalf("%s did not exit: %v\nlogs:\n%s", spec.Name, err, logs)
	}
	if code == 0 {
		t.Fatalf("%s exited 0\nlogs:\n%s", spec.Name, logs)
	}

	probe, cancelProbe := context.WithTimeout(ctx, 5*time.Second)
	defer cancelProbe()
	if err := c.Healthz(probe); err == nil {
		t.Fatalf("%s answered /healthz", spec.Name)
	}

	if !strings.Contains(logs, cause) {
		t.Fatalf("%s logs do not state the cause %q:\n%s", spec.Name, cause, logs)
	}
	for _, value := range []string{
		base64.StdEncoding.EncodeToString(env.MasterKey()),
		base64.StdEncoding.EncodeToString(shortKey),
	} {
		if strings.Contains(logs, value) {
			t.Fatalf("%s logs hold the value of %s", spec.Name, secret.MasterKeyName)
		}
	}
}
