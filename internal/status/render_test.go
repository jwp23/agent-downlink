package status

import (
	"strings"
	"testing"
	"time"
)

func TestRenderWithNoRuns(t *testing.T) {
	got := Render(File{}, t0, "/state/agent-downlink.log")
	want := "No runs recorded yet.\n\nLog: /state/agent-downlink.log\n"
	if got != want {
		t.Errorf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderShowsAgesErrorsAndTheLogPath(t *testing.T) {
	now := t0.Add(72 * time.Hour)
	var f File
	f.Record("push", now.Add(-12*time.Minute), true, "")
	f.Record("local-copy", now.Add(-12*time.Minute), true, "")
	f.Record("pull:laptop", now.Add(-50*time.Hour), true, "")
	f.Record("pull:laptop", now.Add(-12*time.Minute), false, "CRITICAL: connection refused\nsecond line")
	f.Record("pull:old-desktop", now.Add(-12*time.Minute), false, "ERROR: directory not found")

	got := Render(f, now, "/state/agent-downlink.log")
	want := strings.Join([]string{
		"STEP              LAST SUCCESS  STATE",
		"local-copy        12m ago       ok",
		"pull:laptop       2d ago        FAILING",
		"pull:old-desktop  never         FAILING",
		"push              12m ago       ok",
		"",
		"pull:laptop:",
		"    CRITICAL: connection refused",
		"    second line",
		"pull:old-desktop:",
		"    ERROR: directory not found",
		"",
		"Log: /state/agent-downlink.log",
		"",
	}, "\n")
	if got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
}

func TestAge(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second: "just now",
		59 * time.Minute: "59m ago",
		3 * time.Hour:    "3h ago",
		47 * time.Hour:   "47h ago",
		48 * time.Hour:   "2d ago",
		400 * time.Hour:  "16d ago",
	}
	for d, want := range cases {
		if got := age(d); got != want {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
}
