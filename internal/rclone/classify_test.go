package rclone

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCopyArgs(t *testing.T) {
	cases := map[string]struct {
		concurrency         Concurrency
		transfers, checkers string
	}{
		"unset falls back to rclone's own defaults": {Concurrency{}, "4", "8"},
		"both configured":           {Concurrency{Transfers: 16, Checkers: 32}, "16", "32"},
		"only transfers configured": {Concurrency{Transfers: 16}, "16", "8"},
		"only checkers configured":  {Concurrency{Checkers: 32}, "4", "32"},
	}
	for name, tc := range cases {
		got := copyArgs("/cfg/rclone.conf", tc.concurrency, "/src", "crypt-workstation:")
		want := []string{
			"--config", "/cfg/rclone.conf", "copy",
			"--use-json-log", "--skip-links",
			"--contimeout", "15s", "--retries", "1", "--low-level-retries", "3",
			"--transfers", tc.transfers, "--checkers", tc.checkers,
			"/src", "crypt-workstation:",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: copyArgs = %q\nwant %q", name, got, want)
		}
	}
}

func TestObscureArgs(t *testing.T) {
	got := obscureArgs("/cfg/rclone.conf")
	want := []string{"--config", "/cfg/rclone.conf", "obscure", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("obscureArgs = %q\nwant %q", got, want)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		exitCode   int
		stderr     []byte
		want       Outcome
		wantOutput []string // substrings
	}{
		{"quiet success", 0, nil, Success, nil},
		{"success with a notice", 0, []byte(`{"level":"notice","msg":"Skipping undecryptable file name: not a multiple of blocksize","object":"stray"}` + "\n"), Success, []string{"NOTICE: stray: Skipping undecryptable file name"}},
		{"only files being written were left behind", 6, fixture(t, "source-updated.jsonl"), Warning, []string{"ERROR: big.jsonl: Failed to copy: can't copy - source file is being updated"}},
		{"missing source directory", 3, fixture(t, "dir-not-found.jsonl"), Failure, []string{"directory not found"}},
		{"unreachable storage", 1, fixture(t, "unreachable.jsonl"), Failure, []string{"CRITICAL: Failed to create file system", "connection refused"}},
		{"a changing file alongside a real error is a failure", 6, append(fixture(t, "source-updated.jsonl"), []byte(`{"level":"error","msg":"Failed to copy: permission denied","object":"other.jsonl"}`+"\n")...), Failure, []string{"permission denied"}},
		{"a notice line never counts as an error", 6, []byte(
			`{"level":"notice","msg":"Removing partially written file on error","object":"big.jsonl.partial"}` + "\n" +
				`{"level":"error","msg":"Failed to copy: can't copy - source file is being updated","object":"big.jsonl"}` + "\n"),
			Warning, []string{"NOTICE: big.jsonl.partial: Removing partially written file on error"}},
		{"a log line longer than 64 KiB is still read", 6, []byte(
			`{"level":"error","msg":"Failed to copy: can't copy - source file is being updated ` + strings.Repeat("x", 100*1024) + `","object":"big.jsonl"}` + "\n"),
			Warning, []string{"ERROR: big.jsonl: Failed to copy"}},
		{"non-zero exit with no output", 2, nil, Failure, []string{"rclone exited with status 2"}},
		{"output that is not JSON is kept as it is", 2, []byte("panic: something broke\n"), Failure, []string{"panic: something broke"}},
	}
	for _, tc := range cases {
		got := classify(tc.exitCode, tc.stderr)
		if got.Outcome != tc.want || got.ExitCode != tc.exitCode {
			t.Errorf("%s: classify = outcome %v exit %d, want outcome %v exit %d", tc.name, got.Outcome, got.ExitCode, tc.want, tc.exitCode)
		}
		for _, s := range tc.wantOutput {
			if !strings.Contains(got.Output, s) {
				t.Errorf("%s: Output = %q, want it to contain %q", tc.name, got.Output, s)
			}
		}
		if tc.wantOutput == nil && got.Output != "" {
			t.Errorf("%s: Output = %q, want empty", tc.name, got.Output)
		}
	}
}

func TestOutcomeString(t *testing.T) {
	for o, want := range map[Outcome]string{Success: "ok", Warning: "warning", Failure: "FAILED"} {
		if o.String() != want {
			t.Errorf("Outcome(%d).String() = %q, want %q", o, o.String(), want)
		}
	}
}
