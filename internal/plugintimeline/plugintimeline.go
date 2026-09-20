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
// manifestRelPath is read from inside root: root is opened with os.OpenRoot, which refuses a
// symlink that would lead outside root, so a custom tool's manifest path can never read a file
// its own root does not contain.
func Store(root, manifestRelPath, destDir string, now time.Time) (bool, error) {
	current, err := readManifest(root, manifestRelPath)
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
	// Written to a temporary file and published with a rename, so a write error, a close
	// error, or the process dying mid-write never leaves a partial file at name: latestCopy
	// would otherwise treat that partial file as the authoritative snapshot.
	tmp, err := os.CreateTemp(destDir, ".plugintimeline-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once the rename has happened
	if _, err := tmp.Write(current); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	// O_EXCL semantics: a stored copy is history and is never overwritten, so this must fail
	// rather than replace an existing name of the same timestamp. CreateTemp already makes
	// the file 0600.
	if err := os.Link(tmp.Name(), name); err != nil {
		return false, err
	}
	return true, nil
}

// readManifest reads manifestRelPath from inside root through os.Root, so a symlink under a
// custom tool's root cannot make the read land outside it.
func readManifest(root, manifestRelPath string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return r.ReadFile(filepath.FromSlash(manifestRelPath))
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
