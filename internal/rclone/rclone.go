// Package rclone is the only code that executes rclone. It builds argument lists and
// interprets exit statuses and output. Secrets never go into an argument list: other users on
// a machine can read process arguments.
package rclone

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNotInstalled means there is no rclone to run.
var ErrNotInstalled = errors.New("rclone is not installed or could not be found; install it from https://rclone.org/install/")

// configFlag names rclone's config file flag, common to every invocation the Runner makes.
const configFlag = "--config"

// Runner runs rclone against the tool's own rclone.conf, never the user's.
type Runner struct {
	binary     string
	configPath string
}

// New finds rclone. An empty binary means search PATH.
func New(binary, configPath string) (*Runner, error) {
	if binary == "" {
		found, err := exec.LookPath("rclone")
		if err != nil {
			return nil, ErrNotInstalled
		}
		binary = found
	} else if _, err := os.Stat(binary); err != nil {
		return nil, fmt.Errorf("%w (looked for %s)", ErrNotInstalled, binary)
	}
	return &Runner{binary: binary, configPath: configPath}, nil
}

// Binary is the path of the rclone in use.
func (r *Runner) Binary() string { return r.binary }

func copyArgs(configPath, src, dst string) []string {
	return []string{
		configFlag, configPath, "copy",
		"--use-json-log", "--skip-links",
		// Fail fast when offline; the hourly timer is the retry loop.
		"--contimeout", "15s", "--retries", "1", "--low-level-retries", "3",
		src, dst,
	}
}

// Copy runs "rclone copy", which never deletes anything at the destination.
func (r *Runner) Copy(ctx context.Context, src, dst string) Result {
	cmd := exec.CommandContext(ctx, r.binary, copyArgs(r.configPath, src, dst)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return classify(0, stderr.Bytes())
	case errors.As(err, &exitErr):
		return classify(exitErr.ExitCode(), stderr.Bytes())
	default:
		return Result{Outcome: Failure, ExitCode: -1, Output: fmt.Sprintf("could not run rclone: %v", err)}
	}
}

// ListFileVersions lists every stored file at path, one per line, including every version a
// provider like B2 keeps of a superseded file. This is a diagnostics operation: the tool
// itself never calls it, only tests inspecting what a provider actually stored.
func (r *Runner) ListFileVersions(ctx context.Context, path string) ([]string, error) {
	cmd := exec.CommandContext(ctx, r.binary, configFlag, r.configPath, "lsf", "-R", "--files-only", "--b2-versions", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rclone lsf failed: %w", err)
	}
	return strings.Fields(string(out)), nil
}

func obscureArgs(configPath string) []string {
	return []string{configFlag, configPath, "obscure", "-"}
}

// Obscure converts a password to the form rclone.conf stores. The secret goes to rclone on
// standard input.
func (r *Runner) Obscure(ctx context.Context, secret string) (string, error) {
	cmd := exec.CommandContext(ctx, r.binary, obscureArgs(r.configPath)...)
	cmd.Stdin = strings.NewReader(secret)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rclone obscure failed: %w", err)
	}
	obscured := strings.TrimSpace(string(out))
	if obscured == "" {
		return "", errors.New("rclone obscure printed nothing")
	}
	return obscured, nil
}
