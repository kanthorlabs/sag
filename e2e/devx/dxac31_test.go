//go:build devxe2e

package devx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
	"github.com/kanthorlabs/sag/internal/testenv"
)

func init() {
	awsTests["DXAC31_AWSBackend"] = func(t *testing.T, ctx context.Context) {
		DXAC31_AWSBackend(t, ctx)
	}
}

func DXAC31_AWSBackend(t *testing.T, ctx context.Context) {
	t.Helper()

	const resName = "dxac31-db"
	const fakeDSN = "postgres://test:test@nonexistent.internal:5432/testdb"

	uid := fmt.Sprintf("%d", os.Getuid())
	r := dxh.NewRunner()
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC31: wait launch agent idle: %v", err)
	}

	ca, err := testenv.NewCA()
	if err != nil {
		t.Fatalf("DXAC31: test CA: %v", err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM, 0o600); err != nil {
		t.Fatalf("DXAC31: write CA: %v", err)
	}

	t.Log("create with aws-sm backend")
	code, combined := runSAGWithInputAndEnvExtra(t, ctx, []byte(fakeDSN), nil,
		"credential", "create", resName,
		"--kind", "postgres",
		"--agent", "pi",
		"--backend", "aws-sm",
		"--tls", "verify-full",
		"--tls-ca", caFile,
		"--stdin",
		"--accept-failed-check",
	)
	if code != 0 {
		t.Fatalf("DXAC31: credential create: exit %d\n%s", code, combined)
	}
	if want := "resource " + resName + " created; SAG restarted; run sag resource test " + resName; !dxac30HasLine(combined, want) {
		t.Fatalf("DXAC31: create output lacks %q:\n%s", want, combined)
	}

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC31: wait launch agent idle after create: %v", err)
	}

	t.Log("list shows aws-sm backend")
	jsonOut, _, listErr := runSAGWithStderr(t, ctx, "credential", "list", "--json")
	if listErr != nil {
		t.Fatalf("DXAC31: credential list: %v", listErr)
	}

	var listResult struct {
		Credentials []map[string]any `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &listResult); err != nil {
		t.Fatalf("DXAC31: credential list parse: %v", err)
	}

	entry := findEntryByName(listResult.Credentials, resName)
	if entry == nil {
		t.Fatalf("DXAC31: %q not in credential list; got %v", resName, listResult.Credentials)
	}

	secrets, _ := entry["secrets"].([]any)
	if len(secrets) == 0 {
		t.Fatal("DXAC31: credential list: no secrets in entry")
	}
	for _, s := range secrets {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		backend, _ := sm["backend"].(string)
		if backend != "aws-sm" {
			t.Errorf("DXAC31: secret backend = %q; want aws-sm", backend)
		}
		if _, hasMounted := sm["mounted"]; hasMounted {
			t.Errorf("DXAC31: aws-sm secret must not have mounted field; got %v", sm)
		}
		if sm["exists"] != true || sm["owned"] != true {
			t.Errorf("DXAC31: aws-sm secret must exist and be owned; got %v", sm)
		}
	}

	if len(secrets) != 2 {
		t.Errorf("DXAC31: credential list: %d secrets; want connection and tls_ca", len(secrets))
	}

	t.Log("check aws secret tags")
	dxac31AssertTags(t, ctx, resName, "conn", "connection")
	dxac31AssertTags(t, ctx, resName, "ca", "tls_ca")

	t.Log("resource test passes with aws-sm backend")
	const realResName = "dxac31-realdb"
	pgDSN, pgErr := dxh.StartAWSTestPostgres(ctx, r, "sag-dxac31-pg")
	if pgErr != nil {
		t.Fatalf("DXAC31: start test postgres: %v", pgErr)
	}
	t.Cleanup(func() {
		_, _ = r.Output(context.Background(), "podman", "stop", "sag-dxac31-pg")
	})
	realCode, realOut := runSAGWithInputAndEnvExtra(t, ctx, []byte(pgDSN), nil,
		"credential", "create", realResName,
		"--kind", "postgres",
		"--agent", "pi",
		"--backend", "aws-sm",
		"--tls", "disable",
		"--stdin",
	)
	if realCode != 0 {
		t.Fatalf("DXAC31: credential create real: exit %d\n%s", realCode, realOut)
	}
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC31: wait idle after real create: %v", err)
	}
	dxac31AssertTags(t, ctx, realResName, "conn", "connection")
	rtStdout, rtStderr, rtErr := runSAGWithStderr(t, ctx, "resource", "test", realResName)
	if rtErr != nil {
		t.Fatalf("DXAC31: resource test %s: %v\nstdout: %s\nstderr: %s", realResName, rtErr, rtStdout, rtStderr)
	}
	realDelCode, realDelTTY := deleteViaStartTTY(t, ctx, realResName)
	if realDelCode != 0 {
		t.Fatalf("DXAC31: delete real: exit %d; output: %s", realDelCode, realDelTTY.RawOutput())
	}
	dxac31AssertNoSkip(t, realDelTTY.RawOutput())
	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC31: wait idle after real delete: %v", err)
	}

	t.Log("delete")
	code, tty := deleteViaStartTTY(t, ctx, resName)
	if code != 0 {
		t.Fatalf("DXAC31: delete %s: exit %d; output: %s", resName, code, tty.RawOutput())
	}
	dxac31AssertNoSkip(t, tty.RawOutput())

	if err := dxh.WaitLaunchAgentIdle(ctx, r, uid); err != nil {
		t.Fatalf("DXAC31: wait launch agent idle after delete: %v", err)
	}

	t.Log("list does not show resource after delete")
	jsonOut2, _, listErr2 := runSAGWithStderr(t, ctx, "credential", "list", "--json")
	if listErr2 != nil {
		t.Fatalf("DXAC31: credential list after delete: %v", listErr2)
	}
	var listResult2 struct {
		Credentials []map[string]any `json:"credentials"`
	}
	if err := json.Unmarshal([]byte(jsonOut2), &listResult2); err != nil {
		t.Fatalf("DXAC31: credential list after delete parse: %v", err)
	}
	if findEntryByName(listResult2.Credentials, resName) != nil {
		t.Fatalf("DXAC31: %q still in credential list after delete", resName)
	}

	t.Log("AWS secrets gone from LocalStack after delete")
	for _, name := range []string{"sag/" + resName + "/conn", "sag/" + resName + "/ca", "sag/" + realResName + "/conn"} {
		exists, err := dxh.LocalStackSecretExists(ctx, name)
		if err != nil {
			t.Fatalf("DXAC31: describe %s after delete: %v", name, err)
		}
		if exists {
			t.Errorf("DXAC31: AWS secret %s still present after delete", name)
		}
	}
}

func dxac31AssertTags(t *testing.T, ctx context.Context, resName, suffix, role string) {
	t.Helper()
	name := "sag/" + resName + "/" + suffix
	got, err := dxh.LocalStackTags(ctx, name)
	if err != nil {
		t.Fatalf("DXAC31: tags of %s: %v", name, err)
	}
	want := map[string]string{
		"sag.resource": resName,
		"sag.kind":     "postgres",
		"sag.role":     role,
	}
	if !maps.Equal(got, want) {
		t.Errorf("DXAC31: tags of %s = %v; want exactly %v", name, got, want)
	}
}

func dxac31AssertNoSkip(t *testing.T, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "skip ") {
			t.Errorf("DXAC31: delete printed a skip line for an owned secret: %q", line)
		}
	}
}

func runSAGWithInputAndEnvExtra(t *testing.T, ctx context.Context, stdin []byte, extraEnv []string, args ...string) (int, string) {
	t.Helper()
	bin, err := dxh.TestBinary()
	if err != nil {
		t.Fatalf("TestBinary: %v", err)
	}
	groupDir := t.TempDir()
	binDir := t.TempDir()

	baseEnv := append(append([]string{}, os.Environ()...), extraEnv...)
	env, err := dxh.ChildEnv(baseEnv, groupDir)
	if err != nil {
		t.Fatalf("ChildEnv: %v", err)
	}
	env = append(env, "PATH="+dxh.TestPATH(binDir))

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runSAGWithInputAndEnvExtra(%v): %v", args, runErr)
		}
	}
	return code, out.String()
}
