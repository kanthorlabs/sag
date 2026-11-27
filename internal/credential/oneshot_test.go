package credential

import (
	"context"
	"reflect"
	"testing"

	"github.com/kanthorlabs/sag/internal/deploy"
	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
)

func TestOneShotRunsCorrectArgs(t *testing.T) {
	cmd := &fakePodmanCmd{}
	const image = "sag:test"
	const resName = "myres"

	err := revokeOneShot(context.Background(), resName, cmd, image)
	if err != nil {
		t.Fatalf("revokeOneShot: %v", err)
	}
	if len(cmd.args) == 0 {
		t.Fatal("no podman run call")
	}

	p := deploy.DefaultFlags.Product
	masterKeySecret := p.Secret(secret.MasterKeyName, secret.MasterKeyName, "")
	stateVol := p.StateVolume(deploy.StateVolumeName)

	var want []string
	want = append(want, "run", "--rm")
	want = append(want, p.SecurityArgs()...)
	want = append(want, "-v", stateVol)
	want = append(want, "--secret", masterKeySecret)
	want = append(want, image)
	want = append(want, "extension", "revoke", resName)

	if !reflect.DeepEqual(cmd.args[0], want) {
		t.Errorf("args mismatch\ngot:  %v\nwant: %v", cmd.args[0], want)
	}

	for i, arg := range cmd.args[0] {
		if arg == "canary" || len(arg) > 10 && arg[:7] == "eyJ" {
			t.Errorf("arg[%d] holds canary or JWT: %q", i, arg)
		}
	}
}

func TestOneShotPropagatesError(t *testing.T) {
	cmd := &fakePodmanCmd{execErr: &podman.RunError{ExitCode: 1, Stderr: "some error"}}
	err := revokeOneShot(context.Background(), "myres", cmd, "sag:test")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
