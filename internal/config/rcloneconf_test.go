package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRcloneConfRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "rclone.conf")
	want := RcloneConf{
		B2:        &B2{Account: "000placeholderkeyid", Key: "K000placeholderkey"},
		Passwords: map[string]string{"workstation": "obscuredA", "my-laptop-2": "obscuredB"},
	}
	if err := want.Save(path, "b2:scratch-bucket"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRcloneConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadRcloneConf = %+v\nwant %+v", got, want)
	}
}

func TestRcloneConfFileFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rclone.conf")
	c := RcloneConf{
		B2:        &B2{Account: "000placeholderkeyid", Key: "K000placeholderkey"},
		Passwords: map[string]string{"workstation": "obscuredA", "laptop": "obscuredB"},
	}
	if err := c.Save(path, "b2:scratch-bucket"); err != nil {
		t.Fatal(err)
	}
	want := `[b2]
type = b2
account = 000placeholderkeyid
key = K000placeholderkey

[crypt-laptop]
type = crypt
remote = b2:scratch-bucket/laptop
password = obscuredB

[crypt-workstation]
type = crypt
remote = b2:scratch-bucket/workstation
password = obscuredA
`
	got, _ := os.ReadFile(path)
	if string(got) != want {
		t.Errorf("rclone.conf =\n%s\nwant\n%s", got, want)
	}
}

func TestRcloneConfWithoutB2OmitsTheSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rclone.conf")
	c := RcloneConf{Passwords: map[string]string{"workstation": "obscuredA"}}
	if err := c.Save(path, "/srv/bucket"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "[crypt-workstation]\ntype = crypt\nremote = /srv/bucket/workstation\npassword = obscuredA\n"
	if string(got) != want {
		t.Errorf("rclone.conf =\n%q\nwant\n%q", got, want)
	}
	loaded, err := LoadRcloneConf(path)
	if err != nil || loaded.B2 != nil {
		t.Errorf("LoadRcloneConf = %+v, %v; want nil B2", loaded, err)
	}
}

func TestSaveWritesPrivateFileInPrivateDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "rclone.conf")
	c := RcloneConf{Passwords: map[string]string{"workstation": "obscuredA"}}
	if err := c.Save(path, "/srv/bucket"); err != nil {
		t.Fatal(err)
	}
	// Saving over an existing file must keep it private too.
	if err := c.Save(path, "/srv/bucket"); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %04o, want %04o", p, got, want)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("config dir holds %d entries, want only rclone.conf (no temp file left behind)", len(entries))
	}
}

func TestSaveRejectsInvalidMachineName(t *testing.T) {
	c := RcloneConf{Passwords: map[string]string{"Bad Name": "x"}}
	if err := c.Save(filepath.Join(t.TempDir(), "rclone.conf"), "/srv/bucket"); err == nil {
		t.Error("Save accepted an invalid machine name")
	}
}

func TestMachinesIsSorted(t *testing.T) {
	c := RcloneConf{Passwords: map[string]string{"workstation": "a", "laptop": "b"}}
	if got := c.Machines(); !reflect.DeepEqual(got, []string{"laptop", "workstation"}) {
		t.Errorf("Machines = %v", got)
	}
}

func TestCryptRemote(t *testing.T) {
	if got := CryptRemote("my-laptop-2"); got != "crypt-my-laptop-2:" {
		t.Errorf("CryptRemote = %q", got)
	}
}

func TestLoadErrorsNeverQuoteFileContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rclone.conf")
	if err := os.WriteFile(path, []byte("[b2]\nkey = K000secretvalue\nthis line is malformed K000secretvalue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRcloneConf(path)
	if err == nil {
		t.Fatal("LoadRcloneConf accepted a malformed line")
	}
	if strings.Contains(err.Error(), "K000secretvalue") {
		t.Errorf("error quotes file contents: %v", err)
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %v, want it to name line 3", err)
	}
}

func TestCheckPermissions(t *testing.T) {
	newPaths := func(t *testing.T) Paths {
		p := PathsFor(t.TempDir())
		c := RcloneConf{Passwords: map[string]string{"workstation": "obscuredA"}}
		if err := c.Save(p.RcloneConf, "/srv/bucket"); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if err := CheckPermissions(newPaths(t)); err != nil {
		t.Errorf("private files: CheckPermissions = %v, want nil", err)
	}

	p := newPaths(t)
	if err := os.Chmod(p.RcloneConf, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := CheckPermissions(p); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Errorf("group-readable rclone.conf: CheckPermissions = %v, want an error giving the chmod fix", err)
	}

	p = newPaths(t)
	if err := os.Chmod(p.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckPermissions(p); err == nil || !strings.Contains(err.Error(), "chmod 700") {
		t.Errorf("world-readable config dir: CheckPermissions = %v, want an error giving the chmod fix", err)
	}

	missing := PathsFor(t.TempDir())
	if err := CheckPermissions(missing); err == nil || !strings.Contains(err.Error(), "agent-downlink setup") {
		t.Errorf("missing files: CheckPermissions = %v, want an error pointing at setup", err)
	}
}
