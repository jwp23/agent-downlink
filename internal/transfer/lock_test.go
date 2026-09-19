package transfer

import (
	"path/filepath"
	"testing"
)

func TestLockPreventsOverlappingRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "run.lock")
	release, acquired, err := Lock(path)
	if err != nil || !acquired {
		t.Fatalf("first Lock = %v, %v; want acquired", acquired, err)
	}

	_, second, err := Lock(path)
	if err != nil || second {
		t.Errorf("second Lock = %v, %v; want not acquired and no error", second, err)
	}

	release()
	release2, third, err := Lock(path)
	if err != nil || !third {
		t.Fatalf("Lock after release = %v, %v; want acquired", third, err)
	}
	release2()
}
