//go:build devxtest

package endpointcert_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kanthorlabs/sag/internal/endpointcert"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func TestResumeAfterStep3(t *testing.T) {
	dir := t.TempDir()
	now := newNow()
	cfg := endpointcert.Config{CfgDir: dir}

	t.Setenv("SAG_TEST_FAIL_CERT_RENEWAL", "1")

	sf := newStatefulFake()
	out := stepline.New(io.Discard)
	_, err := endpointcert.Ensure(context.Background(), sf, cfg, now, out)
	if err == nil {
		t.Fatal("expected error from FailCertRenewal hook")
	}

	_, errNew := os.Stat(filepath.Join(dir, "ca.pem.new"))
	if errNew != nil {
		t.Fatal("ca.pem.new must exist after partial run")
	}
	_, errPem := os.Stat(filepath.Join(dir, "ca.pem"))
	if errPem == nil {
		t.Fatal("ca.pem must not exist after partial run (rename was not done)")
	}

	if !sf.secrets["sag-tls-cert"] || !sf.secrets["sag-tls-key"] {
		t.Fatal("secrets must be present after first run created them")
	}

	t.Setenv("SAG_TEST_FAIL_CERT_RENEWAL", "")

	out2 := stepline.New(io.Discard)
	changed, err := endpointcert.Ensure(context.Background(), sf, cfg, now, out2)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on second run")
	}

	_, errNew2 := os.Stat(filepath.Join(dir, "ca.pem.new"))
	if errNew2 == nil {
		t.Fatal("ca.pem.new must be removed after second run")
	}
	_, errPem2 := os.Stat(filepath.Join(dir, "ca.pem"))
	if errPem2 != nil {
		t.Fatal("ca.pem must exist after second run")
	}
}
