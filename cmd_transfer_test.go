package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

	obscurer, err := rclone.New("", "/unused", rclone.Concurrency{})
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

// writeHistory gives a machine its history.jsonl, another built-in path a successful push
// needs present: a machine with project records to push has typed prompts too.
func writeHistory(t *testing.T, home, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".claude", "history.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunPushesAndPullsBetweenTwoConfiguredMachines(t *testing.T) {
	bucket := filepath.Join(t.TempDir(), "bucket")
	ws, _, wsErr := configuredEnv(t, "workstation", bucket, "laptop")
	lt, _, ltErr := configuredEnv(t, "laptop", bucket, "workstation")
	writeRecord(t, ws.home, "proj-a/s1.jsonl", "from workstation\n")
	writeHistory(t, ws.home, "typed prompt history\n")
	writeRecord(t, lt.home, "proj-b/s2.jsonl", "from laptop\n")
	writeHistory(t, lt.home, "typed prompt history\n")

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
	writeHistory(t, ws.home, "typed prompt history\n")

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
	writeHistory(t, ws.home, "typed prompt history\n")

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

func TestStartupFailureClearsAfterRecovery(t *testing.T) {
	ws, _, wsErr := configuredEnv(t, "workstation", filepath.Join(t.TempDir(), "bucket"))
	writeRecord(t, ws.home, "proj-a/s1.jsonl", "from workstation\n")
	writeHistory(t, ws.home, "typed prompt history\n")
	paths := config.PathsFor(ws.home)
	if err := os.Chmod(paths.RcloneConf, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dispatch(ws, []string{"push"}); got != 1 {
		t.Fatalf("push with loose permissions: exit %d, want 1", got)
	}
	st, _ := status.Load(paths.StatusFile)
	if st.Steps["startup"].Error == "" {
		t.Fatal("startup failure was not recorded")
	}

	if err := os.Chmod(paths.RcloneConf, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := dispatch(ws, []string{"push"}); got != 0 {
		t.Fatalf("push after fixing permissions: exit %d, stderr %q", got, wsErr())
	}
	st, _ = status.Load(paths.StatusFile)
	if st.Steps["startup"].Error != "" {
		t.Errorf("status = %+v, want the startup failure cleared after recovery", st.Steps)
	}
}

// recordingRclone writes a wrapper around the installed rclone that appends every argument it
// is started with, one per line, to the log file it returns.
func recordingRclone(t *testing.T, dir string) (wrapper, argLog string) {
	t.Helper()
	rcloneBinary, err := exec.LookPath("rclone")
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	argLog = filepath.Join(dir, "args.log")
	wrapper = filepath.Join(dir, "rclone-recording-args")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" >> %q\nexec %q \"$@\"\n", argLog, rcloneBinary)
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return wrapper, argLog
}

// useRclone points a configured machine at a particular rclone binary and copy parallelism.
func useRclone(t *testing.T, e env, binary string, transfers int) {
	t.Helper()
	path := config.PathsFor(e.home).ConfigFile
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rclone, cfg.Transfers = binary, transfers
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
}

func TestPullTakesItsParallelismFromConfigAndTheFlagOverridesIt(t *testing.T) {
	bucket := filepath.Join(t.TempDir(), "bucket")
	lt, _, ltErr := configuredEnv(t, "laptop", bucket)
	writeRecord(t, lt.home, "proj-b/s2.jsonl", "from laptop\n")
	writeHistory(t, lt.home, "typed prompt history\n")
	if got := dispatch(lt, []string{"push"}); got != 0 {
		t.Fatalf("laptop push: exit %d, stderr %q", got, ltErr())
	}

	ws, _, wsErr := configuredEnv(t, "workstation", bucket, "laptop")
	wrapper, argLog := recordingRclone(t, t.TempDir())
	useRclone(t, ws, wrapper, 2)

	for _, tc := range []struct {
		args          []string
		wantTransfers string
	}{
		{[]string{"pull"}, "2"},
		{[]string{"pull", "--transfers=7"}, "7"},
	} {
		if err := os.Remove(argLog); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if got := dispatch(ws, tc.args); got != 0 {
			t.Fatalf("%v: exit %d, stderr %q", tc.args, got, wsErr())
		}
		logged, err := os.ReadFile(argLog)
		if err != nil {
			t.Fatal(err)
		}
		// checkers is unset in config.toml, so every pull keeps rclone's own default.
		for _, want := range []string{"--transfers\n" + tc.wantTransfers + "\n", "--checkers\n8\n"} {
			if !strings.Contains(string(logged), want) {
				t.Errorf("%v: rclone was not given %q:\n%s", tc.args, strings.TrimSuffix(want, "\n"), logged)
			}
		}
	}
}

func TestPullRejectsAnUnusableTransfersFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pull", "--transfers=nope"}, `invalid value "nope" for flag -transfers`},
		{[]string{"pull", "--transfers=-1"}, "--transfers must be 1 or more"},
		{[]string{"pull", "--transfers=2", "extra"}, "usage: agent-downlink pull [--transfers=N]"},
	} {
		e, stdout, stderr := testEnv(t)
		if got := dispatch(e, tc.args); got != 2 {
			t.Errorf("%v: exit status = %d, want 2", tc.args, got)
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%v: stderr = %q, want it to contain %q", tc.args, stderr.String(), tc.want)
		}
		if stdout.String() != "" {
			t.Errorf("%v: stdout = %q, want nothing", tc.args, stdout.String())
		}
	}
}

func TestTransferCommandsRejectExtraArgs(t *testing.T) {
	for _, cmd := range []string{"run", "push", "pull"} {
		e, _, stderr := testEnv(t)
		if got := dispatch(e, []string{cmd, "extra"}); got != 2 {
			t.Errorf("%s: exit status = %d, want 2", cmd, got)
		}
		if want := "usage: agent-downlink " + cmd; !strings.Contains(stderr.String(), want) {
			t.Errorf("%s: stderr = %q, want %q", cmd, stderr.String(), want)
		}
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
