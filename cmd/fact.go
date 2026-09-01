package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

// factConfig holds the configuration for the fact command
type factConfig struct {
	method     string
	deleteType string
	keyValues  []string
}

// factCmd represents the fact command
func newFactCommand() *cobra.Command {
	config := &factConfig{}
	factCmd := &cobra.Command{
		Use:   "fact [role] [instance]",
		Short: "Manage Saltbox configuration facts",
		Long: `This command allows loading, saving, and deleting configuration facts
stored in INI files located in the /opt/saltbox directory.

Example usage:
  sb fact role
  sb fact role instance
  sb fact role instance --method=save --key key1=value --key key2=value
  sb fact role instance --method=delete --delete-type=key --key key1
  sb fact role instance --method=delete --delete-type=instance
  sb fact role --method=delete --delete-type=role`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFactCommand(cmd, args, config)
		},
	}
	factCmd.Flags().StringVar(&config.method, "method", "load", "Method to use (load, save, delete)")
	factCmd.Flags().StringVar(&config.deleteType, "delete-type", "", "Type of deletion (role, instance, key)")
	factCmd.Flags().StringSliceVar(&config.keyValues, "key", []string{}, "Key-value pairs (format: key=value)")
	return factCmd
}

// runFactCommand handles the main logic for the fact command
func runFactCommand(cmd *cobra.Command, args []string, config *factConfig) error {
	if err := validateFactCommand(args, config); err != nil {
		return err
	}

	role := args[0]
	// Get a file path for the role
	filePath := getFilePath(role)

	// Parse key-value pairs
	keys := parseKeyValues(config.keyValues)

	switch config.method {
	case "load":
		// Check if a specific instance was requested
		if len(args) > 1 {
			// Load a specific instance
			instance := args[1]
			facts, err := loadFacts(filePath, instance, keys)
			if err != nil {
				return fmt.Errorf("error loading facts: %v", err)
			}

			if len(facts) == 0 {
				fmt.Printf("No facts found for role '%s', instance '%s'\n", role, instance)
				return nil
			}

			// Display facts for the specific instance
			fmt.Printf("Facts for role '%s', instance '%s':\n", role, instance)
			displayFacts(facts)
		} else {
			// Load all instances for the role
			instances, err := loadAllInstances(filePath)
			if err != nil {
				return fmt.Errorf("error loading instances: %v", err)
			}

			if len(instances) == 0 {
				fmt.Printf("No facts found for role '%s'\n", role)
				return nil
			}

			// Display facts for all instances
			fmt.Printf("Facts for role '%s':\n", role)

			// Sort instance names for a consistent output
			instanceNames := make([]string, 0, len(instances))
			for instance := range instances {
				instanceNames = append(instanceNames, instance)
			}
			sortStrings(instanceNames)

			// Display each instance
			for _, instance := range instanceNames {
				facts := instances[instance]
				fmt.Printf("\nInstance: %s\n", instance)
				displayFacts(facts)
			}
		}
		return nil

	case "save":
		// For save, we must have an instance
		if len(args) < 2 {
			fmt.Println("Error: Instance name is required for save method")
			_ = cmd.Help()
			normalStyle := lipgloss.NewStyle()
			return fmt.Errorf("%s", normalStyle.Render("instance name is required for save method"))
		}
		instance := args[1]

		// Get the Saltbox user for owner/group
		saltboxUser, err := host.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting Saltbox user: %v", err)
		}

		facts, changed, err := saveFacts(filePath, instance, keys, saltboxUser)
		if err != nil {
			return fmt.Errorf("error saving facts: %v", err)
		}

		if changed {
			fmt.Println("Facts were updated")
		} else {
			fmt.Println("No changes were made")
		}

		// Display saved facts
		fmt.Printf("Facts for role '%s', instance '%s':\n", role, instance)
		displayFacts(facts)
		return nil

	case "delete":
		if config.deleteType == "" {
			fmt.Println("Error: delete-type is required for delete method")
			normalStyle := lipgloss.NewStyle()
			return fmt.Errorf("%s", normalStyle.Render("delete-type is required for delete method"))
		}

		// Get the Saltbox user for owner/group if needed for cleanup
		saltboxUser, err := host.GetSaltboxUser()
		if err != nil {
			return fmt.Errorf("error getting Saltbox user: %v", err)
		}

		// Handle delete based on type
		if config.deleteType == "role" {
			// No instance needed for role deletion
			changed, err := deleteFacts(filePath, config.deleteType, "", keys, saltboxUser)
			if err != nil {
				return fmt.Errorf("error deleting facts: %v", err)
			}

			if changed {
				fmt.Printf("Role '%s' was deleted\n", role)
			} else {
				fmt.Println("No changes were made")
			}
			return nil
		} else {
			// For instance or key deletion, we need an instance
			if len(args) < 2 {
				fmt.Println("Error: Instance name is required for instance or key deletion")
				_ = cmd.Help()
				normalStyle := lipgloss.NewStyle()
				return fmt.Errorf("%s", normalStyle.Render("instance name is required for instance or key deletion"))
			}
			instance := args[1]

			changed, err := deleteFacts(filePath, config.deleteType, instance, keys, saltboxUser)
			if err != nil {
				return fmt.Errorf("error deleting facts: %v", err)
			}

			if changed {
				switch config.deleteType {
				case "instance":
					fmt.Printf("Instance '%s' of role '%s' was deleted\n", instance, role)
				case "key":
					fmt.Printf("Keys %v were deleted from instance '%s' of role '%s'\n",
						getKeyNames(keys), instance, role)
				}
			} else {
				fmt.Println("No changes were made")
			}
			return nil
		}

	default:
		fmt.Printf("Unknown method: %s\n", config.method)
		_ = cmd.Help()
		return fmt.Errorf("unknown method: %s", config.method)
	}
}

func validateFactCommand(args []string, config *factConfig) error {
	if len(args) < 1 {
		return fmt.Errorf("role name is required")
	}
	if err := validateFactIdentifier("role", args[0], true); err != nil {
		return err
	}
	if len(args) == 2 {
		if err := validateFactIdentifier("instance", args[1], false); err != nil {
			return err
		}
	}

	switch config.method {
	case "load":
		if config.deleteType != "" {
			return fmt.Errorf("--delete-type is only valid with --method=delete")
		}
	case "save":
		if len(args) != 2 {
			return fmt.Errorf("instance name is required for save method")
		}
		if config.deleteType != "" {
			return fmt.Errorf("--delete-type is only valid with --method=delete")
		}
		if len(config.keyValues) == 0 {
			return fmt.Errorf("at least one --key key=value is required for save method")
		}
		for _, keyValue := range config.keyValues {
			key, _, found := strings.Cut(keyValue, "=")
			if !found {
				return fmt.Errorf("invalid save key %q: expected key=value", keyValue)
			}
			if err := validateFactKey(key); err != nil {
				return err
			}
		}
	case "delete":
		switch config.deleteType {
		case "role":
			if len(args) != 1 {
				return fmt.Errorf("instance must not be provided for role deletion")
			}
			if len(config.keyValues) > 0 {
				return fmt.Errorf("--key is not valid for role deletion")
			}
		case "instance":
			if len(args) != 2 {
				return fmt.Errorf("instance name is required for instance deletion")
			}
			if len(config.keyValues) > 0 {
				return fmt.Errorf("--key is not valid for instance deletion")
			}
		case "key":
			if len(args) != 2 {
				return fmt.Errorf("instance name is required for key deletion")
			}
			if len(config.keyValues) == 0 {
				return fmt.Errorf("at least one --key is required for key deletion")
			}
			for _, keyValue := range config.keyValues {
				key, _, _ := strings.Cut(keyValue, "=")
				if err := validateFactKey(key); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid delete type %q: expected role, instance, or key", config.deleteType)
		}
	default:
		return fmt.Errorf("unknown method %q: expected load, save, or delete", config.method)
	}
	return nil
}

func validateFactIdentifier(kind, value string, fileName bool) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s name must not be empty", kind)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.ContainsAny(value, "[]") {
		return fmt.Errorf("%s name contains unsupported characters", kind)
	}
	if kind == "instance" && value == ini.DefaultSection {
		return fmt.Errorf("instance name must not be %q", ini.DefaultSection)
	}
	if fileName && (value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsAny(value, `/\`)) {
		return fmt.Errorf("role name must not contain a path")
	}
	return nil
}

func validateFactKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("key name must not be empty")
	}
	if strings.IndexFunc(key, unicode.IsControl) >= 0 || strings.Contains(key, "=") {
		return fmt.Errorf("key name %q contains unsupported characters", key)
	}
	trimmed := strings.TrimLeftFunc(key, unicode.IsSpace)
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return fmt.Errorf("key name %q must not be interpreted as a comment", key)
	}
	return nil
}

func sortStrings(items []string) {
	sort.Strings(items)
}

// getSortedKeys returns sorted keys from a map
func getSortedKeys(facts map[string]string) []string {
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

// displayFacts prints facts in a consistent sorted format
func displayFacts(facts map[string]string) {
	sortedKeys := getSortedKeys(facts)
	for _, key := range sortedKeys {
		fmt.Printf("  %s: %s\n", key, facts[key])
	}
}

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

// Get names of keys from a map
func getKeyNames(keys map[string]string) []string {
	keyNames := make([]string, 0, len(keys))
	for k := range keys {
		keyNames = append(keyNames, k)
	}
	sort.Strings(keyNames)
	return keyNames
}

// getFilePath returns the configuration file path for a role
func getFilePath(role string) string {
	return filepath.Join(layout.Current().SaltboxFactsPath, role+".ini")
}

// parseKeyValues parses key=value string slices into a map
func parseKeyValues(keyVals []string) map[string]string {
	result := make(map[string]string)
	for _, kv := range keyVals {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		} else if len(parts) == 1 {
			// For delete operations, we might just have the key name
			result[parts[0]] = ""
		}
	}
	return result
}

// loadAllInstances loads all instances and their facts from an ini file
func loadAllInstances(filePath string) (map[string]map[string]string, error) {
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
func loadFacts(filePath, instance string, defaults map[string]string) (map[string]string, error) {
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
func saveFacts(filePath, instance string, keys map[string]string, saltboxUser string) (map[string]string, bool, error) {
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
func deleteFacts(filePath, deleteType, instance string, keys map[string]string, saltboxUser string) (bool, error) {
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

func addFactCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newFactCommand())
}
