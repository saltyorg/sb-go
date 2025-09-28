package validate2

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"gopkg.in/yaml.v3"
)

// configValidationJob represents a single config file validation task
type configValidationJob struct {
	configPath string
	schemaPath string
	name       string
	optional   bool
}

// AllSaltboxConfigs validates all Saltbox configuration files using YAML schemas
func AllSaltboxConfigs(verbose bool) error {
	// Set verbose mode
	SetVerbose(verbose)

	// Define all validation jobs
	jobs := []configValidationJob{
		{
			configPath: constants.SaltboxAccountsPath,
			schemaPath: "/srv/git/saltbox/schema/accounts.schema.yml",
			name:       "accounts.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxAdvancedSettingsPath,
			schemaPath: "/srv/git/saltbox/schema/adv_settings.schema.yml",
			name:       "adv_settings.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxBackupConfigPath,
			schemaPath: "/srv/git/saltbox/schema/backup_config.schema.yml",
			name:       "backup_config.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxHetznerVLANPath,
			schemaPath: "/srv/git/saltbox/schema/hetzner_vlan.schema.yml",
			name:       "hetzner_vlan.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxSettingsPath,
			schemaPath: "/srv/git/saltbox/schema/settings.schema.yml",
			name:       "settings.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxMOTDPath,
			schemaPath: "/srv/git/saltbox/schema/motd.schema.yml",
			name:       "motd.yml",
			optional:   true,
		},
	}

	// Process each validation job
	for _, job := range jobs {
		if err := processValidationJob(job, verbose); err != nil {
			return err
		}
	}

	return nil
}

// processValidationJob handles validation of a single config file
func processValidationJob(job configValidationJob, verbose bool) error {
	// Check if config file exists
	if _, err := os.Stat(job.configPath); err != nil {
		if job.optional {
			if verbose {
				fmt.Printf("%s not found, skipping validation\n", job.name)
			}
			return nil
		}
		return fmt.Errorf("required config file not found: %s", job.configPath)
	}

	// Try to find schema file (first in current directory, then fallback)
	schemaPath := job.schemaPath
	if _, err := os.Stat(schemaPath); err != nil {
		// Fallback to old validation method if schema not found
		if verbose {
			fmt.Printf("Schema file %s not found, skipping YAML schema validation for %s\n", schemaPath, job.name)
		}
		return nil
	}

	// Perform validation with spinner or verbose output
	successMessage := fmt.Sprintf("Validated %s", job.name)
	failureMessage := fmt.Sprintf("Failed to validate %s", job.name)

	var validationError error
	if verbose {
		fmt.Printf("Validating %s...\n", job.name)
		validationError = validateConfigWithSchema(job.configPath, schemaPath)
		if validationError == nil {
			fmt.Println(successMessage)
		} else {
			fmt.Println(failureMessage)
		}
	} else {
		validationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", job.name),
			StopMessage:     successMessage,
			StopFailMessage: failureMessage,
		}, func() error {
			return validateConfigWithSchema(job.configPath, schemaPath)
		})
	}

	if validationError != nil {
		return fmt.Errorf("%s: %w", failureMessage, validationError)
	}

	return nil
}

// validateConfigWithSchema validates a config file using the original struct approach but with schema for structure checking
func validateConfigWithSchema(configPath, schemaPath string) error {
	debugPrintf("DEBUG: validateConfigWithSchema called with config=%s, schema=%s\n", configPath, schemaPath)

	// Load the config file
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file (%s): %w", configPath, err)
	}

	// Load into generic map for structure checking
	var inputMap map[string]interface{}
	if err := yaml.Unmarshal(configFile, &inputMap); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s): %w", configPath, err)
	}

	// Load the schema for additional validation if available
	schema, err := LoadSchema(schemaPath)
	if err != nil {
		// If schema loading fails, skip schema validation and rely on struct validation
		debugPrintf("DEBUG: Schema loading failed, skipping schema validation: %v\n", err)
	} else {
		// Perform full schema validation including type checking and custom validators
		if err := schema.Validate(inputMap); err != nil {
			return fmt.Errorf("schema validation failed: %w", err)
		}
	}

	// Determine config type and validate using existing struct validation
	configName := filepath.Base(configPath)
	switch configName {
	case "accounts.yml":
		return validateWithStruct(configFile, &config.Config{}, config.ValidateConfig, inputMap)
	case "adv_settings.yml":
		return validateWithStruct(configFile, &config.AdvSettingsConfig{}, config.ValidateAdvSettingsConfig, inputMap)
	case "backup_config.yml":
		return validateWithStruct(configFile, &config.BackupConfig{}, config.ValidateBackupConfig, inputMap)
	case "hetzner_vlan.yml":
		return validateWithStruct(configFile, &config.HetznerVLANConfig{}, config.ValidateHetznerVLANConfig, inputMap)
	case "settings.yml":
		return validateWithStruct(configFile, &config.SettingsConfig{}, config.ValidateSettingsConfig, inputMap)
	case "motd.yml":
		return validateWithStruct(configFile, &config.MOTDConfig{}, config.ValidateMOTDConfig, inputMap)
	default:
		return fmt.Errorf("unknown config file: %s", configName)
	}
}

// validateWithStruct uses the original struct-based validation approach
func validateWithStruct[T any](configFile []byte, configData *T, validateFn func(*T, map[string]interface{}) error, inputMap map[string]interface{}) error {
	// Unmarshal into the typed struct (this handles type conversion properly)
	if err := yaml.Unmarshal(configFile, configData); err != nil {
		// Try to provide better error context by analyzing the input map
		enhancedErr := enhanceUnmarshalError(err, inputMap)
		return fmt.Errorf("error unmarshaling config into struct: %w", enhancedErr)
	}

	// Use the original validation function
	if err := validateFn(configData, inputMap); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}

	return nil
}

// enhanceUnmarshalError attempts to provide better context for unmarshaling errors
func enhanceUnmarshalError(err error, inputMap map[string]interface{}) error {
	errStr := err.Error()

	// Look for "invalid Ansible boolean value" errors and try to find the field
	if strings.Contains(errStr, "invalid Ansible boolean value:") {
		// Extract the invalid value from the error
		parts := strings.Split(errStr, "invalid Ansible boolean value: ")
		if len(parts) > 1 {
			invalidValue := strings.TrimSpace(parts[1])

			// Search for this value in the input map to find the field
			if fieldPath := findFieldWithValue(inputMap, invalidValue, ""); fieldPath != "" {
				return fmt.Errorf("invalid Ansible boolean value '%s' in field '%s'. Valid values are: yes/no, true/false, on/off, 1/0", invalidValue, fieldPath)
			}
		}
	}

	// For other errors, just return the original
	return err
}

// findFieldWithValue recursively searches for a field containing the specified value
func findFieldWithValue(obj map[string]interface{}, targetValue, currentPath string) string {
	for key, value := range obj {
		fieldPath := key
		if currentPath != "" {
			fieldPath = currentPath + "." + key
		}

		// Check if this field has the target value
		if fmt.Sprintf("%v", value) == targetValue {
			return fieldPath
		}

		// Recursively search nested objects
		if nestedObj, ok := value.(map[string]interface{}); ok {
			if found := findFieldWithValue(nestedObj, targetValue, fieldPath); found != "" {
				return found
			}
		}

		// Search in arrays
		if arr, ok := value.([]interface{}); ok {
			for i, item := range arr {
				if fmt.Sprintf("%v", item) == targetValue {
					return fmt.Sprintf("%s[%d]", fieldPath, i)
				}
				if nestedObj, ok := item.(map[string]interface{}); ok {
					itemPath := fmt.Sprintf("%s[%d]", fieldPath, i)
					if found := findFieldWithValue(nestedObj, targetValue, itemPath); found != "" {
						return found
					}
				}
			}
		}
	}
	return ""
}