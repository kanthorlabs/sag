//go:build devxe2e

package dxh

import "testing"

const LocalStackImage = "docker.io/localstack/localstack@sha256:b279c01f4cfb8f985a482e4014cabc1e2697b9d7a6c8c8db2e40f4d9f93687c7"

func EnsureLocalStack(t *testing.T) {
	t.Helper()
}
