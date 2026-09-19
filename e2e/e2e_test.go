//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
	"github.com/jwp23/agent-downlink/internal/runlog"
	"github.com/jwp23/agent-downlink/internal/transfer"
)

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("%s is not set; see e2e/README.md", name)
	}
	return v
}

type machine struct {
	name   string
	claude string
	paths  config.Paths
	cycle  *transfer.Cycle
	errors *bytes.Buffer
}

// newMachine sets up one simulated machine against the real bucket. passwords maps every
// machine this one can read (itself included) to a plaintext placeholder password.
func newMachine(t *testing.T, name, bucket string, key config.B2, passwords map[string]string) *machine {
	t.Helper()
	home := t.TempDir()
	paths := config.PathsFor(home)
	obscurer, err := rclone.New("", "/unused")
	if err != nil {
		t.Fatalf("the suite needs rclone: %v", err)
	}
	conf := config.RcloneConf{B2: &key, Passwords: map[string]string{}}
	var readable []string
	for other, password := range passwords {
		if conf.Passwords[other], err = obscurer.Obscure(context.Background(), password); err != nil {
			t.Fatal(err)
		}
		if other != name {
			readable = append(readable, other)
		}
	}
	if err := conf.Save(paths.RcloneConf, "b2:"+bucket); err != nil {
		t.Fatal(err)
	}
	runner, err := rclone.New("", paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	m := &machine{name: name, claude: filepath.Join(home, ".claude"), paths: paths, errors: &bytes.Buffer{}}
	m.cycle = &transfer.Cycle{
		Machine:    name,
		Mirror:     paths.DefaultMirror,
		Sources:    []config.Source{{Tool: "claude-code", Root: m.claude, Paths: []string{"projects"}}},
		Readable:   readable,
		Runner:     runner,
		StatusPath: paths.StatusFile,
		Log:        runlog.New(paths.LogFile),
		Now:        time.Now,
		Errors:     m.errors,
	}
	return m
}

func (m *machine) write(t *testing.T, rel, content string, appendTo bool) {
	t.Helper()
	path := filepath.Join(m.claude, "projects", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendTo {
		flags = os.O_APPEND | os.O_WRONLY
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func mustSucceed(t *testing.T, m *machine, ok bool) {
	t.Helper()
	if !ok {
		log, _ := os.ReadFile(m.paths.LogFile)
		t.Fatalf("%s: a step failed: %s\nlog:\n%s", m.name, m.errors.String(), log)
	}
}

// storedVersions counts every stored version of every file in a machine's area, old versions
// included. This is the one place outside internal/rclone that executes rclone: it is a test
// probe of the provider's versioning, not something the tool does. The storage key reaches
// rclone through the config file, never the command line.
func storedVersions(t *testing.T, m *machine, bucket string) int {
	t.Helper()
	runner, err := rclone.New("", m.paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(runner.Binary(), "--config", m.paths.RcloneConf, "lsf", "-R", "--files-only", "--b2-versions", "b2:"+bucket+"/"+m.name)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing versions failed: %v", err)
	}
	return len(strings.Fields(string(out)))
}

func names() (a, b string) {
	stamp := time.Now().Unix()
	return fmt.Sprintf("e2e-%d-a", stamp), fmt.Sprintf("e2e-%d-b", stamp)
}

func TestNoDeleteKeyPushesAndASecondMachinePullsAndDecrypts(t *testing.T) {
	ctx := context.Background()
	bucket := requireEnv(t, "AGENT_DOWNLINK_E2E_BUCKET")
	key := config.B2{Account: requireEnv(t, "AGENT_DOWNLINK_E2E_KEY_ID"), Key: requireEnv(t, "AGENT_DOWNLINK_E2E_KEY")}
	nameA, nameB := names()
	passwords := map[string]string{nameA: "placeholder-password-a", nameB: "placeholder-password-b"}
	a := newMachine(t, nameA, bucket, key, passwords)
	b := newMachine(t, nameB, bucket, key, passwords)
	a.write(t, "proj-a/s1.jsonl", "from a\n", false)
	b.write(t, "proj-b/s2.jsonl", "from b\n", false)

	mustSucceed(t, a, a.cycle.Push(ctx))
	mustSucceed(t, b, b.cycle.Run(ctx))
	mustSucceed(t, a, a.cycle.Run(ctx))

	for _, check := range []struct {
		m         *machine
		rel, want string
	}{
		{b, nameA + "/claude-code/projects/proj-a/s1.jsonl", "from a\n"},
		{a, nameB + "/claude-code/projects/proj-b/s2.jsonl", "from b\n"},
	} {
		got, err := os.ReadFile(filepath.Join(check.m.paths.DefaultMirror, filepath.FromSlash(check.rel)))
		if err != nil || string(got) != check.want {
			t.Errorf("%s: %s = %q, %v", check.m.name, check.rel, got, err)
		}
	}
}

func TestAnAppendedFileIsCompletedAndTheOverwriteKeepsThePriorVersion(t *testing.T) {
	ctx := context.Background()
	bucket := requireEnv(t, "AGENT_DOWNLINK_E2E_BUCKET")
	key := config.B2{Account: requireEnv(t, "AGENT_DOWNLINK_E2E_KEY_ID"), Key: requireEnv(t, "AGENT_DOWNLINK_E2E_KEY")}
	nameA, nameB := names()
	passwords := map[string]string{nameA: "placeholder-password-a", nameB: "placeholder-password-b"}
	a := newMachine(t, nameA, bucket, key, passwords)
	b := newMachine(t, nameB, bucket, key, passwords)
	b.write(t, "proj-b/s2.jsonl", "from b\n", false)
	mustSucceed(t, b, b.cycle.Push(ctx))

	a.write(t, "proj-a/growing.jsonl", "line 1\n", false)
	mustSucceed(t, a, a.cycle.Run(ctx))
	if got := storedVersions(t, a, bucket); got != 1 {
		t.Fatalf("after the first push the area holds %d versions, want 1", got)
	}

	a.write(t, "proj-a/growing.jsonl", "line 2\n", true)
	mustSucceed(t, a, a.cycle.Run(ctx))
	if got := storedVersions(t, a, bucket); got != 2 {
		t.Errorf("after the overwrite the area holds %d versions, want 2: the prior version must stay retrievable", got)
	}

	mustSucceed(t, b, b.cycle.Run(ctx))
	got, err := os.ReadFile(filepath.Join(b.paths.DefaultMirror, nameA, "claude-code", "projects", "proj-a", "growing.jsonl"))
	if err != nil || string(got) != "line 1\nline 2\n" {
		t.Errorf("the reader's copy = %q, %v; want the completed file", got, err)
	}
}

// TestPrefixRestrictedKeyCanPush settles an open question from the epic. A failure here is a
// finding, not a defect in the tool: record it as the test's message directs.
func TestPrefixRestrictedKeyCanPush(t *testing.T) {
	ctx := context.Background()
	bucket := requireEnv(t, "AGENT_DOWNLINK_E2E_BUCKET")
	name := requireEnv(t, "AGENT_DOWNLINK_E2E_PREFIX")
	key := config.B2{Account: requireEnv(t, "AGENT_DOWNLINK_E2E_PREFIX_KEY_ID"), Key: requireEnv(t, "AGENT_DOWNLINK_E2E_PREFIX_KEY")}
	m := newMachine(t, name, bucket, key, map[string]string{name: "placeholder-password-prefix"})
	m.write(t, fmt.Sprintf("proj/%d.jsonl", time.Now().Unix()), "pushed with a prefix-restricted key\n", false)

	if !m.cycle.Push(ctx) {
		log, _ := os.ReadFile(m.paths.LogFile)
		t.Fatalf("rclone could not push with a key restricted to the prefix %q. Use the fallback from the epic "+
			"(a whole-bucket no-delete key, deleted after the retiring machine's one push) and record it in a new ADR "+
			"that supersedes that clause of ADR-002.\nlog:\n%s", name+"/", log)
	}
}
