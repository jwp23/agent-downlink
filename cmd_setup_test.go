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
