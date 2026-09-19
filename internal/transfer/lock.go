package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Lock takes the run lock without waiting, so a run that overlaps another can exit quietly.
// The lock is an flock on the file, released by the kernel if the process dies; the file
// itself is never removed.
func Lock(path string) (release func(), acquired bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { _ = f.Close() }, true, nil
}
