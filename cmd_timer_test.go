package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/scheduler"
)

// markTimerInstalled creates the systemd timer unit that Scheduler.Remove looks for, so a
// fake Remove exercises its "disable, then remove files" path instead of the no-op case.
func markTimerInstalled(t *testing.T, home string) {
	t.Helper()
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "agent-downlink.timer"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

func timerUnitExists(t *testing.T, home string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "agent-downlink.timer"))
	return err == nil
}

func TestTimerNeedsInstallOrRemove(t *testing.T) {
	for _, args := range [][]string{{"timer"}, {"timer", "frobnicate"}, {"timer", "install", "extra"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), "agent-downlink timer install|remove") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

// withFakeScheduler overrides ownBinaryFn and newScheduler for the duration of a test, so
// cmdTimer never touches the real executable path or the real systemd/launchd tooling.
func withFakeScheduler(t *testing.T, execErr error) {
	t.Helper()
	origOwnBinary, origNewScheduler := ownBinaryFn, newScheduler
	t.Cleanup(func() {
		ownBinaryFn = origOwnBinary
		newScheduler = origNewScheduler
	})
	ownBinaryFn = func() (string, error) { return "/usr/local/bin/agent-downlink", nil }
	newScheduler = func(home, binary string) *scheduler.Scheduler {
		return &scheduler.Scheduler{
			Home:   home,
			Binary: binary,
			GOOS:   "linux",
			Exec:   func(name string, args ...string) error { return execErr },
		}
	}
}

func TestTimerInstallSucceeds(t *testing.T) {
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	if got := dispatch(e, []string{"timer", "install"}); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Hourly schedule installed") {
		t.Errorf("stdout = %q, want the install confirmation", stdout.String())
	}
	if !timerUnitExists(t, e.home) {
		t.Error("timer install wrote no timer unit")
	}
}

func TestTimerRemoveSucceeds(t *testing.T) {
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	markTimerInstalled(t, e.home)
	if got := dispatch(e, []string{"timer", "remove"}); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Hourly schedule removed") {
		t.Errorf("stdout = %q, want the remove confirmation", stdout.String())
	}
	if timerUnitExists(t, e.home) {
		t.Error("timer remove left the timer unit in place")
	}
}

func TestTimerInstallReportsSchedulerError(t *testing.T) {
	withFakeScheduler(t, errors.New("boom"))
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"timer", "install"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink timer install: boom") {
		t.Errorf("stderr = %q, want the install error", stderr.String())
	}
}

func TestTimerRemoveReportsSchedulerError(t *testing.T) {
	withFakeScheduler(t, errors.New("boom"))
	e, _, stderr := testEnv(t)
	markTimerInstalled(t, e.home)
	if got := dispatch(e, []string{"timer", "remove"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink timer remove: boom") {
		t.Errorf("stderr = %q, want the remove error", stderr.String())
	}
}

func TestTimerReportsOwnBinaryError(t *testing.T) {
	origOwnBinary := ownBinaryFn
	t.Cleanup(func() { ownBinaryFn = origOwnBinary })
	ownBinaryFn = func() (string, error) { return "", errors.New("no exe") }

	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"timer", "install"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink timer: no exe") {
		t.Errorf("stderr = %q, want the ownBinary error", stderr.String())
	}
}

func TestOwnBinaryResolvesTheTestBinary(t *testing.T) {
	path, err := ownBinary()
	if err != nil {
		t.Fatalf("ownBinary() error = %v", err)
	}
	if path == "" {
		t.Error("ownBinary() = empty path")
	}
}
