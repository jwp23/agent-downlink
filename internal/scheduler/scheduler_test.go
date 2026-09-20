package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testBinary = "/usr/local/bin/agent-downlink"

func expected(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGeneratedFilesMatchTheExpectedFiles(t *testing.T) {
	cases := map[string]string{
		"agent-downlink.service": SystemdService(testBinary),
		"agent-downlink.timer":   SystemdTimer(),
		Label + ".plist":         LaunchdPlist(testBinary),
	}
	for name, got := range cases {
		if want := expected(t, name); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
		}
	}
}

func TestPlistEscapesTheBinaryPath(t *testing.T) {
	got := LaunchdPlist("/Users/u/R&D <tools>/agent-downlink")
	if !strings.Contains(got, "<string>/Users/u/R&amp;D &lt;tools&gt;/agent-downlink</string>") {
		t.Errorf("plist does not escape XML characters:\n%s", got)
	}
}

func TestSystemdServiceQuotesTheBinaryPath(t *testing.T) {
	got := SystemdService(`/home/u/R&D "tools"/agent-downlink`)
	if !strings.Contains(got, `ExecStart="/home/u/R&D \"tools\"/agent-downlink" run`) {
		t.Errorf("service unit does not quote a path with whitespace and quotes:\n%s", got)
	}
}

type recorder struct {
	calls [][]string
	fail  map[string]error // keyed by the first argument after the program name
}

func (r *recorder) exec(name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(args) > 0 {
		return r.fail[args[0]]
	}
	return nil
}

func TestInstallAndRemoveOnLinux(t *testing.T) {
	home := t.TempDir()
	rec := &recorder{}
	s := &Scheduler{Home: home, Binary: testBinary, GOOS: "linux", Exec: rec.exec}
	unitDir := filepath.Join(home, ".config", "systemd", "user")

	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"agent-downlink.service": SystemdService(testBinary), "agent-downlink.timer": SystemdTimer()} {
		got, err := os.ReadFile(filepath.Join(unitDir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
	wantCalls := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "agent-downlink.timer"},
	}
	if !reflect.DeepEqual(rec.calls, wantCalls) {
		t.Errorf("install ran %v\nwant %v", rec.calls, wantCalls)
	}

	rec.calls = nil
	if err := s.Remove(); err != nil {
		t.Fatal(err)
	}
	wantCalls = [][]string{
		{"systemctl", "--user", "disable", "--now", "agent-downlink.timer"},
		{"systemctl", "--user", "daemon-reload"},
	}
	if !reflect.DeepEqual(rec.calls, wantCalls) {
		t.Errorf("remove ran %v\nwant %v", rec.calls, wantCalls)
	}
	if entries, _ := os.ReadDir(unitDir); len(entries) != 0 {
		t.Errorf("unit files left behind: %v", entries)
	}
}

func TestInstallAndRemoveOnMacOS(t *testing.T) {
	home := t.TempDir()
	rec := &recorder{fail: map[string]error{"bootout": errors.New("not loaded")}}
	s := &Scheduler{Home: home, Binary: testBinary, GOOS: "darwin", UID: 501, Exec: rec.exec}
	plist := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")

	// Install boots out any earlier copy first; "not loaded" from that is expected and ignored.
	if err := s.Install(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(plist); err != nil || string(got) != LaunchdPlist(testBinary) {
		t.Errorf("plist = %q, %v", got, err)
	}
	wantCalls := [][]string{
		{"launchctl", "bootout", "gui/501/" + Label},
		{"launchctl", "bootstrap", "gui/501", plist},
	}
	if !reflect.DeepEqual(rec.calls, wantCalls) {
		t.Errorf("install ran %v\nwant %v", rec.calls, wantCalls)
	}

	rec.calls, rec.fail = nil, nil
	if err := s.Remove(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rec.calls, [][]string{{"launchctl", "bootout", "gui/501/" + Label}}) {
		t.Errorf("remove ran %v", rec.calls)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Errorf("plist left behind (err = %v)", err)
	}
}

func TestInstallReportsAFailedEnable(t *testing.T) {
	rec := &recorder{fail: map[string]error{"--user": errors.New("Failed to connect to bus")}}
	s := &Scheduler{Home: t.TempDir(), Binary: testBinary, GOOS: "linux", Exec: rec.exec}
	if err := s.Install(); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Errorf("Install = %v, want the systemctl failure", err)
	}
}

func TestRemoveWhenNothingIsInstalledSucceeds(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		rec := &recorder{fail: map[string]error{"bootout": errors.New("not loaded")}}
		s := &Scheduler{Home: t.TempDir(), Binary: testBinary, GOOS: goos, UID: 501, Exec: rec.exec}
		if err := s.Remove(); err != nil {
			t.Errorf("%s: Remove with nothing installed = %v, want nil", goos, err)
		}
	}
}

func TestUnsupportedOS(t *testing.T) {
	s := &Scheduler{Home: t.TempDir(), Binary: testBinary, GOOS: "windows", Exec: (&recorder{}).exec}
	for name, err := range map[string]error{"Install": s.Install(), "Remove": s.Remove()} {
		if err == nil || !strings.Contains(err.Error(), "windows") {
			t.Errorf("%s = %v, want an unsupported-OS error", name, err)
		}
	}
}
