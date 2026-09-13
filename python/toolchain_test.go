package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFiles(t *testing.T) {
	dir := t.TempDir()
	pythonPath := filepath.Join(dir, ".python-version")
	uvPath := filepath.Join(dir, ".uv-version")
	if err := os.WriteFile(pythonPath, []byte("3.12.13\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uvPath, []byte("0.12.3\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadFiles(pythonPath, uvPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.13" || config.PythonMinor != "3.12" || config.MinimumUV != "0.12.3" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestLoadFilesRejectsNonExactVersions(t *testing.T) {
	dir := t.TempDir()
	pythonPath := filepath.Join(dir, ".python-version")
	uvPath := filepath.Join(dir, ".uv-version")
	if err := os.WriteFile(pythonPath, []byte("3.12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uvPath, []byte("0.12.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFiles(pythonPath, uvPath); err == nil {
		t.Fatal("non-exact Python version was accepted")
	}
}

func TestLoadFromRootUsesOnlyCanonicalVersionFiles(t *testing.T) {
	canonicalRoot := t.TempDir()
	writeVersionFile(t, canonicalRoot, ".python-version", "3.12.13\n")
	writeVersionFile(t, canonicalRoot, ".uv-version", "0.12.10\n")

	ambientRoot := t.TempDir()
	ambientProject := filepath.Join(ambientRoot, "project")
	if err := os.Mkdir(ambientProject, 0o755); err != nil {
		t.Fatal(err)
	}
	writeVersionFile(t, ambientRoot, ".python-version", "9.9.9\n")
	writeVersionFile(t, ambientProject, ".python-version", "8.8.8\n")
	writeVersionFile(t, ambientProject, ".uv-version", "7.7.7\n")
	writeVersionFile(t, ambientProject, "uv.toml", "invalid = [")
	t.Chdir(ambientProject)
	t.Setenv("UV_CONFIG_FILE", filepath.Join(ambientProject, "uv.toml"))
	t.Setenv("UV_WORKING_DIR", ambientRoot)

	config, err := loadFromRoot(canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.13" || config.MinimumUV != "0.12.10" {
		t.Fatalf("config = %#v, want canonical Python and uv versions", config)
	}

	writeVersionFile(t, canonicalRoot, ".python-version", "3.12.14\n")
	config, err = loadFromRoot(canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.14" {
		t.Fatalf("Python = %q after canonical bump, want 3.12.14", config.Python)
	}
}

func TestLoadFromRootReportsCanonicalVersionFailures(t *testing.T) {
	root := t.TempDir()
	writeVersionFile(t, root, ".uv-version", "0.12.10\n")
	if _, err := loadFromRoot(root); err == nil || !strings.Contains(err.Error(), filepath.Join(root, ".python-version")) {
		t.Fatalf("missing canonical Python error = %v", err)
	}

	writeVersionFile(t, root, ".python-version", "3.12\n")
	if _, err := loadFromRoot(root); err == nil || !strings.Contains(err.Error(), filepath.Join(root, ".python-version")) {
		t.Fatalf("invalid canonical Python error = %v", err)
	}
}

func writeVersionFile(t *testing.T, root, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		actual  string
		minimum string
		want    bool
	}{
		{"0.12.3", "0.12.3", true},
		{"0.12.4", "0.12.3", true},
		{"0.13.0", "0.12.99", true},
		{"0.12.2", "0.12.3", false},
	}
	for _, test := range tests {
		got, err := AtLeast(test.actual, test.minimum)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("AtLeast(%q, %q) = %v, want %v", test.actual, test.minimum, got, test.want)
		}
	}
}
