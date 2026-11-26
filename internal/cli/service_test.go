package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestServiceStartSucceeds(t *testing.T) {
	old := serviceStartImpl
	defer func() { serviceStartImpl = old }()
	serviceStartImpl = func(_ []string, std IO) int {
		return ExitOK
	}

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitOK {
		t.Fatalf("serviceStart = %d; want 0", code)
	}
}

func TestServiceStartFailsOnError(t *testing.T) {
	old := serviceStartImpl
	defer func() { serviceStartImpl = old }()
	serviceStartImpl = func(_ []string, std IO) int {
		return fail(std, fmt.Errorf("start failed"))
	}

	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStart(nil, std); code != ExitFailure {
		t.Fatalf("serviceStart = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "start failed") {
		t.Fatalf("stderr = %q; want 'start failed'", errOut.String())
	}
}

func TestServiceStopSucceeds(t *testing.T) {
	old := serviceStopImpl
	defer func() { serviceStopImpl = old }()
	serviceStopImpl = func(_ []string, std IO) int {
		return ExitOK
	}

	std, _ := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitOK {
		t.Fatalf("serviceStop = %d; want 0", code)
	}
}

func TestServiceStopFailsOnError(t *testing.T) {
	old := serviceStopImpl
	defer func() { serviceStopImpl = old }()
	serviceStopImpl = func(_ []string, std IO) int {
		return fail(std, fmt.Errorf("stop failed"))
	}

	std, errOut := testIO()
	std.Mode = ModeHost
	if code := serviceStop(nil, std); code != ExitFailure {
		t.Fatalf("serviceStop = %d; want %d", code, ExitFailure)
	}
	if !strings.Contains(errOut.String(), "stop failed") {
		t.Fatalf("stderr = %q; want 'stop failed'", errOut.String())
	}
}
