package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSetupAllowed(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	t.Run("missing path", func(t *testing.T) {
		if err := checkSetupAllowed(filepath.Join(base, "missing"), false); err != nil {
			t.Fatalf("missing setup path should be allowed: %v", err)
		}
	})

	t.Run("existing directory is blocked", func(t *testing.T) {
		repoPath := filepath.Join(base, "existing")
		if err := os.Mkdir(repoPath, 0755); err != nil {
			t.Fatal(err)
		}

		err := checkSetupAllowed(repoPath, false)
		if err == nil || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("expected force guidance for existing setup, got %v", err)
		}
	})

	t.Run("force allows existing directory", func(t *testing.T) {
		repoPath := filepath.Join(base, "forced")
		if err := os.Mkdir(repoPath, 0755); err != nil {
			t.Fatal(err)
		}

		if err := checkSetupAllowed(repoPath, true); err != nil {
			t.Fatalf("force should allow an existing setup directory: %v", err)
		}
	})

	t.Run("force does not allow regular file", func(t *testing.T) {
		repoPath := filepath.Join(base, "file")
		if err := os.WriteFile(repoPath, []byte("not a repository"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkSetupAllowed(repoPath, true)
		if err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("expected regular file to be rejected, got %v", err)
		}
	})

	t.Run("force does not allow symlink", func(t *testing.T) {
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0755); err != nil {
			t.Fatal(err)
		}
		repoPath := filepath.Join(base, "symlink")
		if err := os.Symlink(target, repoPath); err != nil {
			t.Fatal(err)
		}

		err := checkSetupAllowed(repoPath, true)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink to be rejected, got %v", err)
		}
	})
}
