package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func validFile() File {
	return File{
		Machine: "workstation",
		Storage: "b2:scratch-bucket",
		Mirror:  "/home/u/.local/share/agent-downlink/mirror",
		Rclone:  "/usr/bin/rclone",
		Tools:   []string{"claude-code"},
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "config.toml")
	want := validFile()
	want.CustomTools = []CustomTool{{Name: "other-agent", Root: "/home/u/.other", Paths: []string{"sessions"}, PluginManifest: "plugins.json"}}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v\nwant %+v", got, want)
	}
}

func TestSaveCreatesPrivateDirectoryAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "config.toml")
	if err := validFile().Save(path); err != nil {
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
}

func TestSaveLeavesTheExistingFileUntouchedWhenItCannotBeReplaced(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("this test makes a directory read-only, which root ignores")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := validFile().Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	other := validFile()
	other.Machine = "laptop"
	if err := other.Save(path); err == nil {
		t.Fatal("Save = nil error, want it to fail because the directory cannot be written to")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("existing config.toml changed after a failed save: %q, now %q, %v", before, after, err)
	}
}

// TestCopyParallelismIsOptional covers both halves of an optional setting: a file that leaves
// it out stays free of it and loads, and a file that sets it round-trips.
func TestCopyParallelismIsOptional(t *testing.T) {
	dir := t.TempDir()
	unset := filepath.Join(dir, "unset.toml")
	if err := validFile().Save(unset); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(unset)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"transfers", "checkers"} {
		if strings.Contains(string(b), field+" =") {
			t.Errorf("unset %q was written to config.toml:\n%s", field, b)
		}
	}
	got, err := Load(unset)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transfers != 0 || got.Checkers != 0 {
		t.Errorf("Load = transfers %d, checkers %d; want both unset", got.Transfers, got.Checkers)
	}

	set := filepath.Join(dir, "set.toml")
	f := validFile()
	f.Transfers, f.Checkers = 16, 32
	if err := f.Save(set); err != nil {
		t.Fatal(err)
	}
	got, err = Load(set)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transfers != 16 || got.Checkers != 32 {
		t.Errorf("Load = transfers %d, checkers %d; want 16 and 32", got.Transfers, got.Checkers)
	}
}

func TestSavedFileExplainsEveryField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	f := validFile()
	f.Transfers, f.Checkers = 16, 32 // optional fields, absent from a file that leaves them out
	if err := f.Save(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	for _, field := range []string{"machine", "storage", "mirror", "rclone", "tools", "transfers", "checkers"} {
		found := false
		for i, line := range lines {
			if strings.HasPrefix(line, field+" =") {
				found = true
				if i == 0 || !strings.HasPrefix(lines[i-1], "#") {
					t.Errorf("field %q has no comment on the line above it:\n%s", field, b)
				}
			}
		}
		if !found {
			t.Errorf("field %q not written:\n%s", field, b)
		}
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := validFile().Save(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append([]byte("mirorr = '/typo'\n"), b...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load accepted a misspelled field, want an error")
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		mutate func(*File)
		want   string // substring of the error
	}{
		"bad machine":            {func(f *File) { f.Machine = "Work Station" }, "lowercase letters, digits, and hyphens"},
		"empty storage":          {func(f *File) { f.Storage = "" }, "storage"},
		"relative storage":       {func(f *File) { f.Storage = "scratch-bucket" }, "storage"},
		"storage with no bucket": {func(f *File) { f.Storage = "b2:" }, "bucket name"},
		"relative mirror":        {func(f *File) { f.Mirror = "mirror" }, "mirror must be an absolute path"},
		"relative rclone":        {func(f *File) { f.Rclone = "rclone" }, "rclone must be an absolute path"},
		"negative transfers":     {func(f *File) { f.Transfers = -1 }, "transfers must be 1 or more"},
		"negative checkers":      {func(f *File) { f.Checkers = -1 }, "checkers must be 1 or more"},
		"unknown tool":           {func(f *File) { f.Tools = []string{"nonesuch"} }, `unknown tool "nonesuch"`},
		"custom shadows builtin": {func(f *File) { f.CustomTools = []CustomTool{{Name: "claude-code", Root: "/x", Paths: []string{"a"}}} }, "built in"},
		"duplicate custom tool name": {func(f *File) {
			f.CustomTools = []CustomTool{
				{Name: "other", Root: "/x", Paths: []string{"a"}},
				{Name: "other", Root: "/y", Paths: []string{"b"}},
			}
		}, "duplicate"},
		"custom bad name":      {func(f *File) { f.CustomTools = []CustomTool{{Name: "Bad Name", Root: "/x", Paths: []string{"a"}}} }, "lowercase letters, digits, and hyphens"},
		"custom relative root": {func(f *File) { f.CustomTools = []CustomTool{{Name: "other", Root: "x", Paths: []string{"a"}}} }, "root must be an absolute path"},
		"custom no paths":      {func(f *File) { f.CustomTools = []CustomTool{{Name: "other", Root: "/x"}} }, "at least one path"},
		"custom escaping path": {func(f *File) {
			f.CustomTools = []CustomTool{{Name: "other", Root: "/x", Paths: []string{"../secrets"}}}
		}, "must stay inside root"},
		"custom absolute path": {func(f *File) { f.CustomTools = []CustomTool{{Name: "other", Root: "/x", Paths: []string{"/etc"}}} }, "must stay inside root"},
		"custom escaping manifest": {func(f *File) {
			f.CustomTools = []CustomTool{{Name: "other", Root: "/x", Paths: []string{"a"}, PluginManifest: "../m.json"}}
		}, "must stay inside root"},
	}
	for name, tc := range cases {
		f := validFile()
		tc.mutate(&f)
		err := f.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate() = %v, want an error containing %q", name, err, tc.want)
		}
	}
	if err := validFile().Validate(); err != nil {
		t.Errorf("valid file: Validate() = %v", err)
	}
	empty := validFile()
	empty.Rclone = "" // allowed: means "find rclone on PATH"
	if err := empty.Validate(); err != nil {
		t.Errorf("empty rclone: Validate() = %v", err)
	}
	localBucket := validFile()
	localBucket.Storage = "/tmp/scratch-bucket" // an absolute local path stands in for B2 in integration tests
	if err := localBucket.Validate(); err != nil {
		t.Errorf("absolute local storage: Validate() = %v", err)
	}
}

// TestSaveReportsARenameFailure covers the rename step on its own: CreateTemp and the
// writes succeed, then the rename onto a path that is a directory fails and Save must say
// so rather than fall through to the directory sync and report success.
func TestSaveReportsARenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validFile().Save(path); err == nil {
		t.Fatal("Save = nil error, want the rename onto a directory to fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary file left behind after a failed rename: %v", entries)
	}
}

func TestSyncDirReportsAMissingDirectory(t *testing.T) {
	err := syncDir(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("syncDir = %v, want fs.ErrNotExist", err)
	}
}
