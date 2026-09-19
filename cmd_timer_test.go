package main

import (
	"strings"
	"testing"
)

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
