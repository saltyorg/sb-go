package cmd

import (
	"fmt"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/utils"

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
var factCmd = &cobra.Command{
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flag values and create config
		method, _ := cmd.Flags().GetString("method")
		deleteType, _ := cmd.Flags().GetString("delete-type")
		keyValues, _ := cmd.Flags().GetStringSlice("key")

		config := &factConfig{
			method:     method,
			deleteType: deleteType,
			keyValues:  keyValues,
		}

		return runFactCommand(cmd, args, config)
	},
}

// runFactCommand handles the main logic for the fact command
func runFactCommand(cmd *cobra.Command, args []string, config *factConfig) error {
	if len(args) < 1 {
		fmt.Print("Error: Role name is required\n\n")
		cmd.Help()
		return fmt.Errorf("role name is required")
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
			cmd.Help()
			return fmt.Errorf("instance name is required for save method")
		}
		instance := args[1]

		// Get the Saltbox user for owner/group
		saltboxUser, err := utils.GetSaltboxUser()
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
			return fmt.Errorf("delete-type is required for delete method")
		}

		// Get the Saltbox user for owner/group if needed for cleanup
		saltboxUser, err := utils.GetSaltboxUser()
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
				cmd.Help()
				return fmt.Errorf("instance name is required for instance or key deletion")
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
		cmd.Help()
		return fmt.Errorf("unknown method: %s", config.method)
	}
}

// sortStrings performs a simple bubble sort on a string slice
func sortStrings(items []string) {
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if items[i] > items[j] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
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

// setFileOwnershipAndPermissions sets the file ownership to saltboxUser and permissions to 0640
func setFileOwnershipAndPermissions(filePath, saltboxUser string) error {
	// Always set permissions to 0640
	if err := os.Chmod(filePath, 0640); err != nil {
		return fmt.Errorf("failed to set file permissions: %v", err)
	}

	// Set ownership to the Saltbox user
	passwd, err := user.Lookup(saltboxUser)
	if err == nil {
		uid, _ := strconv.Atoi(passwd.Uid)
		gid, _ := strconv.Atoi(passwd.Gid)
		if err := syscall.Chown(filePath, uid, gid); err != nil {
			// Just log the error but don't fail the operation
			fmt.Printf("Warning: Failed to set ownership to %s: %v\n", saltboxUser, err)
		}
	} else {
		fmt.Printf("Warning: Failed to lookup user %s: %v\n", saltboxUser, err)
	}

	return nil
}

// Get names of keys from a map
func getKeyNames(keys map[string]string) []string {
	keyNames := make([]string, 0, len(keys))
	for k := range keys {
		keyNames = append(keyNames, k)
	}
	return keyNames
}

// getFilePath returns the configuration file path for a role
func getFilePath(role string) string {
	return filepath.Join(constants.SaltboxFactsPath, role+".ini")
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

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return allInstances, nil // Return an empty map if the file doesn't exist
	}

	// Load the ini file
	cfg, err := ini.Load(filePath)
	if err != nil {
		return allInstances, fmt.Errorf("failed to load ini file: %v", err)
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

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return facts, nil // Return defaults if the file doesn't exist
	}

	// Load the ini file
	cfg, err := ini.Load(filePath)
	if err != nil {
		return facts, fmt.Errorf("failed to load ini file: %v", err)
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
	facts := make(map[string]string)
	changed := false

	// Create a new ini file config
	cfg := ini.Empty()

	// If a file exists, load it
	fileExists := false
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		fileExists = true
		if err := cfg.Append(filePath); err != nil {
			return facts, false, fmt.Errorf("failed to load existing ini file: %v", err)
		}
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
		// Create the directory if it doesn't exist
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return facts, false, fmt.Errorf("failed to create directory: %v", err)
		}

		// Save to file
		if err := cfg.SaveTo(filePath); err != nil {
			return facts, false, fmt.Errorf("failed to save ini file: %v", err)
		}

		// Set ownership and permissions
		if err := setFileOwnershipAndPermissions(filePath, saltboxUser); err != nil {
			return facts, true, err
		}
	}

	return facts, changed, nil
}

// deleteFacts deletes facts from an ini file
func deleteFacts(filePath, deleteType, instance string, keys map[string]string, saltboxUser string) (bool, error) {
	changed := false

	// For role deletion, just remove the file
	if deleteType == "role" {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return false, nil // File doesn't exist, no change
		}

		if err := os.Remove(filePath); err != nil {
			return false, fmt.Errorf("failed to delete file: %v", err)
		}

		return true, nil
	}

	// For instance or key deletion, we need to modify the file
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, nil // File doesn't exist, no change
	}

	// Load the ini file
	cfg, err := ini.Load(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to load ini file: %v", err)
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
		if err := cfg.SaveTo(filePath); err != nil {
			return false, fmt.Errorf("failed to save ini file: %v", err)
		}

		// Set ownership and permissions
		if err := setFileOwnershipAndPermissions(filePath, saltboxUser); err != nil {
			return true, err
		}
	}

	return changed, nil
}

func init() {
	rootCmd.AddCommand(factCmd)

	// Add flags
	factCmd.Flags().String("method", "load", "Method to use (load, save, delete)")
	factCmd.Flags().String("delete-type", "", "Type of deletion (role, instance, key)")
	factCmd.Flags().StringSlice("key", []string{}, "Key-value pairs (format: key=value)")
}
