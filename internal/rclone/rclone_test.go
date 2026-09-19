package rclone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// realRunner returns a Runner on the installed rclone. The integration tests fail, rather
// than skip, when rclone is absent: a green run must mean the encryption path was exercised.
func realRunner(t *testing.T, configPath string) *Runner {
	t.Helper()
	r, err := New("", configPath)
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	return r
}

// writeCryptConf writes an rclone.conf with one crypt remote over a local directory.
func writeCryptConf(t *testing.T, r *Runner, dir, machine, password string) (confPath, bucket string) {
	t.Helper()
	obscured, err := r.Obscure(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	bucket = filepath.Join(dir, "bucket")
	confPath = filepath.Join(dir, "rclone.conf")
	conf := fmt.Sprintf("[crypt-%s]\ntype = crypt\nremote = %s/%s\npassword = %s\n", machine, bucket, machine, obscured)
	if err := os.WriteFile(confPath, []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	return confPath, bucket
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewFailsClearlyWhenRcloneIsAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := New("", "/unused")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("New = %v, want ErrNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "https://rclone.org/install/") {
		t.Errorf("error = %q, want it to say where to get rclone", err)
	}
	if _, err := New(filepath.Join(t.TempDir(), "no-such-rclone"), "/unused"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("New with a missing explicit binary = %v, want ErrNotInstalled", err)
	}
}

func TestEncryptThenDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	confPath, bucket := writeCryptConf(t, realRunner(t, "/unused"), dir, "my-laptop-2", "placeholder-password")
	r := realRunner(t, confPath)
	ctx := context.Background()

	const record = `{"type":"user","message":"a recognisable plaintext marker"}` + "\n"
	writeFile(t, filepath.Join(dir, "src", "claude-code", "projects", "proj-a", "session-1.jsonl"), record)

	if res := r.Copy(ctx, filepath.Join(dir, "src"), "crypt-my-laptop-2:"); res.Outcome != Success || res.Output != "" {
		t.Fatalf("push: %+v", res)
	}

	// The bucket holds only ciphertext: no plaintext names, no plaintext content.
	files := 0
	walkErr := filepath.WalkDir(bucket, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(filepath.Join(bucket, "my-laptop-2"), path)
		for _, plain := range []string{"claude-code", "projects", "proj-a", "session-1"} {
			if strings.Contains(rel, plain) {
				t.Errorf("bucket path %q contains the plaintext name %q", rel, plain)
			}
		}
		if !d.IsDir() {
			files++
			b, _ := os.ReadFile(path)
			if strings.Contains(string(b), "recognisable plaintext marker") {
				t.Errorf("bucket file %q holds plaintext", rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if files != 1 {
		t.Errorf("bucket holds %d files, want 1", files)
	}

	if res := r.Copy(ctx, "crypt-my-laptop-2:", filepath.Join(dir, "pulled")); res.Outcome != Success {
		t.Fatalf("pull: %+v", res)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pulled", "claude-code", "projects", "proj-a", "session-1.jsonl"))
	if err != nil || string(got) != record {
		t.Errorf("pulled record = %q, %v; want the original", got, err)
	}
}

func TestCopyReportsAMissingSourceAsFailure(t *testing.T) {
	dir := t.TempDir()
	confPath, _ := writeCryptConf(t, realRunner(t, "/unused"), dir, "workstation", "placeholder-password")
	res := realRunner(t, confPath).Copy(context.Background(), filepath.Join(dir, "no-such-dir"), filepath.Join(dir, "dst"))
	if res.Outcome != Failure || res.ExitCode != 3 || !strings.Contains(res.Output, "directory not found") {
		t.Errorf("Copy = %+v, want Failure, exit 3, 'directory not found'", res)
	}
}

func TestCopySkipsSymlinksQuietly(t *testing.T) {
	dir := t.TempDir()
	confPath, _ := writeCryptConf(t, realRunner(t, "/unused"), dir, "workstation", "placeholder-password")
	writeFile(t, filepath.Join(dir, "src", "real.jsonl"), "x\n")
	if err := os.Symlink(filepath.Join(dir, "src", "real.jsonl"), filepath.Join(dir, "src", "link.jsonl")); err != nil {
		t.Fatal(err)
	}
	res := realRunner(t, confPath).Copy(context.Background(), filepath.Join(dir, "src"), filepath.Join(dir, "dst"))
	if res.Outcome != Success || res.Output != "" {
		t.Errorf("Copy = %+v, want a quiet success", res)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dst", "link.jsonl")); !os.IsNotExist(err) {
		t.Errorf("the symlink was copied (err = %v); the mirror must hold ordinary files only", err)
	}
}

// TestSecretsNeverReachAProcessArgument runs real rclone through a wrapper that records every
// argument it is started with. Other users on a machine can read process arguments.
func TestSecretsNeverReachAProcessArgument(t *testing.T) {
	real, err := exec.LookPath("rclone")
	if err != nil {
		t.Fatalf("integration tests need rclone: %v", err)
	}
	dir := t.TempDir()
	argLog := filepath.Join(dir, "args.log")
	wrapper := filepath.Join(dir, "rclone-recording-args")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" >> %q\nexec %q \"$@\"\n", argLog, real)
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	const password = "placeholder-password-that-must-stay-off-argv"
	recording, err := New(wrapper, "/unused")
	if err != nil {
		t.Fatal(err)
	}
	confPath, _ := writeCryptConf(t, recording, dir, "workstation", password)
	obscured, _ := recording.Obscure(context.Background(), password)

	r, err := New(wrapper, confPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "src", "a.jsonl"), "x\n")
	if res := r.Copy(context.Background(), filepath.Join(dir, "src"), "crypt-workstation:"); res.Outcome != Success {
		t.Fatalf("Copy: %+v", res)
	}

	logged, err := os.ReadFile(argLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "obscure") || !strings.Contains(string(logged), "copy") {
		t.Fatalf("the wrapper did not record the rclone invocations: %q", logged)
	}
	for _, secret := range []string{password, obscured} {
		if strings.Contains(string(logged), secret) {
			t.Error("a secret appeared in rclone's arguments")
		}
	}
}

func TestObscureErrorDoesNotRevealTheSecret(t *testing.T) {
	dir := t.TempDir()
	failing := filepath.Join(dir, "rclone-that-fails")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := New(failing, "/unused")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Obscure(context.Background(), "placeholder-secret")
	if err == nil || strings.Contains(err.Error(), "placeholder-secret") {
		t.Errorf("Obscure error = %v, want an error that does not contain the secret", err)
	}
}
