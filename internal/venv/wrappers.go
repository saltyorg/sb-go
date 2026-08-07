package venv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
)

func installWrappers(venvPath string, commands []string, removeStale bool) error {
	wanted := make(map[string]bool, len(commands))
	for _, command := range commands {
		content, err := managedWrapperContent(command)
		if err != nil {
			return err
		}
		wanted[command] = true
		if err := writeWrapper(filepath.Join("/usr/local/bin", command), content); err != nil {
			return err
		}
	}
	if !removeStale {
		return nil
	}
	entries, err := os.ReadDir("/usr/local/bin")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if wanted[entry.Name()] {
			continue
		}
		path := filepath.Join("/usr/local/bin", entry.Name())
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

func managedWrapperContent(command string) ([]byte, error) {
	if filepath.Base(command) != command || strings.ContainsAny(command, "\n\r") {
		return nil, fmt.Errorf("unsafe venv command name %q", command)
	}
	content := fmt.Sprintf(
		"#!/bin/sh\n%s\nvenv_bin=$(/usr/bin/readlink -f %s/bin) || exit 1\nPATH=\"${venv_bin}${PATH:+:${PATH}}\"\nexport PATH\nexec %s \"$@\"\n",
		wrapperMarker,
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
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), wrapperMarker), nil
}
