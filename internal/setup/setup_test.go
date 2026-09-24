package setup

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jwp23/agent-downlink/internal/config"
)

type timer struct{ installs int }

func (tm *timer) install() error { tm.installs++; return nil }

// runSetup answers, in order: machine name, bucket, key ID, application key, encryption password.
func runSetup(t *testing.T, home string, o Options, answers ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	o.Home = home
	err := Run(context.Background(), NewPrompter(strings.NewReader(strings.Join(answers, "\n")+"\n"), &out), o)
	return out.String(), err
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// completedSetup runs one setup that takes the default machine name and generates its own
// encryption password, and returns what it printed and where it wrote.
func completedSetup(t *testing.T, o Options) (out string, paths config.Paths) {
	t.Helper()
	home := t.TempDir()
	out, err := runSetup(t, home, o, "", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", "")
	if err != nil {
		t.Fatalf("Run = %v\noutput:\n%s", err, out)
	}
	return out, config.PathsFor(home)
}

// shownPassword is the generated password setup told the operator to save.
func shownPassword(t *testing.T, out string) string {
	t.Helper()
	m := regexp.MustCompile(`Encryption password:\s+(\S+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("output does not show a generated password:\n%s", out)
	}
	return m[1]
}

func TestSetupWritesConfigToml(t *testing.T) {
	_, paths := completedSetup(t, Options{Hostname: "Workstation.local", NoTimer: true})
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Machine != "workstation" || cfg.Storage != "b2:scratch-bucket" || cfg.Mirror != paths.DefaultMirror ||
		len(cfg.Tools) != 1 || cfg.Tools[0] != "claude-code" || !strings.HasSuffix(cfg.Rclone, "rclone") {
		t.Errorf("config.toml = %+v", cfg)
	}
}

func TestSetupWritesTheStorageKeyAndThisMachinesCryptRemote(t *testing.T) {
	_, paths := completedSetup(t, Options{Hostname: "Workstation.local", NoTimer: true})
	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		t.Fatal(err)
	}
	if secrets.B2 == nil || secrets.B2.Account != "000placeholderkeyid" || secrets.B2.Key != "K000placeholderkey" {
		t.Errorf("b2 section = %+v", secrets.B2)
	}
	if len(secrets.Passwords) != 1 || secrets.Passwords["workstation"] == "" {
		t.Errorf("crypt passwords = %v", secrets.Machines())
	}
	raw, _ := os.ReadFile(paths.RcloneConf)
	if !strings.Contains(string(raw), "remote = b2:scratch-bucket/workstation\n") {
		t.Errorf("rclone.conf lacks the crypt remote line")
	}
}

func TestSetupLeavesTheSecretsReadableOnlyByTheirOwner(t *testing.T) {
	out, paths := completedSetup(t, Options{Hostname: "Workstation.local", NoTimer: true})
	if mode(t, paths.ConfigDir) != 0o700 || mode(t, paths.RcloneConf) != 0o600 {
		t.Errorf("modes: dir %04o, rclone.conf %04o", mode(t, paths.ConfigDir), mode(t, paths.RcloneConf))
	}
	if err := config.CheckPermissions(paths); err != nil {
		t.Errorf("CheckPermissions after setup = %v", err)
	}
	raw, _ := os.ReadFile(paths.RcloneConf)
	if strings.Contains(string(raw), shownPassword(t, out)) {
		t.Error("rclone.conf holds the password in plaintext")
	}
	if strings.Contains(out, "K000placeholderkey") {
		t.Error("setup echoed the storage key")
	}
}

func TestSetupShowsTheGeneratedPasswordAndHowToAddAReader(t *testing.T) {
	out, _ := completedSetup(t, Options{Hostname: "Workstation.local", NoTimer: true})
	if password := shownPassword(t, out); len(password) < 40 {
		t.Errorf("generated password is %d characters long", len(password))
	}
	if !strings.Contains(out, "password manager") || !strings.Contains(out, "agent-downlink machine add workstation") {
		t.Errorf("output does not say what to save or how to add a reader:\n%s", out)
	}
}

func TestSetupInstallsTheTimerOnce(t *testing.T) {
	tm := &timer{}
	if _, _ = completedSetup(t, Options{Hostname: "Workstation.local", InstallTimer: tm.install}); tm.installs != 1 {
		t.Errorf("timer installed %d times, want 1", tm.installs)
	}
}

func TestGeneratedPasswordsDiffer(t *testing.T) {
	a, err := generatePassword()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := generatePassword()
	if a == b || len(a) < 40 {
		t.Errorf("passwords %d and %d chars long, equal = %v", len(a), len(b), a == b)
	}
}

func TestNoTimerLeavesNoScheduleInstalled(t *testing.T) {
	tm := &timer{}
	out, err := runSetup(t, t.TempDir(), Options{Hostname: "laptop", NoTimer: true, InstallTimer: tm.install},
		"", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", "")
	if err != nil {
		t.Fatal(err)
	}
	if tm.installs != 0 {
		t.Errorf("timer installed %d times, want 0", tm.installs)
	}
	if !strings.Contains(out, "agent-downlink timer install") {
		t.Errorf("output does not say how to install the timer later:\n%s", out)
	}
}

func TestASuppliedPasswordIsUsedAndNotPrinted(t *testing.T) {
	home := t.TempDir()
	out, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"laptop", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", "the-old-machines-password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "the-old-machines-password") || strings.Contains(out, "Encryption password:") {
		t.Errorf("setup printed a password the operator already has:\n%s", out)
	}
	raw, _ := os.ReadFile(config.PathsFor(home).RcloneConf)
	if strings.Contains(string(raw), "the-old-machines-password") {
		t.Error("rclone.conf holds the password in plaintext")
	}
}

func TestInvalidAnswersAreRejectedWithNothingWritten(t *testing.T) {
	cases := map[string][]string{
		"lowercase letters, digits, and hyphens": {"My Machine", "scratch-bucket", "id", "key", ""},
		"bucket name":                            {"laptop", "", "id", "key", ""},
		"key ID":                                 {"laptop", "scratch-bucket", "", "key", ""},
		"application key":                        {"laptop", "scratch-bucket", "id", "", ""},
	}
	for want, answers := range cases {
		home := t.TempDir()
		_, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true}, answers...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("answers %q: Run = %v, want an error containing %q", answers, err, want)
		}
		if _, statErr := os.Stat(config.PathsFor(home).ConfigDir); !os.IsNotExist(statErr) {
			t.Errorf("answers %q: files were written despite the error", answers)
		}
	}
}

func TestRerunRefusesToOverwriteWithoutYes(t *testing.T) {
	home := t.TempDir()
	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"laptop", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", ""); err != nil {
		t.Fatal(err)
	}
	paths := config.PathsFor(home)
	before, _ := os.ReadFile(paths.RcloneConf)

	out, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true}, "no")
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("Run = %v, want a refusal", err)
	}
	if !strings.Contains(out, "already set up") {
		t.Errorf("output = %q", out)
	}
	after, _ := os.ReadFile(paths.RcloneConf)
	if !bytes.Equal(before, after) {
		t.Error("rclone.conf changed although the operator did not type yes")
	}
}

func TestRerunWithYesKeepsReadersMirrorAndCustomTools(t *testing.T) {
	home := t.TempDir()
	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"laptop", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", ""); err != nil {
		t.Fatal(err)
	}
	paths := config.PathsFor(home)
	cfg, _ := config.Load(paths.ConfigFile)
	cfg.Mirror = "/mnt/encrypted/mirror"
	cfg.CustomTools = []config.CustomTool{{Name: "other-agent", Root: "/data/other", Paths: []string{"sessions"}}}
	if err := cfg.Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	secrets, _ := config.LoadRcloneConf(paths.RcloneConf)
	secrets.Passwords["workstation"] = "obscured-workstation-password"
	if err := secrets.Save(paths.RcloneConf, cfg.Storage); err != nil {
		t.Fatal(err)
	}

	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"yes", "laptop", "new-bucket", "000newkeyid", "K000newkey", ""); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(paths.ConfigFile)
	if cfg.Storage != "b2:new-bucket" || cfg.Mirror != "/mnt/encrypted/mirror" || len(cfg.CustomTools) != 1 {
		t.Errorf("config.toml after rerun = %+v", cfg)
	}
	secrets, _ = config.LoadRcloneConf(paths.RcloneConf)
	if secrets.Passwords["workstation"] != "obscured-workstation-password" || secrets.B2.Account != "000newkeyid" {
		t.Errorf("rclone.conf after rerun: machines %v, account %q", secrets.Machines(), secrets.B2.Account)
	}
	raw, _ := os.ReadFile(paths.RcloneConf)
	if !strings.Contains(string(raw), "remote = b2:new-bucket/workstation\n") {
		t.Error("the reader's crypt remote does not point at the new storage")
	}
}

func TestRerunRejectsACorruptExistingConfig(t *testing.T) {
	home := t.TempDir()
	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"laptop", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", ""); err != nil {
		t.Fatal(err)
	}
	paths := config.PathsFor(home)
	before, _ := os.ReadFile(paths.RcloneConf)
	if err := os.WriteFile(paths.ConfigFile, []byte("not valid toml [[["), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"yes", "laptop", "new-bucket", "000newkeyid", "K000newkey", ""); err == nil {
		t.Error("Run = nil error, want the config.toml parse error surfaced rather than silently reset")
	}
	after, _ := os.ReadFile(paths.RcloneConf)
	if !bytes.Equal(before, after) {
		t.Error("rclone.conf changed although config.toml could not be read")
	}
}

func TestRerunRejectsACorruptExistingRcloneConf(t *testing.T) {
	home := t.TempDir()
	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"laptop", "scratch-bucket", "000placeholderkeyid", "K000placeholderkey", ""); err != nil {
		t.Fatal(err)
	}
	paths := config.PathsFor(home)
	if err := os.WriteFile(paths.RcloneConf, []byte("not a valid rclone.conf [[["), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := runSetup(t, home, Options{Hostname: "laptop", NoTimer: true},
		"yes", "laptop", "new-bucket", "000newkeyid", "K000newkey", ""); err == nil {
		t.Error("Run = nil error, want the rclone.conf parse error surfaced rather than silently reset")
	}
}

func TestReplaceConfigPairRestoresOldConfigWhenSecretsSaveFails(t *testing.T) {
	home := t.TempDir()
	paths := config.PathsFor(home)
	oldCfg := config.File{Machine: "workstation", Storage: "b2:old-bucket", Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	if err := oldCfg.Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	// A directory in rclone.conf's place makes RcloneConf.Save's rename fail.
	if err := os.MkdirAll(paths.RcloneConf, 0o700); err != nil {
		t.Fatal(err)
	}

	newCfg := config.File{Machine: "laptop", Storage: "b2:new-bucket", Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	err := replaceConfigPair(paths, newCfg, config.RcloneConf{Passwords: map[string]string{}})
	if err == nil {
		t.Fatal("replaceConfigPair = nil error, want the rclone.conf write failure")
	}
	if strings.Contains(err.Error(), "could not be restored") {
		t.Errorf("error = %v, but the previous config.toml was restored", err)
	}
	got, err := config.Load(paths.ConfigFile)
	if err != nil || got.Machine != "workstation" || got.Storage != "b2:old-bucket" {
		t.Errorf("config.toml after a failed replace = %+v, %v; want the previous config restored", got, err)
	}
}

func TestReplaceConfigPairRemovesNewConfigWhenNoPreviousExisted(t *testing.T) {
	home := t.TempDir()
	paths := config.PathsFor(home)
	if err := os.MkdirAll(paths.RcloneConf, 0o700); err != nil {
		t.Fatal(err)
	}

	newCfg := config.File{Machine: "laptop", Storage: "b2:new-bucket", Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	if err := replaceConfigPair(paths, newCfg, config.RcloneConf{Passwords: map[string]string{}}); err == nil {
		t.Fatal("replaceConfigPair = nil error, want the rclone.conf write failure")
	}
	if _, err := os.Stat(paths.ConfigFile); !os.IsNotExist(err) {
		t.Errorf("config.toml exists after a failed first-time setup (err = %v); want it removed", err)
	}
}

func TestInputEndingEarlyIsAnError(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	err := Run(context.Background(), NewPrompter(strings.NewReader("laptop\n"), &out), Options{Home: home, Hostname: "laptop", NoTimer: true})
	if err == nil {
		t.Error("Run succeeded although input ended after the first answer")
	}
}
