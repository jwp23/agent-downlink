package setup

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/config"
)

func TestListMachinesSortsAndMarksThisMachine(t *testing.T) {
	home := t.TempDir()
	paths := config.PathsFor(home)
	cfg := config.File{Machine: "laptop", Storage: "b2:scratch-bucket", Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	if err := cfg.Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	secrets := config.RcloneConf{Passwords: map[string]string{
		"workstation": "obscured-placeholder-1",
		"laptop":      "obscured-placeholder-2",
		"desktop":     "obscured-placeholder-3",
	}}
	if err := secrets.Save(paths.RcloneConf, "b2:scratch-bucket"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := ListMachines(&out, home); err != nil {
		t.Fatal(err)
	}
	want := "desktop\nlaptop (this machine)\nworkstation\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestListMachinesBeforeSetupFails(t *testing.T) {
	err := ListMachines(io.Discard, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "agent-downlink setup") {
		t.Errorf("error = %v, want one that points at setup", err)
	}
}
