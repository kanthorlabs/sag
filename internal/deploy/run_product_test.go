//go:build !devxtest

package deploy_test

import (
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
)

func TestRunArgsProductBuildNoAWSEndpoint(t *testing.T) {
	t.Setenv("SAG_TEST_AWS_ENDPOINT", "http://host.containers.internal:4566")
	args := deploy.RunArgs(minCfg(t), deploy.RunOpts{Image: "img", ConfigSource: "c"})
	for _, a := range args {
		if a == "AWS_ENDPOINT_URL_SECRETS_MANAGER=http://host.containers.internal:4566" {
			t.Fatalf("product RunArgs must not contain AWS endpoint env entry")
		}
	}
}
