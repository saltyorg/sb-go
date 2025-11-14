package validate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/logging"
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
			configPath: constants.SaltboxAccountsConfigPath,
			schemaPath: "/srv/git/saltbox/schema/accounts.schema.yml",
			name:       "accounts.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxAdvancedSettingsConfigPath,
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
			configPath: constants.SaltboxHetznerVLANConfigPath,
			schemaPath: "/srv/git/saltbox/schema/hetzner_vlan.schema.yml",
			name:       "hetzner_vlan.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxSettingsConfigPath,
			schemaPath: "/srv/git/saltbox/schema/settings.schema.yml",
			name:       "settings.yml",
			optional:   false,
		},
		{
			configPath: constants.SaltboxMOTDConfigPath,
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
				fmt.Printf("%s not found, skipping validation", job.name)
			}
			return nil
		}
		return fmt.Errorf("required config file not found: %s", job.configPath)
	}

	// Check if schema file exists
	schemaPath := job.schemaPath
	if _, err := os.Stat(schemaPath); err != nil {
		return fmt.Errorf("schema file not found: %s", schemaPath)
	}

	// Perform validation with spinner or verbose output
	successMessage := fmt.Sprintf("Validated %s", job.name)
	failureMessage := fmt.Sprintf("Failed to validate %s", job.name)

	var validationError error
	if verbose {
		fmt.Printf("Validating %s...", job.name)
		validationError = validateConfigWithSchema(job.configPath, schemaPath)
		if validationError == nil {
			fmt.Println(successMessage)
		} else {
			fmt.Println(failureMessage)
		}
	} else {
		// Note: Using context.Background() here - consider adding context parameter in future refactor
		validationError = spinners.RunTaskWithSpinnerCustomContext(context.Background(), spinners.SpinnerOptions{
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

// validateConfigWithSchema validates a config file against its YAML schema
func validateConfigWithSchema(configPath, schemaPath string) error {
	startTime := time.Now()
	logging.DebugBool(verboseMode, "validateConfigWithSchema called with config=%s, schema=%s at %v", configPath, schemaPath, startTime)

	// Load the config file
	configFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file (%s): %w", configPath, err)
	}

	// Load into generic map for structure checking
	var inputMap map[string]any
	if err := yaml.Unmarshal(configFile, &inputMap); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s): %w", configPath, err)
	}

	// Load the schema for schema-based validation
	schema, err := LoadSchema(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to load schema file %s: %w", schemaPath, err)
	}

	// Perform schema validation with async API checks
	syncErr, asyncCtx := schema.ValidateWithTypeFlexibilityAsync(inputMap)
	if syncErr != nil {
		return fmt.Errorf("schema validation failed: %w", syncErr)
	}

	syncDuration := time.Since(startTime)
	logging.DebugBool(verboseMode, "Synchronous schema validation completed successfully in %v", syncDuration)

	// Wait for async API validations to complete
	if asyncCtx != nil {
		asyncStartTime := time.Now()
		logging.DebugBool(verboseMode, "Waiting for async API validations to complete")

		// TODO: In the future, we could show progress here like:
		// - "Validating Cloudflare API credentials..."
		// - "Validating Docker Hub credentials..."
		// For now, just wait for completion

		apiErrors := asyncCtx.Wait()
		if len(apiErrors) > 0 {
			// Combine all API validation errors
			var errorMsg strings.Builder
			errorMsg.WriteString("API validation failed:")
			for _, apiErr := range apiErrors {
				errorMsg.WriteString(fmt.Sprintf("\n  - %v", apiErr))
			}
			// Fixed: Use %s format specifier to prevent format string vulnerability
			return fmt.Errorf("%s", errorMsg.String())
		}
		asyncDuration := time.Since(asyncStartTime)
		logging.DebugBool(verboseMode, "Async API validations completed successfully in %v", asyncDuration)
	}

	duration := time.Since(startTime)
	logging.DebugBool(verboseMode, "validateConfigWithSchema completed for %s in %v", configPath, duration)
	return nil
}
