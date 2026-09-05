package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/saltyorg/sb-go/facts"
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

// Temporary adapters preserve the old command until the tree editor is wired.
func loadAllInstances(path string) (map[string]map[string]string, error) {
	return facts.LegacyLoadAll(path)
}
func loadFacts(path, instance string, defaults map[string]string) (map[string]string, error) {
	return facts.LegacyLoad(path, instance, defaults)
}
func saveFacts(path, instance string, keys map[string]string, user string) (map[string]string, bool, error) {
	return facts.LegacySave(path, instance, keys, user)
}
func deleteFacts(path, kind, instance string, keys map[string]string, user string) (bool, error) {
	return facts.LegacyDelete(path, kind, instance, keys, user)
}

func addFactCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newFactCommand())
}
