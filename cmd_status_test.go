package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/status"
)

func TestStatusCommandReportsRecordedSteps(t *testing.T) {
	e, stdout, stderr := testEnv(t)
	paths := config.PathsFor(e.home)
	var f status.File
	f.Record("push", time.Now().Add(-3*time.Hour), true, "")
	f.Record("pull:laptop", time.Now(), false, "CRITICAL: connection refused")
	if err := f.Save(paths.StatusFile); err != nil {
		t.Fatal(err)
	}

	if got := dispatch(e, []string{"status"}); got != 0 {
		t.Fatalf("exit status = %d, stderr = %q", got, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"push", "3h ago", "pull:laptop", "never", "FAILING", "CRITICAL: connection refused", "Log: " + paths.LogFile} {
		if !strings.Contains(out, want) {
			t.Errorf("status output lacks %q:\n%s", want, out)
		}
	}
}

func TestStatusCommandBeforeAnyRun(t *testing.T) {
	e, stdout, _ := testEnv(t)
	if got := dispatch(e, []string{"status"}); got != 0 {
		t.Fatalf("exit status = %d", got)
	}
	if !strings.Contains(stdout.String(), "No runs recorded yet.") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
