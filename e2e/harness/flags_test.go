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
	want := []string{
		"run", "-d", "--name", "sag-e2e-x-sag", "--network", "sag-e2e-x",
		"--log-driver", "k8s-file",
		"--log-opt", "max-size=10mb",
		"--read-only", "--read-only-tmpfs=false", "--cap-drop=all", "--security-opt=no-new-privileges", "--user", "65532:65532",
		"-p", "127.0.0.1::8443",
		"-v", "/tmp/sag.yaml:/etc/sag/sag.yaml:ro",
		"-v", "sag-e2e-x-sag-state:/var/lib/sag:noexec",
		"--secret", "sag-e2e-x-sag-master-key,type=mount,target=sag-master-key,uid=65532,mode=0400",
		"--secret", "sag-e2e-x-conn-main,type=mount,target=conn-main,uid=65532,mode=0400",
		"localhost/sag:e2e", "serve",
	}
	if !slices.Equal(args, want) {
		t.Errorf("args =\n%q\nwant\n%q", args, want)
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

func TestRunArgsReadOnlyAudit(t *testing.T) {
	args := strings.Join(runArgs(runSpec{Name: "n", StateVolume: "n-state", AuditDir: "/tmp/run/n-audit", Image: "i"}), " ")
	if !strings.Contains(args, "-v n-state:/var/lib/sag:noexec -v /tmp/run/n-audit:/var/lib/sag/audit:ro") {
		t.Errorf("args lack the read-only audit mount after the state volume: %s", args)
	}
	if strings.Contains(strings.Join(runArgs(runSpec{Name: "n", StateVolume: "n-state", Image: "i"}), " "), "/var/lib/sag/audit") {
		t.Error("args mount an audit directory without AuditDir")
	}
}

func TestReadOnlyAuditProfile(t *testing.T) {
	for name, p := range containerProfiles {
		if p.readOnlyAudit != (name == ContainerROAudit) {
			t.Errorf("container %s: readOnlyAudit = %v", name, p.readOnlyAudit)
		}
	}
}

func TestRunArgsBusybox(t *testing.T) {
	args := strings.Join(runArgs(runSpec{Name: "n", Busybox: "/tmp/run/busybox", Image: "i"}), " ")
	if !strings.Contains(args, "-v /tmp/run/busybox:/e2e/busybox:ro") {
		t.Errorf("args lack the read-only busybox mount: %s", args)
	}
	if strings.Contains(strings.Join(runArgs(runSpec{Name: "n", Image: "i"}), " "), BusyboxTarget) {
		t.Error("args mount busybox without Busybox")
	}
}

func TestProductFlagsExcludeTestOnlyFlags(t *testing.T) {
	p := ContainerFlags.Product
	product := strings.Join(append(p.SecurityArgs(), p.Publish(ListenPort), p.ConfigVolume("c"), p.StateVolume("s"), p.Secret("a", "b", "")), " ")
	for _, flag := range TestOnlyFlags {
		if strings.Contains(product, flag) {
			t.Errorf("product flags hold the test-only flag %q: %s", flag, product)
		}
	}
	if got := p.Publish(ListenPort); got != "127.0.0.1:8443:8443" {
		t.Errorf("Publish(8443) = %q; want 127.0.0.1:8443:8443", got)
	}
}
