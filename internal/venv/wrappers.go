package venv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
)

const managedWrapperHeader = "#!/bin/sh\n" + wrapperMarker + "\n"

func installWrappers(commands []string) error {
	return installWrappersAt("/usr/local/bin", commands)
}

func installWrappersAt(directory string, commands []string) error {
	for _, command := range commands {
		content, err := managedWrapperContent(command)
		if err != nil {
			return err
		}
		path, err := wrapperPath(directory, command)
		if err != nil {
			return err
		}
		if err := writeWrapper(path, content); err != nil {
			return err
		}
	}
	return nil
}

func removeStaleWrappers(previousCommands, currentCommands []string) error {
	return removeStaleWrappersAt("/usr/local/bin", previousCommands, currentCommands)
}

func removeStaleWrappersAt(directory string, previousCommands, currentCommands []string) error {
	wanted := make(map[string]bool, len(currentCommands))
	for _, command := range currentCommands {
		wanted[command] = true
	}
	for _, command := range previousCommands {
		if wanted[command] {
			continue
		}
		path, err := wrapperPath(directory, command)
		if err != nil {
			return err
		}
		managed, err := isManagedWrapper(path)
		if err != nil {
			return err
		}
		if managed {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale wrapper %s: %w", path, err)
			}
		}
	}
	return nil
}

func wrapperPath(directory, command string) (string, error) {
	if filepath.Base(command) != command || strings.ContainsAny(command, "\n\r") {
		return "", fmt.Errorf("unsafe venv command name %q", command)
	}
	return filepath.Join(directory, command), nil
}

func managedWrapperContent(command string) ([]byte, error) {
	if _, err := wrapperPath("/usr/local/bin", command); err != nil {
		return nil, err
	}
	content := fmt.Sprintf(
		"%svenv_bin=$(/usr/bin/readlink -f %s/bin) || exit 1\nPATH=\"${venv_bin}${PATH:+:${PATH}}\"\nexport PATH\nexec %s \"$@\"\n",
		managedWrapperHeader,
		filepath.Join(constants.AnsibleVenvPath, "venv"),
		filepath.Join(constants.AnsibleVenvPath, "venv", "bin", command),
	)
	return []byte(content), nil
}

func writeWrapper(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sb-wrapper-*")
	if err != nil {
		return fmt.Errorf("create wrapper for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write wrapper for %s: %w", path, err)
	}
	if err := temporary.Chmod(0755); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate wrapper %s: %w", path, err)
	}
	return nil
}

func isManagedWrapper(path string) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	header := make([]byte, len(managedWrapperHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(header, []byte(managedWrapperHeader)), nil
}
