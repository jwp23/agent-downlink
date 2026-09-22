package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/config"
)

// Never dispatch setup without --no-timer here: it would install a real schedule for the user
// running the tests.
func TestSetupCommandWithNoTimer(t *testing.T) {
	e, stdout, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup", "--no-timer"}); got != 0 {
		t.Fatalf("exit status = %d, stderr = %q", got, stderr.String())
	}
	if _, err := os.Stat(config.PathsFor(e.home).RcloneConf); err != nil {
		t.Errorf("rclone.conf not written: %v", err)
	}
	if !strings.Contains(stdout.String(), "password manager") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestSetupCommandRejectsExtraArgs(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"setup", "--no-timer", "extra"}); got != 2 {
		t.Errorf("exit status = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "usage: agent-downlink setup [--no-timer]") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestSetupCommandInstallsTimerWhenNotDisabled(t *testing.T) {
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup"}); got != 0 {
		t.Fatalf("exit status = %d, stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "The hourly schedule is installed") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestSetupCommandReportsTimerInstallError(t *testing.T) {
	withFakeScheduler(t, errors.New("boom"))
	e, _, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink setup:") || !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestSetupCommandRejectsUnknownFlags(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"setup", "--frobnicate"}); got != 2 {
		t.Errorf("exit status = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "frobnicate") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestAddMachineCommandNeedsExactlyOneName(t *testing.T) {
	for _, args := range [][]string{{"add-machine"}, {"add-machine", "a", "b"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), "usage: agent-downlink add-machine <name>") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestAddMachineCommandReportsErrors(t *testing.T) {
	e, _, stderr := testEnv(t)
	e.stdin = strings.NewReader("pw\n")
	if got := dispatch(e, []string{"add-machine", "laptop"}); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestAddMachineCommandSucceeds(t *testing.T) {
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup", "--no-timer"}); got != 0 {
		t.Fatalf("setup: exit status = %d, stderr = %q", got, stderr.String())
	}

	e.stdin = strings.NewReader("laptops-placeholder-password\n")
	if got := dispatch(e, []string{"add-machine", "workstation"}); got != 0 {
		t.Fatalf("add-machine: exit status = %d, stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "workstation is now readable here") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRemoveMachineCommandNeedsExactlyOneName(t *testing.T) {
	for _, args := range [][]string{{"remove-machine"}, {"remove-machine", "a", "b"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), "usage: agent-downlink remove-machine <name>") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestRemoveMachineCommandReportsErrors(t *testing.T) {
	e, _, stderr := testEnv(t)
	e.stdin = strings.NewReader("yes\n")
	if got := dispatch(e, []string{"remove-machine", "laptop"}); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink remove-machine: ") || !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRemoveMachineCommandSucceeds(t *testing.T) {
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup", "--no-timer"}); got != 0 {
		t.Fatalf("setup: exit status = %d, stderr = %q", got, stderr.String())
	}
	e.stdin = strings.NewReader("workstations-placeholder-password\n")
	if got := dispatch(e, []string{"add-machine", "workstation"}); got != 0 {
		t.Fatalf("add-machine: exit status = %d, stderr = %q", got, stderr.String())
	}

	e.stdin = strings.NewReader("yes\n")
	if got := dispatch(e, []string{"remove-machine", "workstation"}); got != 0 {
		t.Fatalf("remove-machine: exit status = %d, stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "workstation is no longer readable here") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
