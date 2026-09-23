package runlog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 3, 1, 2, 0, 0, 0, time.FixedZone("west", -7*3600)) // 2026-03-01T09:00:00Z

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAppendWritesOneLinePerStepInUTC(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "state", "agent-downlink.log"))
	if err := l.Append(at, "push", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(at.Add(time.Second), "pull:laptop", "ok", ""); err != nil {
		t.Fatal(err)
	}
	want := "2026-03-01T09:00:00Z  push  ok\n2026-03-01T09:00:01Z  pull:laptop  ok\n"
	if got := read(t, l.Path); got != want {
		t.Errorf("log =\n%s\nwant\n%s", got, want)
	}
}

func TestFailedStepCarriesItsFullDetailIndented(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "agent-downlink.log"))
	detail := "ERROR: a.jsonl: Failed to copy: permission denied\nERROR: Attempt 1/1 failed with 1 errors\n"
	if err := l.Append(at, "push", "FAILED", detail); err != nil {
		t.Fatal(err)
	}
	want := "2026-03-01T09:00:00Z  push  FAILED\n" +
		"    ERROR: a.jsonl: Failed to copy: permission denied\n" +
		"    ERROR: Attempt 1/1 failed with 1 errors\n"
	if got := read(t, l.Path); got != want {
		t.Errorf("log =\n%q\nwant\n%q", got, want)
	}
}

func TestARecoveredStepKeepsItsFailureHistory(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "agent-downlink.log"))
	if err := l.Append(at, "push", "FAILED", "CRITICAL: connection refused"); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(at.Add(time.Hour), "push", "ok", ""); err != nil {
		t.Fatal(err)
	}
	got := read(t, l.Path)
	if !strings.Contains(got, "push  FAILED\n    CRITICAL: connection refused\n") || !strings.Contains(got, "push  ok\n") {
		t.Errorf("log lost history:\n%s", got)
	}
}

func TestLogAndItsDirectoryArePrivate(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "state", "agent-downlink.log"))
	if err := l.Append(at, "push", "ok", ""); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{filepath.Dir(l.Path): 0o700, l.Path: 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %04o, want %04o", p, got, want)
		}
	}
}

func TestRotatesAtTheSizeLimitKeepingOneOldFile(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "agent-downlink.log"))
	l.MaxBytes = 100
	entry := func(i int) string { return strings.Repeat(string(rune('a'+i)), 40) } // each entry is ~75 bytes

	if err := l.Append(at, "push", "FAILED", entry(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rotated before reaching the limit (err = %v)", err)
	}

	if err := l.Append(at, "push", "FAILED", entry(1)); err != nil { // would pass 100 bytes: rotate first
		t.Fatal(err)
	}
	if got := read(t, l.Path+".1"); !strings.Contains(got, entry(0)) {
		t.Errorf(".1 = %q, want the first entry", got)
	}
	if got := read(t, l.Path); !strings.Contains(got, entry(1)) || strings.Contains(got, entry(0)) {
		t.Errorf("new log = %q, want only the second entry", got)
	}

	if err := l.Append(at, "push", "FAILED", entry(2)); err != nil { // rotates again: .1 is replaced
		t.Fatal(err)
	}
	if got := read(t, l.Path+".1"); !strings.Contains(got, entry(1)) || strings.Contains(got, entry(0)) {
		t.Errorf(".1 = %q, want only the second entry", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(l.Path))
	if len(entries) != 2 {
		t.Errorf("%d files in the log directory, want 2 (the log and one old file)", len(entries))
	}
}

func TestAnEntryLargerThanTheLimitIsStillWritten(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "agent-downlink.log"))
	l.MaxBytes = 10
	if err := l.Append(at, "push", "FAILED", strings.Repeat("x", 100)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, l.Path); !strings.Contains(got, strings.Repeat("x", 100)) {
		t.Errorf("log = %q", got)
	}
}

func TestDefaultLimitIsFiveMiB(t *testing.T) {
	if got := New("x").MaxBytes; got != 5<<20 {
		t.Errorf("MaxBytes = %d, want %d", got, 5<<20)
	}
}

func TestAnEntryThatLandsExactlyOnTheLimitDoesNotRotate(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "agent-downlink.log"))
	const entryBytes = len("2026-03-01T09:00:00Z  push  ok\n")
	l.MaxBytes = int64(2 * entryBytes)
	if err := l.Append(at, "push", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(at, "push", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rotated although the second entry fits exactly (err = %v)", err)
	}
	if got := read(t, l.Path); int64(len(got)) != l.MaxBytes {
		t.Errorf("log is %d bytes, want %d", len(got), l.MaxBytes)
	}
}

// TestAppendReportsAFailedWrite uses /dev/full, which accepts the open and fails every
// write with ENOSPC, so the write error path runs without faking the file system.
func TestAppendReportsAFailedWrite(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/dev/full exists only on Linux")
	}
	l := &Log{Path: "/dev/full", MaxBytes: 5 << 20}
	err := l.Append(at, "push", "ok", "")
	if err == nil || !strings.Contains(err.Error(), "no space left on device") {
		t.Errorf("Append to /dev/full = %v, want the write error", err)
	}
}
