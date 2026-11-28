//go:build !devxtest

package deploy_test

import (
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
)

func TestQuadletProductBuildNoAWSEndpoint(t *testing.T) {
	t.Setenv("SAG_TEST_AWS_ENDPOINT", "http://host.containers.internal:4566")
	q := deploy.Quadlet(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "c", RestartPolicy: "on-failure"})
	if strings.Contains(q, "AWS_ENDPOINT_URL_SECRETS_MANAGER") {
		t.Fatal("product Quadlet must not contain AWS endpoint env entry")
	}
}
