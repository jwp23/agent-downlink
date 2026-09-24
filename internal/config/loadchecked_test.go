package config

import (
	"os"
	"strings"
	"testing"
)

// configuredHome writes a valid config.toml and rclone.conf under a fresh home and returns it.
func configuredHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	paths := PathsFor(home)
	if err := validFile().Save(paths.ConfigFile); err != nil {
		t.Fatal(err)
	}
	secrets := RcloneConf{Passwords: map[string]string{"workstation": "obscuredA"}}
	if err := secrets.Save(paths.RcloneConf, "b2:scratch-bucket"); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestLoadCheckedReturnsBothFilesWhenPrivateAndValid(t *testing.T) {
	home := configuredHome(t)
	paths, cfg, secrets, err := LoadChecked(home)
	if err != nil {
		t.Fatalf("LoadChecked = %v", err)
	}
	if paths != PathsFor(home) {
		t.Errorf("paths = %+v, want PathsFor(home)", paths)
	}
	if cfg.Machine != "workstation" || secrets.Passwords["workstation"] != "obscuredA" {
		t.Errorf("cfg = %+v, secrets = %+v", cfg, secrets)
	}
}

func TestLoadCheckedRefusals(t *testing.T) {
	cases := map[string]struct {
		breakIt func(t *testing.T, p Paths)
		wantErr string
	}{
		"rclone.conf readable by the group": {
			func(t *testing.T, p Paths) {
				if err := os.Chmod(p.RcloneConf, 0o640); err != nil {
					t.Fatal(err)
				}
			}, "chmod 600"},
		"config.toml missing": {
			func(t *testing.T, p Paths) {
				if err := os.Remove(p.ConfigFile); err != nil {
					t.Fatal(err)
				}
			}, "config.toml"},
		"rclone.conf malformed": {
			func(t *testing.T, p Paths) {
				if err := os.WriteFile(p.RcloneConf, []byte("not a section\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}, "line 1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home := configuredHome(t)
			tc.breakIt(t, PathsFor(home))
			_, _, _, err := LoadChecked(home)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("LoadChecked = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}
