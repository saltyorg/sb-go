package python

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestActivateGenerationReplacesActiveWithoutPreviousLink(t *testing.T) {
	layout := newTestReleaseLayout(t)
	oldVenv, _ := newTestGeneration(t, layout, "old", "python-old")
	newVenv, _ := newTestGeneration(t, layout, "new", "python-new")
	activePath := filepath.Join(layout.ansibleRoot, "venv")
	if err := os.Symlink(oldVenv, activePath); err != nil {
		t.Fatal(err)
	}

	if err := activateGenerationAt(newVenv, layout); err != nil {
		t.Fatal(err)
	}
	assertSymlinkTarget(t, activePath, newVenv)
	if _, err := os.Lstat(filepath.Join(layout.ansibleRoot, "previous")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous link exists after activation: %v", err)
	}
}

func TestActivateGenerationMigratesLegacyDirectory(t *testing.T) {
	layout := newTestReleaseLayout(t)
	newVenv, _ := newTestGeneration(t, layout, "new", "python-new")
	legacyVenv := filepath.Join(layout.ansibleRoot, "venv")
	if err := os.MkdirAll(filepath.Join(legacyVenv, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyVenv, "legacy-marker"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := activateGenerationAt(newVenv, layout); err != nil {
		t.Fatal(err)
	}
	assertSymlinkTarget(t, legacyVenv, newVenv)

	entries, err := os.ReadDir(layout.ansibleReleases)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("release count after legacy migration = %d, want 2", len(entries))
	}

	if err := cleanupGenerationsAt(layout); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(layout.ansibleReleases)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "new" {
		t.Fatalf("retained releases = %v, want only new", entryNames(entries))
	}
}

func TestCleanupGenerationsRetainsOnlyActiveGeneration(t *testing.T) {
	layout := newTestReleaseLayout(t)
	oldVenv, oldPython := newTestGeneration(t, layout, "old", "python-old")
	newVenv, newPython := newTestGeneration(t, layout, "new", "python-new")
	activePath := filepath.Join(layout.ansibleRoot, "venv")
	if err := os.Symlink(newVenv, activePath); err != nil {
		t.Fatal(err)
	}
	previousPath := filepath.Join(layout.ansibleRoot, "previous")
	if err := os.Symlink(oldVenv, previousPath); err != nil {
		t.Fatal(err)
	}

	if err := cleanupGenerationsAt(layout); err != nil {
		t.Fatal(err)
	}
	assertExists(t, filepath.Dir(newVenv))
	assertExists(t, newPython)
	assertNotExist(t, filepath.Dir(oldVenv))
	assertNotExist(t, oldPython)
	assertNotExist(t, previousPath)
	assertSymlinkTarget(t, activePath, newVenv)
}

func TestCleanupGenerationsFailsClosedWithoutActiveGeneration(t *testing.T) {
	layout := newTestReleaseLayout(t)
	_, oldPython := newTestGeneration(t, layout, "old", "python-old")

	if err := cleanupGenerationsAt(layout); err == nil {
		t.Fatal("cleanupGenerationsAt() unexpectedly succeeded without an active generation")
	}
	assertExists(t, filepath.Join(layout.ansibleReleases, "old"))
	assertExists(t, oldPython)
}

func TestCleanupGenerationsRejectsActiveTargetOutsideManagedReleases(t *testing.T) {
	layout := newTestReleaseLayout(t)
	_, oldPython := newTestGeneration(t, layout, "old", "python-old")
	externalVenv := filepath.Join(t.TempDir(), "venv")
	if err := os.MkdirAll(externalVenv, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalVenv, filepath.Join(layout.ansibleRoot, "venv")); err != nil {
		t.Fatal(err)
	}

	if err := cleanupGenerationsAt(layout); err == nil {
		t.Fatal("cleanupGenerationsAt() unexpectedly accepted an external active target")
	}
	assertExists(t, filepath.Join(layout.ansibleReleases, "old"))
	assertExists(t, oldPython)
}

func TestCleanupGenerationsRejectsPythonOutsideManagedReleases(t *testing.T) {
	layout := newTestReleaseLayout(t)
	activeVenv, retainedPython := newTestGeneration(t, layout, "active", "python-active")
	_, oldPython := newTestGeneration(t, layout, "old", "python-old")
	generationDir := filepath.Dir(activeVenv)
	manifest, err := readManifest(generationDir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.PythonInstall = filepath.Join(t.TempDir(), "external-python")
	if err := writeManifest(generationDir, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(activeVenv, filepath.Join(layout.ansibleRoot, "venv")); err != nil {
		t.Fatal(err)
	}

	if err := cleanupGenerationsAt(layout); err == nil {
		t.Fatal("cleanupGenerationsAt() unexpectedly accepted an external Python installation")
	}
	assertExists(t, filepath.Join(layout.ansibleReleases, "active"))
	assertExists(t, filepath.Join(layout.ansibleReleases, "old"))
	assertExists(t, retainedPython)
	assertExists(t, oldPython)
}

func TestRemoveObsoletePreviousLinkRejectsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "previous")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeObsoletePreviousLink(path); err == nil {
		t.Fatal("removeObsoletePreviousLink() unexpectedly removed a directory")
	}
	assertExists(t, path)
}

func newTestReleaseLayout(t *testing.T) releaseLayout {
	t.Helper()
	root := t.TempDir()
	layout := releaseLayout{
		ansibleRoot:     filepath.Join(root, "ansible"),
		ansibleReleases: filepath.Join(root, "ansible", "releases"),
		pythonReleases:  filepath.Join(root, "python", "releases"),
	}
	if err := os.MkdirAll(layout.ansibleReleases, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.pythonReleases, 0o755); err != nil {
		t.Fatal(err)
	}
	return layout
}

func newTestGeneration(t *testing.T, layout releaseLayout, generation, python string) (string, string) {
	t.Helper()
	generationDir := filepath.Join(layout.ansibleReleases, generation)
	venvPath := filepath.Join(generationDir, "venv")
	pythonPath := filepath.Join(layout.pythonReleases, python)
	if err := os.MkdirAll(filepath.Join(venvPath, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pythonPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := &Manifest{
		SchemaVersion: manifestSchemaVersion,
		GenerationID:  generation,
		PythonInstall: pythonPath,
	}
	if err := writeManifest(generationDir, manifest); err != nil {
		t.Fatal(err)
	}
	return venvPath, pythonPath
}

func assertSymlinkTarget(t *testing.T, path, want string) {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s resolves to %s, want %s", path, got, want)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent: %v", path, err)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
