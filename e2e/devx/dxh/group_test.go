package dxh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/e2e/devx/dxh"
)

func TestGroupSetupTagsMainImage(t *testing.T) {
	r := newFake()
	r.set("podman tag localhost/sag:devx-e2e docker.io/kanthorlabs/sag:main", nil, nil)

	if err := dxh.GroupSetupImage(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "podman tag") && strings.Contains(joined, "localhost/sag:devx-e2e") && strings.Contains(joined, "docker.io/kanthorlabs/sag:main") {
			found = true
		}
	}
	if !found {
		t.Errorf("GroupSetupImage did not tag localhost/sag:devx-e2e as docker.io/kanthorlabs/sag:main; calls: %v", r.calls)
	}
}
