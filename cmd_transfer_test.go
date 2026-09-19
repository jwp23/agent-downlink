package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
	"github.com/jwp23/agent-downlink/internal/status"
	"github.com/jwp23/agent-downlink/internal/transfer"
)

// configuredEnv is a machine that has been set up against a local-directory bucket.
func configuredEnv(t *testing.T, machine, bucket string, readable ...string) (env, func() string, func() string) {
	t.Helper()
	e, stdout, stderr := testEnv(t)
	paths := config.PathsFor(e.home)

	obscurer, err := rclone.New("", "/unused")
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	passwords := map[string]string{}
	for _, name := range append([]string{machine}, readable...) {
		passwords[name], err = obscurer.Obscure(context.Background(), "placeholder-password-"+name)
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.File{Machine: machine, Storage: bucket, Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	if err := cfg.Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	if err := (config.RcloneConf{Passwords: passwords}).Save(paths.RcloneConf, bucket); err != nil {
		t.Fatal(err)
	}
	return e, stdout.String, stderr.String
}

func writeRecord(t *testing.T, home, rel, content string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "projects", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunPushesAndPullsBetweenTwoConfiguredMachines(t *testing.T) {
	bucket := filepath.Join(t.TempDir(), "bucket")
	ws, _, wsErr := configuredEnv(t, "workstation", bucket, "laptop")
	lt, _, ltErr := configuredEnv(t, "laptop", bucket, "workstation")
	writeRecord(t, ws.home, "proj-a/s1.jsonl", "from workstation\n")
	writeRecord(t, lt.home, "proj-b/s2.jsonl", "from laptop\n")

	if got := dispatch(ws, []string{"push"}); got != 0 {
		t.Fatalf("workstation push: exit %d, stderr %q", got, wsErr())
	}
	if got := dispatch(lt, []string{"run"}); got != 0 {
		t.Fatalf("laptop run: exit %d, stderr %q", got, ltErr())
	}
	if got := dispatch(ws, []string{"pull"}); got != 0 {
		t.Fatalf("workstation pull: exit %d, stderr %q", got, wsErr())
	}

	for home, rel := range map[string]string{
		ws.home: "laptop/claude-code/projects/proj-b/s2.jsonl",
		lt.home: "workstation/claude-code/projects/proj-a/s1.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(config.PathsFor(home).DefaultMirror, filepath.FromSlash(rel))); err != nil {
			t.Errorf("mirror lacks %s: %v", rel, err)
		}
	}
	if wsErr() != "" || ltErr() != "" {
		t.Errorf("successful runs printed to stderr: %q / %q", wsErr(), ltErr())
	}
}

func TestRunExitsNonZeroWhenAStepFailedAndShowsItToAPerson(t *testing.T) {
	bucket := filepath.Join(t.TempDir(), "bucket")
	// laptop is readable but has never pushed, so pull:laptop fails.
	ws, stdout, stderr := configuredEnv(t, "workstation", bucket, "laptop")
	writeRecord(t, ws.home, "proj-a/s1.jsonl", "from workstation\n")

	if got := dispatch(ws, []string{"run"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr(), "pull:laptop FAILED") || stdout() != "" {
		t.Errorf("stderr = %q, stdout = %q", stderr(), stdout())
	}
	st, _ := status.Load(config.PathsFor(ws.home).StatusFile)
	if st.Steps["push"].Error != "" || st.Steps["pull:laptop"].Error == "" {
		t.Errorf("status = %+v", st.Steps)
	}
}

func TestUnderTheTimerNothingIsPrinted(t *testing.T) {
	bucket := filepath.Join(t.TempDir(), "bucket")
	ws, stdout, stderr := configuredEnv(t, "workstation", bucket, "laptop")
	ws.interactive = false
	writeRecord(t, ws.home, "proj-a/s1.jsonl", "from workstation\n")

	if got := dispatch(ws, []string{"run"}); got != 1 { // pull:laptop fails, as above
		t.Fatalf("exit status = %d, want 1", got)
	}
	if stdout() != "" || stderr() != "" {
		t.Errorf("printed under the timer: stdout %q, stderr %q", stdout(), stderr())
	}
	log, _ := os.ReadFile(config.PathsFor(ws.home).LogFile)
	if !strings.Contains(string(log), "pull:laptop  FAILED") {
		t.Errorf("the log must carry what was not printed:\n%s", log)
	}
}

func TestRefusesToRunOnLoosePermissions(t *testing.T) {
	ws, _, stderr := configuredEnv(t, "workstation", filepath.Join(t.TempDir(), "bucket"))
	paths := config.PathsFor(ws.home)
	if err := os.Chmod(paths.RcloneConf, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"run", "push", "pull"} {
		if got := dispatch(ws, []string{cmd}); got != 1 {
			t.Errorf("%s: exit status = %d, want 1", cmd, got)
		}
	}
	if !strings.Contains(stderr(), "chmod 600") {
		t.Errorf("stderr = %q, want the fix", stderr())
	}
	if _, err := os.Stat(paths.DefaultMirror); !os.IsNotExist(err) {
		t.Errorf("the cycle ran despite loose permissions (mirror exists, err = %v)", err)
	}
	log, _ := os.ReadFile(paths.LogFile)
	if !strings.Contains(string(log), "startup  FAILED") {
		t.Errorf("log = %q, want the startup failure", log)
	}
	st, _ := status.Load(paths.StatusFile)
	if st.Steps["startup"].Error == "" {
		t.Errorf("status = %+v, want a startup failure so it is visible to `agent-downlink status`", st.Steps)
	}
}

func TestUnconfiguredMachinePointsAtSetup(t *testing.T) {
	e, _, stderr := testEnv(t)
	if got := dispatch(e, []string{"run"}); got != 1 {
		t.Fatalf("exit status = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "agent-downlink setup") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestOverlappingRunExitsQuietlyButPushAndPullSaySo(t *testing.T) {
	ws, stdout, stderr := configuredEnv(t, "workstation", filepath.Join(t.TempDir(), "bucket"))
	release, acquired, err := transfer.Lock(config.PathsFor(ws.home).LockFile)
	if err != nil || !acquired {
		t.Fatal("could not take the lock")
	}
	defer release()

	if got := dispatch(ws, []string{"run"}); got != 0 || stdout() != "" || stderr() != "" {
		t.Errorf("run while locked: exit %d, stdout %q, stderr %q; want a silent 0", got, stdout(), stderr())
	}
	if got := dispatch(ws, []string{"push"}); got != 1 || !strings.Contains(stderr(), "another run is in progress") {
		t.Errorf("push while locked: exit %d, stderr %q", got, stderr())
	}
	if _, err := os.Stat(config.PathsFor(ws.home).DefaultMirror); !os.IsNotExist(err) {
		t.Errorf("a cycle ran while the lock was held (err = %v)", err)
	}
}
