//go:build devxtest

package testhook

import (
	"os"
	"strconv"
)

func Pins() string {
	return os.Getenv("SAG_TEST_PINS")
}

func Image() string {
	return os.Getenv("SAG_TEST_IMAGE")
}

func CertDays() int {
	v := os.Getenv("SAG_TEST_CERT_DAYS")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func Absent() bool {
	return os.Getenv("SAG_TEST_ABSENT") == "1"
}

func StopBeforeInstall() bool {
	return os.Getenv("SAG_TEST_STOP_BEFORE_INSTALL") == "1"
}

func Registry() string {
	return os.Getenv("SAG_TEST_REGISTRY")
}

func FailCredCreate() bool {
	return os.Getenv("SAG_TEST_FAIL_CRED_CREATE") == "1"
}

func FailCredDelete() bool {
	return os.Getenv("SAG_TEST_FAIL_CRED_DELETE") == "1"
}

func FailCertRenewal() bool {
	return os.Getenv("SAG_TEST_FAIL_CERT_RENEWAL") == "1"
}

func FailTLSCACreate() bool {
	return os.Getenv("SAG_TEST_FAIL_TLS_CA_CREATE") == "1"
}

func RequireHTTPS() bool {
	return false
}

func AWSEndpoint() string {
	return os.Getenv("SAG_TEST_AWS_ENDPOINT")
}
