package main

import (
	"bytes"
	"strings"
	"testing"
)

func testEnv(t *testing.T) (env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return env{home: t.TempDir(), stdin: strings.NewReader(""), stdout: &stdout, stderr: &stderr, interactive: true}, &stdout, &stderr
}

func TestUnknownCommandPrintsUsageAndExits2(t *testing.T) {
	e, stdout, stderr := testEnv(t)
	if got := dispatch(e, []string{"frobnicate"}); got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), `unknown command "frobnicate"`) || !strings.Contains(stderr.String(), "Usage: agent-downlink") {
		t.Errorf("stderr = %q, want the unknown-command message and usage", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestNoCommandPrintsUsageAndExits2(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, nil); got != 2 {
		t.Fatalf("exit status = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "Usage: agent-downlink") {
		t.Errorf("stderr = %q, want usage", stderr.String())
	}
}

func TestHelpPrintsUsageToStdoutAndExits0(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		e, stdout, stderr := testEnv(t)
		if got := dispatch(e, []string{arg}); got != 0 {
			t.Fatalf("%s: exit status = %d, want 0", arg, got)
		}
		if !strings.Contains(stdout.String(), "Usage: agent-downlink") || stderr.Len() != 0 {
			t.Errorf("%s: stdout = %q, stderr = %q", arg, stdout.String(), stderr.String())
		}
	}
}

func TestEveryDocumentedCommandIsRegisteredAndInUsage(t *testing.T) {
	for _, name := range []string{"setup", "add-machine", "run", "push", "pull", "status", "timer"} {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q is not registered", name)
		}
		if !strings.Contains(usage, "\n  "+name) {
			t.Errorf("usage text does not list %q", name)
		}
	}
}
