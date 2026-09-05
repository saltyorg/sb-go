package facts

// Temporary compatibility persistence for the legacy Cobra adapter. Remove
// with that adapter; the interactive editor uses Session exclusively.
import (
	"errors"
	"fmt"
	"gopkg.in/ini.v1"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// setFileOwnershipAndPermissions sets ownership and mode through an already-open
// file descriptor so path replacement cannot redirect the operation.
func setFileOwnershipAndPermissions(file *os.File, saltboxUser string) error {
	if err := file.Chmod(0640); err != nil {
		return fmt.Errorf("failed to set file permissions: %v", err)
	}

	passwd, err := user.Lookup(saltboxUser)
	if err == nil {
		uid, _ := strconv.Atoi(passwd.Uid)
		gid, _ := strconv.Atoi(passwd.Gid)
		if err := file.Chown(uid, gid); err != nil {
			// Just log the error but don't fail the operation
			fmt.Printf("Warning: Failed to set ownership to %s: %v\n", saltboxUser, err)
		}
	} else {
		fmt.Printf("Warning: Failed to lookup user %s: %v\n", saltboxUser, err)
	}

	return nil
}

func inspectFactsDirectory(dir string, create bool) (bool, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, fmt.Errorf("create facts directory: %w", err)
		}
		info, err = os.Lstat(dir)
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect facts directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("facts directory %s must not be a symlink", dir)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("facts path %s is not a directory", dir)
	}
	return true, nil
}

// loadINIFile opens an existing regular file and verifies that the path still
// names the same inode after open. This rejects symlinks and path-swap races.
func loadINIFile(filePath string) (*ini.File, bool, error) {
	dirExists, err := inspectFactsDirectory(filepath.Dir(filePath), false)
	if err != nil || !dirExists {
		return nil, false, err
	}

	pathInfo, err := os.Lstat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect facts file: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("facts file %s must not be a symlink", filePath)
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, false, fmt.Errorf("facts file %s is not a regular file", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, false, fmt.Errorf("open facts file: %w", err)
	}
	defer func() { _ = file.Close() }()

	openedInfo, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("inspect open facts file: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return nil, false, fmt.Errorf("facts file %s changed while it was being opened", filePath)
	}

	cfg, err := ini.Load(file)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load ini file: %v", err)
	}
	return cfg, true, nil
}

// writeINIFile atomically replaces the destination from a same-directory
// temporary file. Existing symlinks are never followed.
func writeINIFile(filePath string, cfg *ini.File, saltboxUser string) error {
	dir := filepath.Dir(filePath)
	if _, err := inspectFactsDirectory(dir, true); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary facts file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := setFileOwnershipAndPermissions(temporary, saltboxUser); err != nil {
		return err
	}
	if _, err := cfg.WriteTo(temporary); err != nil {
		return fmt.Errorf("write temporary facts file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary facts file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary facts file: %w", err)
	}
	if err := os.Rename(temporaryPath, filePath); err != nil {
		return fmt.Errorf("replace facts file: %w", err)
	}
	removeTemporary = false

	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open facts directory for sync: %w", err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync facts directory: %w", err)
	}
	return nil
}

// loadAllInstances loads all instances and their facts from an ini file
func LegacyLoadAll(filePath string) (map[string]map[string]string, error) {
	allInstances := make(map[string]map[string]string)
	cfg, exists, err := loadINIFile(filePath)
	if err != nil {
		return allInstances, err
	}
	if !exists {
		return allInstances, nil
	}

	// Get all sections (instances)
	for _, section := range cfg.Sections() {
		// Skip the default INI section if it has no keys
		if section.Name() == ini.DefaultSection && len(section.Keys()) == 0 {
			continue
		}

		// Create a map for this instance's facts
		facts := make(map[string]string)

		// Get all keys and values for this instance
		for _, key := range section.Keys() {
			facts[key.Name()] = key.Value()
		}

		// Add this instance to the map of all instances
		if len(facts) > 0 {
			allInstances[section.Name()] = facts
		}
	}

	return allInstances, nil
}

// loadFacts loads facts from an ini file for a given role and instance
func LegacyLoad(filePath, instance string, defaults map[string]string) (map[string]string, error) {
	facts := make(map[string]string)

	// Copy defaults into facts
	maps.Copy(facts, defaults)

	cfg, exists, err := loadINIFile(filePath)
	if err != nil {
		return facts, err
	}
	if !exists {
		return facts, nil
	}

	// Check if the instance section exists
	if !cfg.HasSection(instance) {
		return facts, nil // Return defaults if the section doesn't exist
	}

	// Get the section for the instance
	section := cfg.Section(instance)

	// Get all keys and values, overriding defaults
	for _, key := range section.Keys() {
		value := key.Value()
		if value == "None" {
			// Use default value if stored value is 'None' and a default exists
			if defaultVal, exists := defaults[key.Name()]; exists {
				facts[key.Name()] = defaultVal
				continue
			}
		}
		facts[key.Name()] = value
	}

	return facts, nil
}

// saveFacts saves facts to an ini file
func LegacySave(filePath, instance string, keys map[string]string, saltboxUser string) (map[string]string, bool, error) {
	if _, err := inspectFactsDirectory(filepath.Dir(filePath), true); err != nil {
		return nil, false, err
	}

	var (
		facts   map[string]string
		changed bool
	)
	err := withFactFileLock(filePath, func() error {
		var err error
		facts, changed, err = saveFactsUnlocked(filePath, instance, keys, saltboxUser)
		return err
	})
	return facts, changed, err
}

func saveFactsUnlocked(filePath, instance string, keys map[string]string, saltboxUser string) (map[string]string, bool, error) {
	facts := make(map[string]string)
	changed := false

	// Create a new ini file config
	cfg := ini.Empty()

	// If a regular file exists, load it without following symlinks.
	existing, fileExists, err := loadINIFile(filePath)
	if err != nil {
		return facts, false, err
	}
	if fileExists {
		cfg = existing
	}

	// Ensure section exists
	section, err := cfg.NewSection(instance)
	if err != nil {
		if !cfg.HasSection(instance) {
			return facts, false, fmt.Errorf("failed to create section: %v", err)
		}
		section = cfg.Section(instance)
	}

	// If it's a new section, mark as changed
	if !fileExists || !cfg.HasSection(instance) {
		changed = true
	}

	// Update keys and track changes
	for key, value := range keys {
		// Check if the key exists and has the same value
		if section.HasKey(key) {
			existingValue := section.Key(key).Value()
			if existingValue != value {
				section.Key(key).SetValue(value)
				changed = true
			}
		} else {
			// Key doesn't exist, add it
			_, err := section.NewKey(key, value)
			if err != nil {
				return facts, false, fmt.Errorf("failed to set key %s: %v", key, err)
			}
			changed = true
		}
		facts[key] = value
	}

	// Load all existing keys into facts
	for _, key := range section.Keys() {
		if _, exists := facts[key.Name()]; !exists {
			facts[key.Name()] = key.Value()
		}
	}

	// Save the file if changes were made
	if changed {
		if err := writeINIFile(filePath, cfg, saltboxUser); err != nil {
			return facts, false, fmt.Errorf("failed to save ini file: %v", err)
		}
	}

	return facts, changed, nil
}

// deleteFacts deletes facts from an ini file
func LegacyDelete(filePath, deleteType, instance string, keys map[string]string, saltboxUser string) (bool, error) {
	dirExists, err := inspectFactsDirectory(filepath.Dir(filePath), false)
	if err != nil || !dirExists {
		return false, err
	}

	var changed bool
	err = withFactFileLock(filePath, func() error {
		var err error
		changed, err = deleteFactsUnlocked(filePath, deleteType, instance, keys, saltboxUser)
		return err
	})
	return changed, err
}

func deleteFactsUnlocked(filePath, deleteType, instance string, keys map[string]string, saltboxUser string) (bool, error) {
	changed := false

	// For role deletion, just remove the file
	if deleteType == "role" {
		if _, err := inspectFactsDirectory(filepath.Dir(filePath), false); err != nil {
			return false, err
		}
		if _, err := os.Lstat(filePath); errors.Is(err, os.ErrNotExist) {
			return false, nil // File doesn't exist, no change
		} else if err != nil {
			return false, fmt.Errorf("inspect facts file: %v", err)
		}

		if err := os.Remove(filePath); err != nil {
			return false, fmt.Errorf("failed to delete file: %v", err)
		}

		return true, nil
	}

	// For instance or key deletion, load without following symlinks.
	cfg, exists, err := loadINIFile(filePath)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	// If the instance doesn't exist, no change
	if !cfg.HasSection(instance) {
		return false, nil
	}

	switch deleteType {
	case "instance":
		// Remove the entire section
		cfg.DeleteSection(instance)
		changed = true
	case "key":
		// Remove specific keys
		section := cfg.Section(instance)
		for key := range keys {
			if section.HasKey(key) {
				section.DeleteKey(key)
				changed = true
			}
		}
	}

	// Save changes if any were made
	if changed {
		if err := writeINIFile(filePath, cfg, saltboxUser); err != nil {
			return false, fmt.Errorf("failed to save ini file: %v", err)
		}
	}

	return changed, nil
}
