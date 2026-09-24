package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

func TestLoadOfAMissingFileIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "status.json"))
	if err != nil || len(f.Steps) != 0 {
		t.Errorf("Load = %+v, %v; want empty, nil", f, err)
	}
}

func TestRecordSuccessThenFailureThenRecovery(t *testing.T) {
	var f File
	f.Record("push", t0, true, "")
	if got := f.Steps["push"]; got != (Step{LastAttempt: t0, LastSuccess: t0}) {
		t.Errorf("after success: %+v", got)
	}

	f.Record("push", t0.Add(time.Hour), false, "CRITICAL: connection refused")
	want := Step{LastAttempt: t0.Add(time.Hour), LastSuccess: t0, Error: "CRITICAL: connection refused"}
	if got := f.Steps["push"]; got != want {
		t.Errorf("after failure: %+v\nwant %+v (the last success must survive a failure)", got, want)
	}

	f.Record("push", t0.Add(2*time.Hour), true, "")
	want = Step{LastAttempt: t0.Add(2 * time.Hour), LastSuccess: t0.Add(2 * time.Hour)}
	if got := f.Steps["push"]; got != want {
		t.Errorf("after recovery: %+v\nwant %+v (the error must clear)", got, want)
	}
}

func TestRecordKeepsStepsIndependent(t *testing.T) {
	var f File
	f.Record("push", t0, false, "boom")
	f.Record("pull:laptop", t0, true, "")
	if f.Steps["push"].Error != "boom" || f.Steps["pull:laptop"].Error != "" {
		t.Errorf("steps = %+v", f.Steps)
	}
}

func TestRecordTruncatesAVeryLongError(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "ERROR: file: Failed to copy")
	}
	var f File
	f.Record("push", t0, false, strings.Join(lines, "\n"))
	got := strings.Split(f.Steps["push"].Error, "\n")
	if len(got) != 21 || !strings.Contains(got[20], "480 more lines in the log") {
		t.Errorf("error has %d lines, last = %q; want 20 lines plus a pointer to the log", len(got), got[len(got)-1])
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "status.json")
	var want File
	want.Record("push", t0, true, "")
	want.Record("pull:laptop", t0, false, "CRITICAL: connection refused")
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v\nwant %+v", got, want)
	}
	for p, mode := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		info, _ := os.Stat(p)
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %04o, want %04o", p, info.Mode().Perm(), mode)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("%d files in the state directory, want only status.json", len(entries))
	}
}

func TestJSONShapeIsStableForConsumers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	var f File
	f.Record("push", t0, true, "")
	f.Record("pull:laptop", t0, false, "boom")
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var got map[string]map[string]map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	push, pull := got["steps"]["push"], got["steps"]["pull:laptop"]
	if push["last_attempt"] != "2026-03-01T09:00:00Z" || push["last_success"] != "2026-03-01T09:00:00Z" {
		t.Errorf("push = %v", push)
	}
	if _, has := push["error"]; has {
		t.Errorf("push carries an error key: %v", push)
	}
	if _, has := pull["last_success"]; has || pull["error"] != "boom" {
		t.Errorf("pull:laptop = %v, want no last_success and error boom", pull)
	}
}

func TestRecordKeepsAnErrorOfExactlyTheLimit(t *testing.T) {
	var lines []string
	for i := 0; i < maxErrorLines; i++ {
		lines = append(lines, "ERROR: file: Failed to copy")
	}
	text := strings.Join(lines, "\n")
	var f File
	f.Record("push", t0, false, text)
	if got := f.Steps["push"].Error; got != text {
		t.Errorf("error of %d lines was changed:\n%q", maxErrorLines, got)
	}
}
