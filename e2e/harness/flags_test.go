package harness

import (
	"slices"
	"strings"
	"testing"
)

func TestRunArgs(t *testing.T) {
	args := runArgs(runSpec{
		Name:        "sag-e2e-x-sag",
		Network:     "sag-e2e-x",
		ConfigFile:  "/tmp/sag.yaml",
		StateVolume: "sag-e2e-x-sag-state",
		Secrets: []secretMount{
			{Source: "sag-e2e-x-sag-master-key", Target: "sag-master-key"},
			{Source: "sag-e2e-x-conn-main", Target: "conn-main"},
		},
		SecretMode: "",
		Image:      "localhost/sag:e2e",
		Command:    []string{"serve"},
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"run -d --name sag-e2e-x-sag --network sag-e2e-x",
		"--read-only",
		"--cap-drop=all",
		"--security-opt=no-new-privileges",
		"--user 65532:65532",
		"-p 127.0.0.1::8443",
		"-v /tmp/sag.yaml:/etc/sag/sag.yaml:ro",
		"-v sag-e2e-x-sag-state:/var/lib/sag",
		"--secret sag-e2e-x-sag-master-key,type=mount,target=sag-master-key,uid=65532,mode=0400",
		"--secret sag-e2e-x-conn-main,type=mount,target=conn-main,uid=65532,mode=0400",
		"localhost/sag:e2e serve",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q:\n%s", want, joined)
		}
	}
	if !strings.HasSuffix(joined, "localhost/sag:e2e serve") {
		t.Errorf("image and command are not last:\n%s", joined)
	}
}

func TestRunArgsSecretMode(t *testing.T) {
	args := runArgs(runSpec{
		Name:       "n",
		Secrets:    []secretMount{{Source: "s", Target: "t"}},
		SecretMode: "0444",
		Image:      "i",
	})
	if !slices.Contains(args, "s,type=mount,target=t,uid=65532,mode=0444") {
		t.Errorf("args lack mode 0444: %v", args)
	}
}
