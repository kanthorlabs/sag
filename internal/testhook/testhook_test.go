//go:build !devxtest

package testhook

import "testing"

func TestProductHooksReturnZeroValues(t *testing.T) {
	t.Setenv("SAG_TEST_PINS", "some-pins")
	t.Setenv("SAG_TEST_IMAGE", "localhost/sag:fake")
	t.Setenv("SAG_TEST_CERT_DAYS", "20")
	t.Setenv("SAG_TEST_ABSENT", "1")
	t.Setenv("SAG_TEST_STOP_BEFORE_INSTALL", "1")
	t.Setenv("SAG_TEST_REGISTRY", "localhost:5000")
	t.Setenv("SAG_TEST_FAIL_CRED_CREATE", "1")
	t.Setenv("SAG_TEST_FAIL_CRED_DELETE", "1")
	t.Setenv("SAG_TEST_FAIL_CERT_RENEWAL", "1")
	t.Setenv("SAG_TEST_FAIL_TLS_CA_CREATE", "1")

	if Pins() != "" {
		t.Fatalf("Pins() = %q; want empty", Pins())
	}
	if Image() != "" {
		t.Fatalf("Image() = %q; want empty", Image())
	}
	if CertDays() != 0 {
		t.Fatalf("CertDays() = %d; want 0", CertDays())
	}
	if Absent() {
		t.Fatal("Absent() = true; want false")
	}
	if StopBeforeInstall() {
		t.Fatal("StopBeforeInstall() = true; want false")
	}
	if Registry() != "" {
		t.Fatalf("Registry() = %q; want empty", Registry())
	}
	if FailCredCreate() {
		t.Fatal("FailCredCreate() = true; want false")
	}
	if FailCredDelete() {
		t.Fatal("FailCredDelete() = true; want false")
	}
	if FailCertRenewal() {
		t.Fatal("FailCertRenewal() = true; want false")
	}
	if FailTLSCACreate() {
		t.Fatal("FailTLSCACreate() = true; want false")
	}

	t.Setenv("SAG_TEST_AWS_ENDPOINT", "http://host.containers.internal:4566")
	if AWSEndpoint() != "" {
		t.Fatalf("AWSEndpoint() = %q; want empty", AWSEndpoint())
	}
}
