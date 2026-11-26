package deps

import (
	"context"
	"errors"
	"testing"
)

func TestSudoCheckArgsNoInteractive(t *testing.T) {
	args := sudoCheckArgs(true)
	want := []string{"sudo", "-n", "true"}
	if len(args) != len(want) {
		t.Fatalf("sudoCheckArgs(true) = %v; want %v", args, want)
	}
	for i, v := range want {
		if args[i] != v {
			t.Errorf("sudoCheckArgs(true)[%d] = %q; want %q", i, args[i], v)
		}
	}
}

func TestSudoCheckArgsInteractive(t *testing.T) {
	args := sudoCheckArgs(false)
	want := []string{"sudo", "-v"}
	if len(args) != len(want) {
		t.Fatalf("sudoCheckArgs(false) = %v; want %v", args, want)
	}
	for i, v := range want {
		if args[i] != v {
			t.Errorf("sudoCheckArgs(false)[%d] = %q; want %q", i, args[i], v)
		}
	}
}

func TestInstallerArgsNoInteractive(t *testing.T) {
	args := installerArgs("/tmp/pkg.pkg", true)
	want := []string{"sudo", "-n", "installer", "-pkg", "/tmp/pkg.pkg", "-target", "/"}
	if len(args) != len(want) {
		t.Fatalf("installerArgs(noInteractive) = %v; want %v", args, want)
	}
	for i, v := range want {
		if args[i] != v {
			t.Errorf("installerArgs(noInteractive)[%d] = %q; want %q", i, args[i], v)
		}
	}
}

func TestInstallerArgsInteractive(t *testing.T) {
	args := installerArgs("/tmp/pkg.pkg", false)
	want := []string{"sudo", "installer", "-pkg", "/tmp/pkg.pkg", "-target", "/"}
	if len(args) != len(want) {
		t.Fatalf("installerArgs(interactive) = %v; want %v", args, want)
	}
	for i, v := range want {
		if args[i] != v {
			t.Errorf("installerArgs(interactive)[%d] = %q; want %q", i, args[i], v)
		}
	}
}

func TestRunInstallSudoCheckFailsIsSudoRefusal(t *testing.T) {
	callCount := 0
	err := runInstall(context.Background(), "/tmp/pkg.pkg", true, func(_ context.Context, args []string) error {
		callCount++
		if args[0] == "sudo" && len(args) >= 2 && args[1] == "-n" && args[2] == "true" {
			return errors.New("sudo refused")
		}
		return nil
	})
	var sudoRef *SudoRefusal
	if !errors.As(err, &sudoRef) {
		t.Fatalf("runInstall: err = %v; want SudoRefusal", err)
	}
	if callCount != 1 {
		t.Fatalf("callCount = %d; want 1 (only sudo check, no installer)", callCount)
	}
}

func TestRunInstallInstallerErrorIsNotSudoRefusal(t *testing.T) {
	callCount := 0
	installerErr := errors.New("installer failed")
	err := runInstall(context.Background(), "/tmp/pkg.pkg", true, func(_ context.Context, args []string) error {
		callCount++
		if args[0] == "sudo" && len(args) >= 3 && args[2] == "true" {
			return nil
		}
		return installerErr
	})
	if err == nil {
		t.Fatal("runInstall: want error from installer, got nil")
	}
	var sudoRef *SudoRefusal
	if errors.As(err, &sudoRef) {
		t.Fatal("runInstall: installer error mapped to SudoRefusal; want plain error")
	}
	if callCount != 2 {
		t.Fatalf("callCount = %d; want 2 (sudo check + installer)", callCount)
	}
}
