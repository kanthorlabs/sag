//go:build devxtest

package testhook

import (
	"os"
	"testing"
)

func TestHooksReadEnv(t *testing.T) {
	t.Setenv("SAG_TEST_PINS", "testpins")
	if got := Pins(); got != "testpins" {
		t.Fatalf("Pins() = %q; want testpins", got)
	}

	t.Setenv("SAG_TEST_IMAGE", "localhost/sag:test")
	if got := Image(); got != "localhost/sag:test" {
		t.Fatalf("Image() = %q; want localhost/sag:test", got)
	}

	t.Setenv("SAG_TEST_CERT_DAYS", "20")
	if got := CertDays(); got != 20 {
		t.Fatalf("CertDays() = %d; want 20", got)
	}

	t.Setenv("SAG_TEST_ABSENT", "1")
	if !Absent() {
		t.Fatal("Absent() = false; want true")
	}
	os.Unsetenv("SAG_TEST_ABSENT")
	if Absent() {
		t.Fatal("Absent() = true; want false after unset")
	}

	t.Setenv("SAG_TEST_STOP_BEFORE_INSTALL", "1")
	if !StopBeforeInstall() {
		t.Fatal("StopBeforeInstall() = false; want true")
	}

	t.Setenv("SAG_TEST_REGISTRY", "localhost:5000")
	if got := Registry(); got != "localhost:5000" {
		t.Fatalf("Registry() = %q; want localhost:5000", got)
	}

	t.Setenv("SAG_TEST_FAIL_CRED_CREATE", "1")
	if !FailCredCreate() {
		t.Fatal("FailCredCreate() = false; want true")
	}

	t.Setenv("SAG_TEST_FAIL_CRED_DELETE", "1")
	if !FailCredDelete() {
		t.Fatal("FailCredDelete() = false; want true")
	}

	t.Setenv("SAG_TEST_FAIL_CERT_RENEWAL", "1")
	if !FailCertRenewal() {
		t.Fatal("FailCertRenewal() = false; want true")
	}

	t.Setenv("SAG_TEST_FAIL_TLS_CA_CREATE", "1")
	if !FailTLSCACreate() {
		t.Fatal("FailTLSCACreate() = false; want true")
	}
}
