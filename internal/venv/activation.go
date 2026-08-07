package venv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
)

type releaseLayout struct {
	ansibleRoot     string
	ansibleReleases string
	pythonReleases  string
}

func managedReleaseLayout() releaseLayout {
	return releaseLayout{
		ansibleRoot:     constants.AnsibleVenvPath,
		ansibleReleases: constants.AnsibleReleasesPath,
		pythonReleases:  constants.PythonReleasesPath,
	}
}

func activateGeneration(newVenvPath string) error {
	return activateGenerationAt(newVenvPath, managedReleaseLayout())
}

func activateGenerationAt(newVenvPath string, layout releaseLayout) error {
	if err := os.MkdirAll(layout.ansibleReleases, 0755); err != nil {
		return fmt.Errorf("create Ansible releases directory: %w", err)
	}
	activePath := filepath.Join(layout.ansibleRoot, "venv")
	legacyMoved := false
	legacyPath := ""

	info, err := os.Lstat(activePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("inspect active venv before switch: %w", err)
	case info.Mode()&os.ModeSymlink != 0:
		// A managed active link can be replaced atomically below.
	case info.IsDir():
		legacyID := "legacy-" + time.Now().UTC().Format("20060102T150405.000000000")
		legacyDir := filepath.Join(layout.ansibleReleases, legacyID)
		if err := os.MkdirAll(legacyDir, 0755); err != nil {
			return fmt.Errorf("create legacy generation: %w", err)
		}
		legacyPath = filepath.Join(legacyDir, "venv")
		if err := os.Rename(activePath, legacyPath); err != nil {
			return fmt.Errorf("adopt legacy venv: %w", err)
		}
		legacyMoved = true
	default:
		return fmt.Errorf("%s is not a managed venv directory or symlink", activePath)
	}

	if err := atomicSymlink(newVenvPath, activePath); err != nil {
		if legacyMoved {
			_ = os.Rename(legacyPath, activePath)
		}
		return fmt.Errorf("activate Ansible venv: %w", err)
	}
	return nil
}

func atomicSymlink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(linkPath), ".link-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := os.Symlink(target, temporaryPath); err != nil {
		return err
	}
	return os.Rename(temporaryPath, linkPath)
}

func cleanupGenerations() error {
	return cleanupGenerationsAt(managedReleaseLayout())
}

func cleanupGenerationsAt(layout releaseLayout) error {
	if err := removeObsoletePreviousLink(filepath.Join(layout.ansibleRoot, "previous")); err != nil {
		return err
	}

	activePath := filepath.Join(layout.ansibleRoot, "venv")
	target, err := filepath.EvalSymlinks(activePath)
	if err != nil {
		return fmt.Errorf("resolve active generation %s: %w", activePath, err)
	}
	generationDir := filepath.Dir(target)
	if !directChildOf(generationDir, layout.ansibleReleases) {
		return fmt.Errorf("active generation %s is not directly under managed releases %s", generationDir, layout.ansibleReleases)
	}
	manifest, err := readManifest(generationDir)
	if err != nil {
		return err
	}
	if manifest.PythonInstall == "" || !directChildOf(manifest.PythonInstall, layout.pythonReleases) {
		return fmt.Errorf("active Python installation %s is not directly under managed releases %s", manifest.PythonInstall, layout.pythonReleases)
	}

	keepGenerations := map[string]bool{generationDir: true}
	keepPython := map[string]bool{manifest.PythonInstall: true}
	if err := removeUnretainedDirectories(layout.ansibleReleases, keepGenerations); err != nil {
		return err
	}
	return removeUnretainedDirectories(layout.pythonReleases, keepPython)
}

func directChildOf(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && filepath.Dir(relative) == "."
}

func removeObsoletePreviousLink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect obsolete previous venv link: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("obsolete previous venv path %s is not a symlink", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove obsolete previous venv link: %w", err)
	}
	return nil
}

func removeUnretainedDirectories(root string, keep map[string]bool) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if keep[path] {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove unretained release %s: %w", path, err)
		}
	}
	return nil
}

func removePythonIfUnreferenced(path string) error {
	target, err := filepath.EvalSymlinks(filepath.Join(constants.AnsibleVenvPath, "venv"))
	if err == nil {
		manifest, manifestErr := readManifest(filepath.Dir(target))
		if manifestErr == nil && manifest.PythonInstall == path {
			return nil
		}
	}
	return os.RemoveAll(path)
}
