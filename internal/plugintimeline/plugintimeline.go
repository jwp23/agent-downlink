// Package plugintimeline keeps a dated copy of an agent's plugin manifest each time it
// changes, so a consumer can learn which plugin versions were installed when a session ran.
package plugintimeline

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// timestampLayout sorts by time as plain text, so the latest copy is the greatest name.
const timestampLayout = "20060102T150405Z"

// DestDir is where a manifest's copies live inside a tool's mirror folder.
func DestDir(toolDir, manifestRelPath string) string {
	rel := filepath.FromSlash(manifestRelPath)
	return filepath.Join(toolDir, strings.TrimSuffix(rel, filepath.Ext(rel)))
}

// Store keeps a dated copy of the manifest when it differs from the latest stored copy.
func Store(manifestPath, destDir string, now time.Time) (bool, error) {
	current, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	latest, err := latestCopy(destDir)
	if err != nil {
		return false, err
	}
	if latest != "" {
		previous, err := os.ReadFile(filepath.Join(destDir, latest))
		if err != nil {
			return false, err
		}
		if bytes.Equal(previous, current) {
			return false, nil
		}
	}

	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return false, err
	}
	name := filepath.Join(destDir, now.UTC().Format(timestampLayout)+".json")
	// O_EXCL: a stored copy is history and is never overwritten.
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	if _, err := f.Write(current); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
}

func latestCopy(destDir string) (string, error) {
	entries, err := os.ReadDir(destDir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", nil
	}
	sort.Strings(names)
	return names[len(names)-1], nil
}
