package validate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/spinners"

	"gopkg.in/yaml.v3"
)

// validationFunc defines a function type that validates a config struct
type validationFunc[T any] func(config *T, input map[string]interface{}) error

// AllSaltboxConfigs validates all Saltbox configuration files
// verbose: Enable verbose output mode
// Returns an error if any validation fails, nil otherwise
func AllSaltboxConfigs(verbose bool) error {
	// Set verbose mode in the config package.
	config.SetVerbose(verbose)

	// --- Accounts Configuration Validation ---
	if err := validateConfigFileWithSpinner(
		constants.SaltboxAccountsPath,
		&config.Config{},
		config.ValidateConfig,
		verbose,
	); err != nil {
		return err
	}

	// --- Advanced Settings Validation ---
	if err := validateConfigFileWithSpinner(
		constants.SaltboxAdvancedSettingsPath,
		&config.AdvSettingsConfig{},
		config.ValidateAdvSettingsConfig,
		verbose,
	); err != nil {
		return err
	}

	// --- Backup Configuration Validation ---
	if err := validateConfigFileWithSpinner(
		constants.SaltboxBackupConfigPath,
		&config.BackupConfig{},
		config.ValidateBackupConfig,
		verbose,
	); err != nil {
		return err
	}

	// --- Hetzner VLAN Configuration Validation ---
	if err := validateConfigFileWithSpinner(
		constants.SaltboxHetznerVLANPath,
		&config.HetznerVLANConfig{},
		config.ValidateHetznerVLANConfig,
		verbose,
	); err != nil {
		return err
	}

	// --- Settings Configuration Validation ---
	if err := validateConfigFileWithSpinner(
		constants.SaltboxSettingsPath,
		&config.SettingsConfig{},
		config.ValidateSettingsConfig,
		verbose,
	); err != nil {
		return err
	}

	// --- MOTD Configuration Validation ---
	motdConfigPath := constants.SaltboxMOTDPath

	// Skip this validation if the file doesn't exist yet
	if _, err := os.Stat(motdConfigPath); err == nil {
		if err := validateConfigFileWithSpinner(
			motdConfigPath,
			&config.MOTDConfig{},
			config.ValidateMOTDConfig,
			verbose,
		); err != nil {
			return err
		}
	} else if verbose {
		fmt.Printf("MOTD config file not found at %s, skipping validation\n", motdConfigPath)
	}

	return nil
}

// validateConfigFile reads and validates a config file against its schema
func validateConfigFile[T any](filePath string, configData *T, validateFn validationFunc[T]) error {
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading config file (%s): %w", filePath, err)
	}

	var inputMap map[string]interface{}
	if err := yaml.Unmarshal(configFile, &inputMap); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s): %w", filePath, err)
	}

	if err := yaml.Unmarshal(configFile, configData); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s) into struct: %w", filePath, err)
	}

	if err := validateFn(configData, inputMap); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}
	return nil
}

// validateConfigFileWithSpinner validates a config file with or without a spinner based on verbose mode
func validateConfigFileWithSpinner[T any](
	filePath string,
	configData *T,
	validateFn validationFunc[T],
	verbose bool,
) error {
	filename := filepath.Base(filePath)
	successMessage := fmt.Sprintf("Validated %s", filename)
	failureMessage := fmt.Sprintf("Failed to validate %s", filename)

	var validationError error
	if verbose {
		fmt.Printf("Validating %s...\n", filename)
		validationError = validateConfigFile(filePath, configData, validateFn)
		if validationError == nil {
			fmt.Println(successMessage)
		} else {
			fmt.Println(failureMessage)
		}
	} else {
		validationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", filename),
			StopMessage:     successMessage,
			StopFailMessage: failureMessage,
		}, func() error {
			return validateConfigFile(filePath, configData, validateFn)
		})
	}

	if validationError != nil {
		return fmt.Errorf("%s: %w", failureMessage, validationError)
	}

	return nil
}
