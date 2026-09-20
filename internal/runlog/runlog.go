// Package runlog is the tool's own log file, so that "where are the logs" has the same
// answer on every OS whatever the scheduler does with a job's output.
package runlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Log appends run history to a file and rotates it by size.
type Log struct {
	Path     string
	MaxBytes int64
}

// New returns a log that rotates at about 5 MiB.
func New(path string) *Log {
	return &Log{Path: path, MaxBytes: 5 << 20}
}

// Append writes one step's outcome, with each line of detail indented beneath it.
func (l *Log) Append(now time.Time, step, outcome, detail string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  %s\n", now.UTC().Format(time.RFC3339), step, outcome)
	for _, line := range strings.Split(strings.TrimRight(detail, "\n"), "\n") {
		if line != "" {
			b.WriteString("    " + line + "\n")
		}
	}
	entry := b.String()

	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	if err := l.rotateIfFull(int64(len(entry))); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(entry); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// rotateIfFull keeps exactly one old file. An empty log is never rotated, so an entry larger
// than the limit is still written.
func (l *Log) rotateIfFull(incoming int64) error {
	info, err := os.Stat(l.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 || info.Size()+incoming <= l.MaxBytes {
		return nil
	}
	return os.Rename(l.Path, l.Path+".1")
}
