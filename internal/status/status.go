// Package status records the outcome of each step of a run and renders the status report.
// status.json is also read by consumers.
package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxErrorLines bounds what status.json holds for a step; the log keeps the full output.
const maxErrorLines = 20

// Step is what is known about one step.
type Step struct {
	LastAttempt time.Time `json:"last_attempt"`
	LastSuccess time.Time `json:"last_success,omitzero"`
	Error       string    `json:"error,omitempty"`
}

// File is status.json.
type File struct {
	Steps map[string]Step `json:"steps"`
}

// Load reads status.json. A machine that has never run has none.
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Record notes an attempt at a step. A failure keeps the time of the last success, so the
// report can say how stale the step is.
func (f *File) Record(step string, now time.Time, ok bool, errText string) {
	if f.Steps == nil {
		f.Steps = map[string]Step{}
	}
	s := f.Steps[step]
	s.LastAttempt = now.UTC()
	if ok {
		s.LastSuccess = now.UTC()
		s.Error = ""
	} else {
		s.Error = truncate(errText)
	}
	f.Steps[step] = s
}

func truncate(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= maxErrorLines {
		return strings.Join(lines, "\n")
	}
	kept := append([]string{}, lines[:maxErrorLines]...)
	kept = append(kept, fmt.Sprintf("... %d more lines in the log", len(lines)-maxErrorLines))
	return strings.Join(kept, "\n")
}

// Save writes status.json through a temporary file, so a consumer never reads a torn file.
func (f File) Save(path string) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".status.json-*") // CreateTemp makes the file 0600
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once the rename has happened
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
