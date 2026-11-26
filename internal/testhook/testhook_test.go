//go:build !devxtest

package testhook

import "testing"

func TestProductHooksReturnZeroValues(t *testing.T) {
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
}
