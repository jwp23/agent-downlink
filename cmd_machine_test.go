package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestMachineCommandNeedsAKnownSubcommand(t *testing.T) {
	for _, args := range [][]string{{"machine"}, {"machine", "frobnicate"}, {"machine", "frobnicate", "laptop"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), machineUsage) {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestMachineAddNeedsExactlyOneName(t *testing.T) {
	for _, args := range [][]string{{"machine", "add"}, {"machine", "add", "a", "b"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), "usage: agent-downlink machine add <name>") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestMachineAddReportsErrors(t *testing.T) {
	e, _, stderr := testEnv(t)
	e.stdin = strings.NewReader("pw\n")
	if got := dispatch(e, []string{"machine", "add", "laptop"}); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink machine add: ") || !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// setUpLaptop runs setup for a machine named laptop so machine subcommands have a config to
// work on. Never dispatch setup without --no-timer: it would install a real schedule.
func setUpLaptop(t *testing.T) (env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	withFakeScheduler(t, nil)
	e, stdout, stderr := testEnv(t)
	e.stdin = strings.NewReader("laptop\nscratch-bucket\n000placeholderkeyid\nK000placeholderkey\n\n")
	if got := dispatch(e, []string{"setup", "--no-timer"}); got != 0 {
		t.Fatalf("setup: exit status = %d, stderr = %q", got, stderr.String())
	}
	return e, stdout, stderr
}

func TestMachineAddSucceeds(t *testing.T) {
	e, stdout, stderr := setUpLaptop(t)
	e.stdin = strings.NewReader("workstations-placeholder-password\n")
	if got := dispatch(e, []string{"machine", "add", "workstation"}); got != 0 {
		t.Fatalf("machine add: exit status = %d, stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "workstation is now readable here") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestMachineRemoveNeedsExactlyOneName(t *testing.T) {
	for _, args := range [][]string{{"machine", "remove"}, {"machine", "remove", "a", "b"}} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", args, got)
		}
		if !strings.Contains(stderr.String(), "usage: agent-downlink machine remove <name>") {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestMachineRemoveReportsErrors(t *testing.T) {
	e, _, stderr := testEnv(t)
	e.stdin = strings.NewReader("yes\n")
	if got := dispatch(e, []string{"machine", "remove", "laptop"}); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink machine remove: ") || !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestMachineRemoveSucceeds(t *testing.T) {
	e, stdout, stderr := setUpLaptop(t)
	e.stdin = strings.NewReader("workstations-placeholder-password\n")
	if got := dispatch(e, []string{"machine", "add", "workstation"}); got != 0 {
		t.Fatalf("machine add: exit status = %d, stderr = %q", got, stderr.String())
	}
	e.stdin = strings.NewReader("yes\n")
	if got := dispatch(e, []string{"machine", "remove", "workstation"}); got != 0 {
		t.Fatalf("machine remove: exit status = %d, stderr = %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "workstation is no longer readable here") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestOldMachineSpellingsAreGone(t *testing.T) {
	for _, name := range []string{"add-machine", "remove-machine"} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, []string{name, "laptop"}); got != 2 {
			t.Errorf("%s: exit status = %d, want 2", name, got)
		}
		if !strings.Contains(stderr.String(), "unknown command") {
			t.Errorf("%s: stderr = %q", name, stderr.String())
		}
	}
}

func TestMachineListTakesNoArguments(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"machine", "list", "extra"}); got != 2 {
		t.Errorf("exit status = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "usage: agent-downlink machine list") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestMachineListReportsErrors(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"machine", "list"}); got != 1 {
		t.Errorf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink machine list: ") || !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestMachineListShowsReadableMachines(t *testing.T) {
	e, stdout, stderr := setUpLaptop(t)
	e.stdin = strings.NewReader("workstations-placeholder-password\n")
	if got := dispatch(e, []string{"machine", "add", "workstation"}); got != 0 {
		t.Fatalf("machine add: exit status = %d, stderr = %q", got, stderr.String())
	}
	stdout.Reset()
	if got := dispatch(e, []string{"machine", "list"}); got != 0 {
		t.Fatalf("machine list: exit status = %d, stderr = %q", got, stderr.String())
	}
	if want := "laptop (this machine)\nworkstation\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}
