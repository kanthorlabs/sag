//go:build !devxtest

package testhook

func Pins() string {
	return ""
}

func Image() string {
	return ""
}

func CertDays() int {
	return 0
}

func Absent() bool {
	return false
}

func StopBeforeInstall() bool {
	return false
}

func Registry() string {
	return ""
}

func FailCredCreate() bool {
	return false
}

func FailCredDelete() bool {
	return false
}

func FailCertRenewal() bool {
	return false
}

func FailTLSCACreate() bool {
	return false
}
