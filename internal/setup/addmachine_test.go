package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
)

// configured sets up a machine against a local-directory bucket with a known password.
func configured(t *testing.T, machine, bucket, password string) (home string, runner *rclone.Runner) {
	t.Helper()
	home = t.TempDir()
	paths := config.PathsFor(home)
	obscurer, err := rclone.New("", "/unused")
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	obscured, err := obscurer.Obscure(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.File{Machine: machine, Storage: bucket, Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	if err := cfg.Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	if err := (config.RcloneConf{Passwords: map[string]string{machine: obscured}}).Save(paths.RcloneConf, bucket); err != nil {
		t.Fatal(err)
	}
	runner, err = rclone.New("", paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	return home, runner
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func addMachine(t *testing.T, home, machine, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := AddMachine(context.Background(), NewPrompter(strings.NewReader(input), &out), home, machine)
	return out.String(), err
}

func TestAddMachineMakesAnotherMachinesRecordsReadable(t *testing.T) {
	ctx := context.Background()
	bucket := filepath.Join(t.TempDir(), "bucket")

	// laptop pushes a record, encrypted with its own password.
	_, laptop := configured(t, "laptop", bucket, "laptops-placeholder-password")
	src := t.TempDir()
	write(t, filepath.Join(src, "claude-code", "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	if res := laptop.Copy(ctx, src, config.CryptRemote("laptop")); res.Outcome != rclone.Success {
		t.Fatalf("laptop push: %+v", res)
	}

	wsHome, workstation := configured(t, "workstation", bucket, "workstations-placeholder-password")
	out, err := addMachine(t, wsHome, "laptop", "laptops-placeholder-password\n")
	if err != nil {
		t.Fatalf("AddMachine = %v", err)
	}
	if strings.Contains(out, "laptops-placeholder-password") || !strings.Contains(out, "laptop is now readable") {
		t.Errorf("output = %q", out)
	}

	pulled := t.TempDir()
	if res := workstation.Copy(ctx, config.CryptRemote("laptop"), pulled); res.Outcome != rclone.Success {
		t.Fatalf("workstation pull of laptop: %+v", res)
	}
	got, err := os.ReadFile(filepath.Join(pulled, "claude-code", "projects", "proj-b", "s2.jsonl"))
	if err != nil || string(got) != "from laptop\n" {
		t.Errorf("decrypted record = %q, %v", got, err)
	}

	paths := config.PathsFor(wsHome)
	raw, _ := os.ReadFile(paths.RcloneConf)
	if strings.Contains(string(raw), "laptops-placeholder-password") {
		t.Error("rclone.conf holds the password in plaintext")
	}
	if err := config.CheckPermissions(paths); err != nil {
		t.Errorf("CheckPermissions after add-machine = %v", err)
	}
	secrets, _ := config.LoadRcloneConf(paths.RcloneConf)
	if len(secrets.Machines()) != 2 {
		t.Errorf("machines = %v, want workstation's own password kept", secrets.Machines())
	}
}

func TestAddMachineRejectsBadInput(t *testing.T) {
	home, _ := configured(t, "workstation", filepath.Join(t.TempDir(), "bucket"), "placeholder-password")
	cases := []struct{ machine, input, want string }{
		{"Bad Name", "pw\n", "lowercase letters, digits, and hyphens"},
		{"laptop", "\n", "password is empty"},
	}
	for _, tc := range cases {
		if _, err := addMachine(t, home, tc.machine, tc.input); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("AddMachine(%q) = %v, want an error containing %q", tc.machine, err, tc.want)
		}
	}
}

func TestAddMachineOnAnUnconfiguredMachinePointsAtSetup(t *testing.T) {
	_, err := addMachine(t, t.TempDir(), "laptop", "pw\n")
	if err == nil || !strings.Contains(err.Error(), "agent-downlink setup") {
		t.Errorf("AddMachine = %v", err)
	}
}

func TestAddMachineAsksBeforeReplacingAPassword(t *testing.T) {
	home, _ := configured(t, "workstation", filepath.Join(t.TempDir(), "bucket"), "placeholder-password")
	if _, err := addMachine(t, home, "laptop", "first-password\n"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(config.PathsFor(home).RcloneConf)

	if _, err := addMachine(t, home, "laptop", "no\n"); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("AddMachine = %v, want a refusal", err)
	}
	after, _ := os.ReadFile(config.PathsFor(home).RcloneConf)
	if !bytes.Equal(before, after) {
		t.Error("rclone.conf changed without a yes")
	}

	if _, err := addMachine(t, home, "laptop", "yes\nsecond-password\n"); err != nil {
		t.Errorf("AddMachine with yes = %v", err)
	}
}
