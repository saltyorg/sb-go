package python

import (
	"os"
	"path/filepath"
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
