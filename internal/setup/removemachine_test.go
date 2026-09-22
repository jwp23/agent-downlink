package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/status"
)

// readable sets up workstation with laptop readable, a mirror folder for each, and a status
// entry for each step, then returns the paths involved.
func readable(t *testing.T) (home string, paths config.Paths, mirror string) {
	t.Helper()
	home, _ = configured(t, "workstation", filepath.Join(t.TempDir(), "bucket"), "workstations-placeholder-password")
	if _, err := addMachine(t, home, "laptop", "laptops-placeholder-password\n"); err != nil {
		t.Fatal(err)
	}
	paths = config.PathsFor(home)
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	mirror = cfg.Mirror
	write(t, filepath.Join(mirror, "laptop", "claude-code", "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	write(t, filepath.Join(mirror, "workstation", "claude-code", "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	var st status.File
	st.Record("push", time.Now(), true, "")
	st.Record("pull:laptop", time.Now(), true, "")
	if err := st.Save(paths.StatusFile); err != nil {
		t.Fatal(err)
	}
	return home, paths, mirror
}

func removeMachine(t *testing.T, home, machine, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RemoveMachine(NewPrompter(strings.NewReader(input), &out), home, machine)
	return out.String(), err
}

func snapshot(t *testing.T, paths config.Paths) (rcloneConf, statusFile []byte) {
	t.Helper()
	rcloneConf, err := os.ReadFile(paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	statusFile, err = os.ReadFile(paths.StatusFile)
	if err != nil {
		t.Fatal(err)
	}
	return rcloneConf, statusFile
}

func TestRemoveMachineForgetsTheMachineHere(t *testing.T) {
	home, paths, mirror := readable(t)
	out, err := removeMachine(t, home, "laptop", "yes\n")
	if err != nil {
		t.Fatalf("RemoveMachine = %v", err)
	}
	for _, want := range []string{"Remove laptop here?", filepath.Join(mirror, "laptop"), "laptop is no longer readable here", "bucket"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}

	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	if got := secrets.Machines(); len(got) != 1 || got[0] != "workstation" {
		t.Errorf("machines = %v, want only workstation", got)
	}
	if err := config.CheckPermissions(paths); err != nil {
		t.Errorf("CheckPermissions after remove-machine = %v", err)
	}

	st, err := status.Load(paths.StatusFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := st.Steps["pull:laptop"]; present {
		t.Error("status.json still has pull:laptop")
	}
	if _, present := st.Steps["push"]; !present {
		t.Error("status.json lost the push step")
	}

	if _, err := os.Stat(filepath.Join(mirror, "laptop")); !os.IsNotExist(err) {
		t.Errorf("mirror/laptop still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mirror, "workstation", "claude-code", "projects", "proj-a", "s1.jsonl")); err != nil {
		t.Errorf("mirror/workstation was touched: %v", err)
	}
}

func TestRemoveMachineWithoutAMirrorFolderStillForgetsIt(t *testing.T) {
	home, paths, mirror := readable(t)
	if err := os.RemoveAll(filepath.Join(mirror, "laptop")); err != nil {
		t.Fatal(err)
	}
	if _, err := removeMachine(t, home, "laptop", "yes\n"); err != nil {
		t.Fatalf("RemoveMachine = %v", err)
	}
	secrets, _ := config.LoadRcloneConf(paths.RcloneConf)
	if len(secrets.Machines()) != 1 {
		t.Errorf("machines = %v, want only workstation", secrets.Machines())
	}
}

func TestRemoveMachineRefusesWithoutAYes(t *testing.T) {
	home, paths, mirror := readable(t)
	confBefore, statusBefore := snapshot(t, paths)

	_, err := removeMachine(t, home, "laptop", "no\n")
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("RemoveMachine = %v, want a refusal", err)
	}

	confAfter, statusAfter := snapshot(t, paths)
	if !bytes.Equal(confBefore, confAfter) {
		t.Error("rclone.conf changed without a yes")
	}
	if !bytes.Equal(statusBefore, statusAfter) {
		t.Error("status.json changed without a yes")
	}
	if _, err := os.Stat(filepath.Join(mirror, "laptop", "claude-code", "projects", "proj-b", "s2.jsonl")); err != nil {
		t.Errorf("mirror/laptop changed without a yes: %v", err)
	}
}

func TestRemoveMachineRejectsBadInput(t *testing.T) {
	home, paths, _ := readable(t)
	confBefore, statusBefore := snapshot(t, paths)
	cases := []struct{ machine, want string }{
		{"Bad Name", "lowercase letters, digits, and hyphens"},
		{"workstation", "this machine"},
		{"desktop", "not readable here"},
	}
	for _, tc := range cases {
		if _, err := removeMachine(t, home, tc.machine, "yes\n"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("RemoveMachine(%q) = %v, want an error containing %q", tc.machine, err, tc.want)
		}
	}
	confAfter, statusAfter := snapshot(t, paths)
	if !bytes.Equal(confBefore, confAfter) || !bytes.Equal(statusBefore, statusAfter) {
		t.Error("a refused remove-machine changed a file")
	}
}

func TestRemoveMachineOnAnUnconfiguredMachinePointsAtSetup(t *testing.T) {
	_, err := removeMachine(t, t.TempDir(), "laptop", "yes\n")
	if err == nil || !strings.Contains(err.Error(), "agent-downlink setup") {
		t.Errorf("RemoveMachine = %v", err)
	}
}

func TestRemoveMachinePropagatesLoadErrors(t *testing.T) {
	cases := []struct {
		name    string
		corrupt func(paths config.Paths)
	}{
		{"config.toml", func(paths config.Paths) { write(t, paths.ConfigFile, "not valid toml [[[") }},
		{"rclone.conf", func(paths config.Paths) { write(t, paths.RcloneConf, "not valid rclone.conf [[[") }},
		{"status.json", func(paths config.Paths) { write(t, paths.StatusFile, "not json") }},
	}
	for _, tc := range cases {
		home, paths, _ := readable(t)
		tc.corrupt(paths)
		if _, err := removeMachine(t, home, "laptop", "yes\n"); err == nil {
			t.Errorf("%s: RemoveMachine = nil error, want the parse error", tc.name)
		}
	}
}

func TestRemoveMachinePropagatesConfirmError(t *testing.T) {
	home, _, _ := readable(t)
	if _, err := removeMachine(t, home, "laptop", ""); err == nil || !strings.Contains(err.Error(), "input ended before every question was answered") {
		t.Errorf("RemoveMachine with no input = %v", err)
	}
}
