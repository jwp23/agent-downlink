package transfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
	"github.com/jwp23/agent-downlink/internal/runlog"
	"github.com/jwp23/agent-downlink/internal/status"
)

type machine struct {
	name   string
	claude string // the agent's data directory on this machine
	paths  config.Paths
	cycle  *Cycle
	errors *bytes.Buffer
}

// newMachines simulates machines that share one bucket (a local directory) and can all read
// each other. Machines named in neverPushes get a password everywhere but no machine of their
// own, like a machine that was added as readable before its first push.
func newMachines(t *testing.T, names []string, neverPushes ...string) (bucket string, machines map[string]*machine) {
	t.Helper()
	ctx := context.Background()
	bucket = filepath.Join(t.TempDir(), "bucket")

	obscurer, err := rclone.New("", "/unused", rclone.Concurrency{})
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	passwords := map[string]string{}
	for _, name := range append(append([]string{}, names...), neverPushes...) {
		passwords[name], err = obscurer.Obscure(ctx, "placeholder-password-"+name)
		if err != nil {
			t.Fatal(err)
		}
	}

	clock := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	machines = map[string]*machine{}
	for _, name := range names {
		home := t.TempDir()
		paths := config.PathsFor(home)
		if err := (config.RcloneConf{Passwords: passwords}).Save(paths.RcloneConf, bucket); err != nil {
			t.Fatal(err)
		}
		runner, err := rclone.New("", paths.RcloneConf, rclone.Concurrency{})
		if err != nil {
			t.Fatal(err)
		}
		var readable []string
		for other := range passwords {
			if other != name {
				readable = append(readable, other)
			}
		}
		sort.Strings(readable) // map order is random; the cycle pulls in the order it is given
		m := &machine{name: name, claude: filepath.Join(home, ".claude"), paths: paths, errors: &bytes.Buffer{}}
		m.cycle = &Cycle{
			Machine:    name,
			Mirror:     paths.DefaultMirror,
			Sources:    []config.Source{{Tool: "claude-code", Root: m.claude, Paths: []string{"projects"}, PluginManifest: "plugins/installed_plugins.json"}},
			Readable:   readable,
			Runner:     runner,
			StatusPath: paths.StatusFile,
			Log:        runlog.New(paths.LogFile),
			Now:        func() time.Time { clock = clock.Add(time.Minute); return clock },
			Errors:     m.errors,
		}
		machines[name] = m
	}
	return bucket, machines
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

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// chmodDirs sets the mode of dir and every directory below it.
func chmodDirs(t *testing.T, dir string, mode fs.FileMode) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return os.Chmod(path, mode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustSucceed(t *testing.T, m *machine, ok bool) {
	t.Helper()
	if !ok || m.errors.Len() != 0 {
		t.Fatalf("%s: run ok = %v, errors = %q\nlog:\n%s", m.name, ok, m.errors.String(), mustRead(t, m.paths.LogFile))
	}
}

func TestTwoMachinesSeeEachOthersRecordsInTheDocumentedLayout(t *testing.T) {
	ctx := context.Background()
	_, ms := newMachines(t, []string{"workstation", "laptop"})
	ws, lt := ms["workstation"], ms["laptop"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	write(t, filepath.Join(ws.claude, "plugins", "installed_plugins.json"), `{"a":"1.0.0"}`)
	write(t, filepath.Join(ws.claude, ".credentials.json"), "must never be archived")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")

	// Push is the local-copy and push steps only, so the plugin timeline reaches the bucket
	// with workstation's first full Run; laptop sees it on the pull after that.
	mustSucceed(t, ws, ws.cycle.Push(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))
	mustSucceed(t, ws, ws.cycle.Run(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))

	for _, m := range []*machine{ws, lt} {
		assertBothMachinesInTheMirror(t, m)
	}

	info, err := os.Stat(ws.paths.DefaultMirror)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("mirror mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}

	st, err := status.Load(ws.paths.StatusFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"plugins", "local-copy", "push", "pull:laptop"} {
		if s, ok := st.Steps[step]; !ok || s.Error != "" || s.LastSuccess.IsZero() {
			t.Errorf("status step %q = %+v, recorded = %v; want a success", step, s, ok)
		}
	}
	log := mustRead(t, ws.paths.LogFile)
	for _, want := range []string{"  plugins  ok\n", "  local-copy  ok\n", "  push  ok\n", "  pull:laptop  ok\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
}

// assertBothMachinesInTheMirror checks one machine's mirror against the layout contract:
// <machine>/<tool>/<the tool's native tree>, holding only the paths a source lists.
func assertBothMachinesInTheMirror(t *testing.T, m *machine) {
	t.Helper()
	mirror := m.paths.DefaultMirror
	if got := mustRead(t, filepath.Join(mirror, "workstation", "claude-code", "projects", "proj-a", "s1.jsonl")); got != "from workstation\n" {
		t.Errorf("%s: workstation record = %q", m.name, got)
	}
	if got := mustRead(t, filepath.Join(mirror, "laptop", "claude-code", "projects", "proj-b", "s2.jsonl")); got != "from laptop\n" {
		t.Errorf("%s: laptop record = %q", m.name, got)
	}
	copies, err := os.ReadDir(filepath.Join(mirror, "workstation", "claude-code", "plugins", "installed_plugins"))
	if err != nil || len(copies) != 1 || !strings.HasSuffix(copies[0].Name(), "Z.json") {
		t.Errorf("%s: plugin timeline = %v, %v; want one dated copy", m.name, copies, err)
	}
	_ = filepath.WalkDir(mirror, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("%s: %s is a symlink; the mirror holds ordinary files only", m.name, path)
		}
		if strings.Contains(path, "credentials") {
			t.Errorf("%s: %s was archived; only the listed paths may be", m.name, path)
		}
		return nil
	})
}

func TestNothingIsDeletedWhenASourceFileIsRemoved(t *testing.T) {
	ctx := context.Background()
	bucket, ms := newMachines(t, []string{"workstation", "laptop"})
	ws, lt := ms["workstation"], ms["laptop"]
	pruned := filepath.Join(ws.claude, "projects", "proj-a", "pruned.jsonl")
	write(t, pruned, "the agent prunes this later\n")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	mustSucceed(t, ws, ws.cycle.Push(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))
	bucketFilesBefore := countFiles(t, bucket)

	if err := os.Remove(pruned); err != nil { // the test plays the agent; the tool itself never removes anything
		t.Fatal(err)
	}
	mustSucceed(t, ws, ws.cycle.Run(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))

	if got := countFiles(t, bucket); got != bucketFilesBefore {
		t.Errorf("bucket holds %d files, had %d; nothing may be deleted", got, bucketFilesBefore)
	}
	for _, m := range []*machine{ws, lt} {
		kept := filepath.Join(m.paths.DefaultMirror, "workstation", "claude-code", "projects", "proj-a", "pruned.jsonl")
		if got := mustRead(t, kept); got != "the agent prunes this later\n" {
			t.Errorf("%s: pruned record in the mirror = %q", m.name, got)
		}
	}
}

func TestAGrownFileIsCompleteAfterTheNextRun(t *testing.T) {
	ctx := context.Background()
	_, ms := newMachines(t, []string{"workstation", "laptop"})
	ws, lt := ms["workstation"], ms["laptop"]
	session := filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl")
	write(t, session, "line 1\n")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	mustSucceed(t, ws, ws.cycle.Push(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))

	f, err := os.OpenFile(session, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("line 2, ending in a partial li"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, ws, ws.cycle.Run(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))

	got := mustRead(t, filepath.Join(lt.paths.DefaultMirror, "workstation", "claude-code", "projects", "proj-a", "s1.jsonl"))
	if got != "line 1\nline 2, ending in a partial li" {
		t.Errorf("laptop's copy = %q", got)
	}
}

func TestAFailedPushDoesNotPreventPull(t *testing.T) {
	if os.Getuid() == 0 {
		t.Fatal("this test makes a directory read-only, which root ignores; run the tests as an ordinary user")
	}
	ctx := context.Background()
	bucket, ms := newMachines(t, []string{"workstation", "laptop"})
	ws, lt := ms["workstation"], ms["laptop"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	mustSucceed(t, ws, ws.cycle.Push(ctx))
	mustSucceed(t, lt, lt.cycle.Run(ctx))

	// Storage starts refusing workstation's writes. Every directory of the area has to be
	// closed: the encrypted folders the first push made are where the next file would go.
	area := filepath.Join(bucket, "workstation")
	chmodDirs(t, area, 0o555)
	t.Cleanup(func() { chmodDirs(t, area, 0o755) })
	write(t, filepath.Join(ws.claude, "projects", "proj-new", "s3.jsonl"), "cannot be pushed yet\n")

	if ws.cycle.Run(ctx) {
		t.Error("Run reported success although push failed")
	}
	if got := mustRead(t, filepath.Join(ws.paths.DefaultMirror, "laptop", "claude-code", "projects", "proj-b", "s2.jsonl")); got != "from laptop\n" {
		t.Errorf("pull did not happen after the failed push: %q", got)
	}
	st, _ := status.Load(ws.paths.StatusFile)
	if push := st.Steps["push"]; push.Error == "" || push.LastSuccess.IsZero() {
		t.Errorf("push status = %+v; want an error and the earlier success kept", push)
	}
	if pull := st.Steps["pull:laptop"]; pull.Error != "" {
		t.Errorf("pull:laptop status = %+v; want success", pull)
	}
	if got := ws.errors.String(); !strings.Contains(got, "push FAILED") || !strings.Contains(got, "permission denied") {
		t.Errorf("errors shown to the operator = %q", got)
	}
	if log := mustRead(t, ws.paths.LogFile); !strings.Contains(log, "  push  FAILED\n    ") {
		t.Errorf("log lacks the failed push with its detail:\n%s", log)
	}
}

func TestOneFailedPullDoesNotBlockTheOthers(t *testing.T) {
	ctx := context.Background()
	// old-desktop sorts between laptop and tablet and has never pushed, so the pull that comes
	// after it (tablet) is the one that proves a failure does not stop the loop.
	_, ms := newMachines(t, []string{"workstation", "laptop", "tablet"}, "old-desktop")
	ws, lt, tb := ms["workstation"], ms["laptop"], ms["tablet"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	write(t, filepath.Join(tb.claude, "projects", "proj-c", "s3.jsonl"), "from tablet\n")
	mustSucceed(t, lt, lt.cycle.Push(ctx))
	mustSucceed(t, tb, tb.cycle.Push(ctx))

	if ws.cycle.Run(ctx) {
		t.Error("Run reported success although one pull failed")
	}
	st, _ := status.Load(ws.paths.StatusFile)
	if s := st.Steps["pull:old-desktop"]; !strings.Contains(s.Error, "directory not found") {
		t.Errorf("pull:old-desktop = %+v", s)
	}
	if s := st.Steps["pull:laptop"]; s.Error != "" || s.LastSuccess.IsZero() {
		t.Errorf("pull:laptop = %+v; want success", s)
	}
	if s := st.Steps["pull:tablet"]; s.Error != "" || s.LastSuccess.IsZero() {
		t.Errorf("pull:tablet = %+v; want success even though it is pulled after the failure", s)
	}
	if !strings.Contains(ws.errors.String(), "pull:old-desktop FAILED") {
		t.Errorf("errors = %q", ws.errors.String())
	}
}

func TestOfflineRunFailsFastAndRecordsIt(t *testing.T) {
	ctx := context.Background()
	_, ms := newMachines(t, []string{"workstation"})
	ws := ms["workstation"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")

	// Point this machine at B2 storage whose endpoint refuses connections, as an offline
	// machine would see. No network is used: 127.0.0.1:1 is a closed local port.
	b, err := os.ReadFile(ws.paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	var password string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "password = ") {
			password = strings.TrimPrefix(line, "password = ")
		}
	}
	offline := fmt.Sprintf("[b2]\ntype = b2\naccount = 000placeholder\nkey = K000placeholder\nendpoint = http://127.0.0.1:1\n\n"+
		"[crypt-workstation]\ntype = crypt\nremote = b2:scratch-bucket/workstation\npassword = %s\n", password)
	if err := os.WriteFile(ws.paths.RcloneConf, []byte(offline), 0o600); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if ws.cycle.Run(ctx) {
		t.Error("Run reported success while offline")
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("offline run took %v; it must fail fast", took)
	}
	st, _ := status.Load(ws.paths.StatusFile)
	if s := st.Steps["push"]; !strings.Contains(s.Error, "connection refused") {
		t.Errorf("push = %+v", s)
	}
	if s := st.Steps["local-copy"]; s.Error != "" {
		t.Errorf("local-copy = %+v; the local copy needs no network", s)
	}
}

// A changing source file cannot be produced on demand, so this drives record directly with
// the outcome the rclone package reports for it (that classification is tested there, on
// recorded rclone output).
func TestAWarningCountsAsSuccessAndIsLoggedWithItsDetail(t *testing.T) {
	_, ms := newMachines(t, []string{"workstation"})
	ws := ms["workstation"]
	detail := "ERROR: s1.jsonl: Failed to copy: can't copy - source file is being updated"

	if !ws.cycle.record("local-copy", rclone.Warning, detail) {
		t.Error("record reported a warning as a failed step")
	}
	st, _ := status.Load(ws.paths.StatusFile)
	if s := st.Steps["local-copy"]; s.Error != "" || s.LastSuccess.IsZero() {
		t.Errorf("status = %+v; a warning is a success, the next run copies the file", s)
	}
	if log := mustRead(t, ws.paths.LogFile); !strings.Contains(log, "  local-copy  warning\n    "+detail) {
		t.Errorf("log = %q", log)
	}
	if ws.errors.Len() != 0 {
		t.Errorf("a warning was shown as an error: %q", ws.errors.String())
	}
}

func TestPullTightensAnExistingLooseMirror(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "mirror")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatal(err)
	}
	runner, err := rclone.New("", filepath.Join(dir, "rclone.conf"), rclone.Concurrency{})
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     mirror,
		Readable:   []string{"laptop"},
		Runner:     runner,
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	c.Pull(context.Background()) // laptop has never pushed, so the pull itself fails; the mirror is still tightened first
	info, err := os.Stat(mirror)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("mirror mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestLocalCopyTightensAnExistingLooseMirror(t *testing.T) {
	dir := t.TempDir()
	mirror := filepath.Join(dir, "mirror")
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     mirror,
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if !c.localCopy(context.Background()) {
		t.Fatal("localCopy failed")
	}
	info, err := os.Stat(mirror)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("mirror mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestPullReportsMirrorCreationError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     filepath.Join(blocker, "mirror"), // blocker is a file, so MkdirAll fails
		Readable:   []string{"laptop"},
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.Pull(context.Background()) {
		t.Error("Pull reported success although the mirror could not be created")
	}
}

func TestStorePluginsSkipsSourcesWithNoManifest(t *testing.T) {
	dir := t.TempDir()
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     filepath.Join(dir, "mirror"),
		Sources:    []config.Source{{Tool: "claude-code", Root: dir, Paths: []string{"projects"}}},
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if !c.storePlugins() {
		t.Error("storePlugins failed although no source has a plugin manifest")
	}
}

func TestStorePluginsRecordsFailureWhenStoreErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "plugins", "installed_plugins.json"), `{"a":"1.0.0"}`)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     blocker, // machineDir() sits under a file, so Store's MkdirAll fails
		Sources:    []config.Source{{Tool: "claude-code", Root: dir, PluginManifest: "plugins/installed_plugins.json"}},
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.storePlugins() {
		t.Error("storePlugins reported success although the manifest could not be stored")
	}
}

func TestLocalCopyReportsDirectoryCreationError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     blocker,
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.localCopy(context.Background()) {
		t.Error("localCopy reported success although the directory could not be created")
	}
}

func TestPushReportsDirectoryCreationError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     blocker,
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.push(context.Background()) {
		t.Error("push reported success although the directory could not be created")
	}
}

func TestLocalCopyRecordsWorstOutcomeAndDetailsAcrossPaths(t *testing.T) {
	ctx := context.Background()
	_, ms := newMachines(t, []string{"workstation"})
	ws := ms["workstation"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	ws.cycle.Sources[0].Paths = append(ws.cycle.Sources[0].Paths, "does-not-exist")

	if ws.cycle.localCopy(ctx) {
		t.Error("localCopy reported success although one path does not exist")
	}
	if !strings.Contains(ws.errors.String(), "local-copy FAILED") {
		t.Errorf("errors = %q", ws.errors.String())
	}
}

// TestLocalCopyPutsAFileSourceAtItsMirrorPathNotADirectory guards the history.jsonl bug: rclone
// treats a file destination path as a directory to copy into, so a naive "same relative path"
// copy of a file source produces <dest>/<basename> instead of <dest>.
func TestLocalCopyPutsAFileSourceAtItsMirrorPathNotADirectory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	root := filepath.Join(dir, "claude")
	write(t, filepath.Join(root, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	write(t, filepath.Join(root, "history.jsonl"), "typed prompt history\n")

	runner, err := rclone.New("", filepath.Join(dir, "rclone.conf"), rclone.Concurrency{})
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	c := &Cycle{
		Machine:    "workstation",
		Mirror:     filepath.Join(dir, "mirror"),
		Sources:    []config.Source{{Tool: "claude-code", Root: root, Paths: []string{"projects", "history.jsonl"}}},
		Runner:     runner,
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if !c.localCopy(ctx) {
		t.Fatal("localCopy failed")
	}

	historyPath := filepath.Join(dir, "mirror", "workstation", "claude-code", "history.jsonl")
	info, err := os.Stat(historyPath)
	if err != nil {
		t.Fatalf("history.jsonl not copied: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory, want a file", historyPath)
	}
	if got := mustRead(t, historyPath); got != "typed prompt history\n" {
		t.Errorf("history.jsonl content = %q", got)
	}
	if got := mustRead(t, filepath.Join(dir, "mirror", "workstation", "claude-code", "projects", "proj-a", "s1.jsonl")); got != "from workstation\n" {
		t.Errorf("directory source still copies as before: content = %q", got)
	}
}

func TestRecordReportsLogWriteError(t *testing.T) {
	dir := t.TempDir()
	logIsADir := filepath.Join(dir, "run.log")
	if err := os.Mkdir(logIsADir, 0o700); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		StatusPath: filepath.Join(dir, "status.json"),
		Log:        runlog.New(logIsADir),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.record("local-copy", rclone.Success, "") {
		t.Error("record reported success although the log write failed")
	}
}

func TestRecordReportsStatusLoadError(t *testing.T) {
	dir := t.TempDir()
	statusIsADir := filepath.Join(dir, "status.json")
	if err := os.Mkdir(statusIsADir, 0o700); err != nil {
		t.Fatal(err)
	}
	c := &Cycle{
		StatusPath: statusIsADir,
		Log:        runlog.New(filepath.Join(dir, "run.log")),
		Now:        time.Now,
		Errors:     io.Discard,
	}
	if c.record("local-copy", rclone.Success, "") {
		t.Error("record reported success although the status file could not be read")
	}
}

func TestPushDoesNotPullAndPullDoesNotPush(t *testing.T) {
	ctx := context.Background()
	bucket, ms := newMachines(t, []string{"workstation", "laptop"})
	ws, lt := ms["workstation"], ms["laptop"]
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s1.jsonl"), "from workstation\n")
	write(t, filepath.Join(lt.claude, "projects", "proj-b", "s2.jsonl"), "from laptop\n")
	mustSucceed(t, lt, lt.cycle.Push(ctx))

	mustSucceed(t, ws, ws.cycle.Push(ctx))
	if _, err := os.Stat(filepath.Join(ws.paths.DefaultMirror, "laptop")); !os.IsNotExist(err) {
		t.Errorf("Push pulled laptop (err = %v)", err)
	}

	before := countFiles(t, bucket)
	write(t, filepath.Join(ws.claude, "projects", "proj-a", "s9.jsonl"), "not pushed by pull\n")
	mustSucceed(t, ws, ws.cycle.Pull(ctx))
	if got := countFiles(t, bucket); got != before {
		t.Errorf("Pull changed the bucket: %d files, had %d", got, before)
	}
	if got := mustRead(t, filepath.Join(ws.paths.DefaultMirror, "laptop", "claude-code", "projects", "proj-b", "s2.jsonl")); got != "from laptop\n" {
		t.Errorf("pulled record = %q", got)
	}
}
