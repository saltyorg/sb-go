package completion

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const completionComment = "# sb-go completion "

// This comment identifies retired aliases when scanning shared directories.
// Current executable names are always replaced, regardless of their contents.
func completionContent(shell Shell, name string, body []byte) []byte {
	marker := fmt.Sprintf("%s%s %s\n", completionComment, shell, name)
	if shell == Zsh && bytes.HasPrefix(body, []byte("#compdef ")) {
		// compinit inspects the first line. Keep Cobra's registration there.
		line, rest, found := bytes.Cut(body, []byte("\n"))
		if found {
			return append(append(append(bytes.Clone(line), '\n'), marker...), rest...)
		}
	}
	return append([]byte(marker), body...)
}

func ensureDirectory(path string) error {
	if err := os.Mkdir(path, 0755); err == nil {
		// Apply the shared mode to our new directory even under a restrictive umask.
		return os.Chmod(path, 0755)
	} else if errors.Is(err, os.ErrExist) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return fmt.Errorf("completion directory path is not a directory: %s", path)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("create completion directory %s: %w", path, err)
	}
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return ensureDirectory(path)
}

func (m *Manager) atomicWrite(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".sb-completion-*")
	if err != nil {
		return fmt.Errorf("create temporary completion: %w", err)
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0644); err != nil {
		return errors.Join(err, file.Close())
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace completion %s: %w", path, err)
	}
	return nil
}

func removeCompletion(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("remove obsolete completion %s: %w", path, err)
	}
	return true, nil
}
