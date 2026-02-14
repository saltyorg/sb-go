package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseYAMLFileInvalidYAML(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "invalid.yml")
	if err := os.WriteFile(configPath, []byte("invalid: [yaml"), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, _, err := parseYAMLFile(configPath)
	if err == nil {
		t.Fatal("expected invalid YAML error, got nil")
	}

	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("expected invalid YAML error, got: %v", err)
	}
}

func TestProcessValidationJobInvalidYAMLBeforeSchemaCheck(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "settings.yml")
	missingSchemaPath := filepath.Join(tmpDir, "missing.schema.yml")

	if err := os.WriteFile(configPath, []byte("invalid: [yaml"), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	job := configValidationJob{
		configPath: configPath,
		schemaPath: missingSchemaPath,
		name:       "settings.yml",
		optional:   false,
	}

	err := processValidationJob(job, false)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("expected invalid YAML error, got: %v", err)
	}

	if strings.Contains(err.Error(), "schema file not found") {
		t.Fatalf("expected YAML validation to run before schema checks, got: %v", err)
	}
}
