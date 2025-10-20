package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerAppdataPath_MissingFile(t *testing.T) {
	// Test with a non-existent file
	result := loadServerAppdataPath("/nonexistent/path/localhost.yml")
	expected := "/opt"

	if result != expected {
		t.Errorf("Expected %s when file is missing, got %s", expected, result)
	}
}

func TestLoadServerAppdataPath_EmptyVariable(t *testing.T) {
	// Create a temporary file with empty server_appdata_path
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "localhost.yml")

	content := `---
some_other_var: value
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := loadServerAppdataPath(tmpFile)
	expected := "/opt"

	if result != expected {
		t.Errorf("Expected %s when variable is empty, got %s", expected, result)
	}
}

func TestLoadServerAppdataPath_WithCustomPath(t *testing.T) {
	// Create a temporary file with custom server_appdata_path
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "localhost.yml")

	content := `---
server_appdata_path: /opt2
some_other_var: value
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := loadServerAppdataPath(tmpFile)
	expected := "/opt2"

	if result != expected {
		t.Errorf("Expected %s when custom path is set, got %s", expected, result)
	}
}

func TestLoadServerAppdataPath_InvalidYAML(t *testing.T) {
	// Create a temporary file with invalid YAML
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "localhost.yml")

	content := `---
invalid yaml content: [[[
`
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := loadServerAppdataPath(tmpFile)
	expected := "/opt"

	if result != expected {
		t.Errorf("Expected %s when YAML is invalid, got %s", expected, result)
	}
}
