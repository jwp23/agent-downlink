package plugintimeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	first  = time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	second = time.Date(2026, 3, 2, 17, 5, 9, 0, time.FixedZone("west", -7*3600)) // 2026-03-03T00:05:09Z
)

func setup(t *testing.T, manifest string) (manifestPath, destDir string) {
	t.Helper()
	dir := t.TempDir()
	manifestPath = filepath.Join(dir, "installed_plugins.json")
	if manifest != "" {
		if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return manifestPath, filepath.Join(dir, "mirror", "workstation", "claude-code", "plugins", "installed_plugins")
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestFirstRunStoresACopyNamedByUTCTime(t *testing.T) {
	manifest, dest := setup(t, `{"plugins":{"a":"1.0.0"}}`)
	stored, err := Store(manifest, dest, first)
	if err != nil || !stored {
		t.Fatalf("Store = %v, %v; want true, nil", stored, err)
	}
	got := names(t, dest)
	if len(got) != 1 || got[0] != "20260301T093000Z.json" {
		t.Fatalf("stored files = %v, want [20260301T093000Z.json]", got)
	}
	b, _ := os.ReadFile(filepath.Join(dest, got[0]))
	if string(b) != `{"plugins":{"a":"1.0.0"}}` {
		t.Errorf("copy content = %q", b)
	}
}

func TestUnchangedManifestStoresNothing(t *testing.T) {
	manifest, dest := setup(t, `{"plugins":{"a":"1.0.0"}}`)
	if _, err := Store(manifest, dest, first); err != nil {
		t.Fatal(err)
	}
	stored, err := Store(manifest, dest, second)
	if err != nil || stored {
		t.Fatalf("Store = %v, %v; want false, nil", stored, err)
	}
	if got := names(t, dest); len(got) != 1 {
		t.Errorf("stored files = %v, want one", got)
	}
}

func TestChangedManifestStoresASecondCopyAndKeepsTheFirst(t *testing.T) {
	manifest, dest := setup(t, `{"plugins":{"a":"1.0.0"}}`)
	if _, err := Store(manifest, dest, first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"plugins":{"a":"1.1.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stored, err := Store(manifest, dest, second)
	if err != nil || !stored {
		t.Fatalf("Store = %v, %v; want true, nil", stored, err)
	}
	got := names(t, dest)
	if len(got) != 2 || got[0] != "20260301T093000Z.json" || got[1] != "20260303T000509Z.json" {
		t.Fatalf("stored files = %v", got)
	}
	old, _ := os.ReadFile(filepath.Join(dest, got[0]))
	if string(old) != `{"plugins":{"a":"1.0.0"}}` {
		t.Errorf("the first copy was altered: %q", old)
	}
}

func TestRevertedManifestIsStoredAgain(t *testing.T) {
	// The comparison is with the latest copy, not with every copy: a revert is an event.
	manifest, dest := setup(t, "v1")
	_, _ = Store(manifest, dest, first)
	_ = os.WriteFile(manifest, []byte("v2"), 0o600)
	_, _ = Store(manifest, dest, first.Add(time.Hour))
	_ = os.WriteFile(manifest, []byte("v1"), 0o600)
	stored, err := Store(manifest, dest, first.Add(2*time.Hour))
	if err != nil || !stored {
		t.Fatalf("Store = %v, %v; want true, nil", stored, err)
	}
	if got := names(t, dest); len(got) != 3 {
		t.Errorf("stored files = %v, want three", got)
	}
}

func TestMissingManifestIsNotAnError(t *testing.T) {
	manifest, dest := setup(t, "")
	stored, err := Store(manifest, dest, first)
	if err != nil || stored {
		t.Fatalf("Store = %v, %v; want false, nil", stored, err)
	}
	if got := names(t, dest); len(got) != 0 {
		t.Errorf("stored files = %v, want none", got)
	}
}

func TestACopyIsNeverOverwritten(t *testing.T) {
	manifest, dest := setup(t, "v1")
	_, _ = Store(manifest, dest, first)
	_ = os.WriteFile(manifest, []byte("v2"), 0o600)
	if _, err := Store(manifest, dest, first); err == nil { // same second
		t.Error("Store overwrote or silently skipped a copy with the same timestamp; want an error so the next run stores it")
	}
	b, _ := os.ReadFile(filepath.Join(dest, "20260301T093000Z.json"))
	if string(b) != "v1" {
		t.Errorf("existing copy = %q, want it untouched", b)
	}
}

func TestDestDir(t *testing.T) {
	got := DestDir("/m/workstation/claude-code", "plugins/installed_plugins.json")
	if want := filepath.FromSlash("/m/workstation/claude-code/plugins/installed_plugins"); got != want {
		t.Errorf("DestDir = %q, want %q", got, want)
	}
}
