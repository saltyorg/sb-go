package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/go-playground/validator/v10"
)

// SettingsConfig represents the settings configuration.
type SettingsConfig struct {
	Authelia   AutheliaConfig `yaml:"authelia"`
	Downloads  string         `yaml:"downloads" validate:"required,dirpath"`
	Rclone     RcloneSettings `yaml:"rclone"`
	Shell      string         `yaml:"shell" validate:"required"`
	Transcodes string         `yaml:"transcodes" validate:"required,dirpath"`
}

// AutheliaConfig holds Authelia-related settings.
type AutheliaConfig struct {
	Master    AnsibleBool `yaml:"master" validate:"required,ansiblebool"`
	Subdomain string      `yaml:"subdomain" validate:"required"`
}

// RcloneSettings holds rclone settings.
type RcloneSettings struct {
	Enabled AnsibleBool    `yaml:"enabled" validate:"required,ansiblebool"`
	Remotes []RemoteConfig `yaml:"remotes" validate:"required"`
	Version string         `yaml:"version" validate:"required"`
}

// RemoteConfig holds settings for a single rclone remote.
type RemoteConfig struct {
	Remote   string         `yaml:"remote" validate:"required"`
	Settings RemoteSettings `yaml:"settings" validate:"required"`
}

// RemoteSettings holds the advanced settings for a single rclone remote.
type RemoteSettings struct {
	EnableRefresh AnsibleBool    `yaml:"enable_refresh" validate:"required,ansiblebool"`
	Mount         AnsibleBool    `yaml:"mount" validate:"required,ansiblebool"`
	Template      string         `yaml:"template" validate:"required,rclone_template"`
	Union         AnsibleBool    `yaml:"union" validate:"required,ansiblebool"`
	Upload        AnsibleBool    `yaml:"upload" validate:"required,ansiblebool"`
	UploadFrom    string         `yaml:"upload_from" validate:"required,dirpath"`
	VFSCache      VFSCacheConfig `yaml:"vfs_cache" validate:"required"`
}

// VFSCacheConfig holds VFS cache settings.
type VFSCacheConfig struct {
	Enabled AnsibleBool `yaml:"enabled" validate:"required,ansiblebool"`
	MaxAge  string      `yaml:"max_age" validate:"required"`
	Size    string      `yaml:"size" validate:"required"`
}

// rcloneTemplateValidator is the custom validator for rclone templates.
func rcloneTemplateValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	logging.DebugBool(verboseMode, "rcloneTemplateValidator called with value: '%s'", value)

	// Check for predefined values.
	switch strings.ToLower(value) {
	case "dropbox", "google", "sftp", "nfs":
		logging.DebugBool(verboseMode, "rcloneTemplateValidator - value is a predefined type, returning true")
		return true
	}

	// Check for absolute path and file existence.
	if strings.HasPrefix(value, "/") {
		logging.DebugBool(verboseMode, "rcloneTemplateValidator - value is an absolute path, checking file existence")
		_, err := os.Stat(value)
		isValid := err == nil // Valid if the file exists
		logging.DebugBool(verboseMode, "rcloneTemplateValidator - file exists: %t, returning %t", isValid, isValid)
		return isValid
	}

	logging.DebugBool(verboseMode, "rcloneTemplateValidator - value is not a predefined type or absolute path, returning false")
	return false
}

// ValidateSettingsConfig validates the SettingsConfig struct.
func ValidateSettingsConfig(config *SettingsConfig, inputMap map[string]any) error {
	logging.DebugBool(verboseMode, "\nDEBUG: ValidateSettingsConfig called with config: %+v, inputMap: %+v", config, inputMap)
	validate := validator.New()
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - registering custom validators")
	if err := RegisterCustomValidators(validate); err != nil {
		return err
	}

	logging.DebugBool(verboseMode, "ValidateSettingsConfig - registering dirpath validator")
	err := validate.RegisterValidation("dirpath", dirPathValidator) // Register the dirpath validator
	if err != nil {
		err := fmt.Errorf("failed to register dirpath validator: %w", err)
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - registering rclone_template validator")
	err = validate.RegisterValidation("rclone_template", rcloneTemplateValidator)
	if err != nil {
		err := fmt.Errorf("failed to register rclone template validator: %w", err)
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - %v", err)
		return err
	}
	// Validate the struct.
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - validating struct: %+v", config)
	if err := validate.Struct(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - struct validation error: %v", err)
		return formatValidationError(err, config) // Pass the config to help with remote identification
	}

	// Check for extra fields
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - checking for extra fields")
	if err := checkExtraFields(inputMap, config); err != nil {
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - checkExtraFields returned error: %v", err)
		return err
	}

	// Check if rclone is enabled by converting AnsibleBool to lowercase string
	rcloneEnabledValue := strings.ToLower(string(config.Rclone.Enabled))
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - rclone.enabled = '%s'", rcloneEnabledValue)

	// Determine if rclone is enabled based on the value
	rcloneEnabled := false
	switch rcloneEnabledValue {
	case "yes", "true", "on", "1":
		rcloneEnabled = true
	}

	// Now, validate nested structs explicitly.
	logging.DebugBool(verboseMode, "ValidateSettingsConfig - validating nested structs")
	for i, remote := range config.Rclone.Remotes {
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - validating remote: %+v", remote)
		if err := validate.Struct(remote); err != nil {
			logging.DebugBool(verboseMode, "ValidateSettingsConfig - remote validation error: %v", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - validating remote.Settings: %+v", remote.Settings)
		if err := validate.Struct(remote.Settings); err != nil {
			logging.DebugBool(verboseMode, "ValidateSettingsConfig - remote.Settings validation error: %v", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}
		logging.DebugBool(verboseMode, "ValidateSettingsConfig - validating remote.Settings.VFSCache: %+v", remote.Settings.VFSCache)
		if err := validate.Struct(remote.Settings.VFSCache); err != nil {
			logging.DebugBool(verboseMode, "ValidateSettingsConfig - remote.Settings.VFSCache validation error: %v", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}

		// Only validate remote existence if rclone is enabled
		if rcloneEnabled {
			// Additional validation for rclone remote existence (except for NFS).
			if strings.ToLower(remote.Settings.Template) != "nfs" {
				logging.DebugBool(verboseMode, "ValidateSettingsConfig - template is not NFS, validating rclone remote existence")
				// Split the remote string into name and path.
				parts := strings.SplitN(remote.Remote, ":", 2)
				remoteName := remote.Remote
				if len(parts) == 2 {
					remoteName = parts[0]
					logging.DebugBool(verboseMode, "ValidateSettingsConfig - remote is in 'remote:path' format, remoteName: '%s'", remoteName)
				} else {
					logging.DebugBool(verboseMode, "ValidateSettingsConfig - remote is a bare name: '%s'", remote.Remote)
				}
				logging.DebugBool(verboseMode, "ValidateSettingsConfig - remoteName: '%s'", remoteName)

				if err := validateRcloneRemote(remoteName); err != nil {
					logging.DebugBool(verboseMode, "ValidateSettingsConfig - validateRcloneRemote returned error: %v", err)
					//Only return if rclone and the user and the config all exist
					if errors.Is(err, ErrRcloneNotInstalled) || errors.Is(err, ErrSystemUserNotFound) || errors.Is(err, ErrRcloneConfigNotFound) {
						fmt.Printf("Warning: rclone remote validation skipped: %v", err)
					} else {
						return err
					}
				} else {
					logging.DebugBool(verboseMode, "ValidateSettingsConfig - validateRcloneRemote successful")
				}
			} else {
				logging.DebugBool(verboseMode, "ValidateSettingsConfig - template is NFS, skipping rclone remote existence validation")
			}
		} else {
			logging.DebugBool(verboseMode, "ValidateSettingsConfig - rclone is disabled, skipping remote existence validation")
		}
	}

	logging.DebugBool(verboseMode, "ValidateSettingsConfig - validation successful")
	return nil
}

// formatValidationError formats validation errors for better readability.
func formatValidationError(err error, config *SettingsConfig) error {
	logging.DebugBool(verboseMode, "formatValidationError called with error: %v", err)
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var sb strings.Builder

		// Group errors by remote
		remoteErrors := make(map[string][]string)
		var generalErrors []string

		for _, e := range validationErrors {
			// Get the full path to the field based on the namespace
			fieldPath := e.Namespace()

			logging.DebugBool(verboseMode, "formatValidationError - validation error on field '%s', tag '%s', value '%v', param '%s'",
				fieldPath, e.Tag(), e.Value(), e.Param())

			// Check if this is a remote-related error
			remoteMatch := regexp.MustCompile(`Rclone\.Remotes\[(\d+)]`).FindStringSubmatch(fieldPath)

			// Convert struct field names to YAML field names for better user understanding
			yamlFieldPath := convertToYAMLFieldPath(fieldPath)

			var errorMsg string
			switch e.Tag() {
			case "required":
				errorMsg = fmt.Sprintf("field '%s' is required", yamlFieldPath)
			case "ansiblebool":
				errorMsg = fmt.Sprintf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s",
					yamlFieldPath, e.Value())
			case "dirpath":
				errorMsg = fmt.Sprintf("field '%s' must be a valid directory path, got: %s",
					yamlFieldPath, e.Value())
			case "rclone_template":
				errorMsg = fmt.Sprintf("field '%s' must be one of 'dropbox', 'google', 'sftp', 'nfs', or a valid absolute file path, got: %s",
					yamlFieldPath, e.Value())
			default:
				errorMsg = fmt.Sprintf("field '%s' is invalid: %s", yamlFieldPath, e.Error())
			}

			// If this is a remote-specific error, add it to the appropriate group
			if len(remoteMatch) > 1 {
				remoteIndex := remoteMatch[1]
				// Try to get a human-readable identifier for this remote
				remoteIdentifier := fmt.Sprintf("remote #%s", remoteIndex)

				// If we have access to the config struct, try to get the remote name
				if config != nil && len(config.Rclone.Remotes) > 0 {
					idx, _ := strconv.Atoi(remoteIndex)
					if idx < len(config.Rclone.Remotes) {
						remoteIdentifier = fmt.Sprintf("remote '%s'", config.Rclone.Remotes[idx].Remote)
					}
				}

				remoteErrors[remoteIdentifier] = append(remoteErrors[remoteIdentifier], errorMsg)
			} else {
				generalErrors = append(generalErrors, errorMsg)
			}
		}

		// Write general errors first
		for _, errMsg := range generalErrors {
			sb.WriteString(errMsg + "")
		}

		// Write remote-specific errors grouped by remote
		if len(remoteErrors) > 0 {
			for remoteIdentifier, singleRemoteErrors := range remoteErrors {
				sb.WriteString(fmt.Sprintf("\nErrors for %s:", remoteIdentifier))

				for _, errMsg := range singleRemoteErrors {
					sb.WriteString("  - " + errMsg + "")
				}
			}
		}

		// Fixed: Use %s format specifier to prevent format string vulnerability
		formattedError := fmt.Errorf("%s", sb.String())
		logging.DebugBool(verboseMode, "formatValidationError - formatted error: %v", formattedError)
		return formattedError
	}
	logging.DebugBool(verboseMode, "formatValidationError - error is not a validation error, returning original error")
	return err // Return the original error if it's not a validator.ValidationErrors
}

// formatRemoteValidationError formats validation errors specifically for remote validation
func formatRemoteValidationError(err error, remoteName string, remoteIndex int) error {
	logging.DebugBool(verboseMode, "formatRemoteValidationError called with error: %v, remoteName: %s, remoteIndex: %d",
		err, remoteName, remoteIndex)

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var sb strings.Builder

		sb.WriteString(fmt.Sprintf("Errors for remote '%s':", remoteName))

		for _, e := range validationErrors {
			fieldPath := e.Namespace()
			logging.DebugBool(verboseMode, "formatRemoteValidationError - validation error on field '%s', tag '%s', value '%v'",
				fieldPath, e.Tag(), e.Value())

			// Convert the struct field path to a meaningful YAML path
			yamlFieldPath := convertToYAMLFieldPath(fieldPath)

			var errorMsg string
			switch e.Tag() {
			case "required":
				errorMsg = fmt.Sprintf("field '%s' is required", yamlFieldPath)
			case "ansiblebool":
				errorMsg = fmt.Sprintf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s",
					yamlFieldPath, e.Value())
			case "dirpath":
				errorMsg = fmt.Sprintf("field '%s' must be a valid directory path, got: %s",
					yamlFieldPath, e.Value())
			case "rclone_template":
				errorMsg = fmt.Sprintf("field '%s' must be one of 'dropbox', 'google', 'sftp', 'nfs', or a valid absolute file path, got: %s",
					yamlFieldPath, e.Value())
			default:
				errorMsg = fmt.Sprintf("field '%s' is invalid: %s", yamlFieldPath, e.Error())
			}

			sb.WriteString("  - " + errorMsg + "")
		}

		// Fixed: Use %s format specifier to prevent format string vulnerability
		formattedError := fmt.Errorf("%s", sb.String())
		logging.DebugBool(verboseMode, "formatRemoteValidationError - formatted error: %v", formattedError)
		return formattedError
	}

	return err
}

// convertToYAMLFieldPath converts internal struct field names to their YAML equivalents
func convertToYAMLFieldPath(fieldPath string) string {
	// First, handle rclone.remotes path pattern
	remotePathPattern := regexp.MustCompile(`Rclone\.Remotes\[(\d+)]`)
	fieldPath = remotePathPattern.ReplaceAllString(fieldPath, "rclone.remotes[$1]")

	// Extract the actual YAML path structure from the field path
	// This preserves the hierarchical structure while converting to YAML names

	// Convert camelCase to snake_case
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	fieldPath = re.ReplaceAllString(fieldPath, "${1}_${2}")
	fieldPath = strings.ToLower(fieldPath)

	// Remove struct names from the path
	fieldPath = strings.Replace(fieldPath, "settings_config.", "", 1)
	fieldPath = strings.Replace(fieldPath, "remote_config.", "", 1)
	fieldPath = strings.Replace(fieldPath, "remote_settings.", "", 1)
	fieldPath = strings.Replace(fieldPath, "vfs_cache_config.", "vfs_cache.", 1)
	fieldPath = strings.Replace(fieldPath, "rclone_settings.", "rclone.", 1)
	fieldPath = strings.Replace(fieldPath, "authelia_config.", "authelia.", 1)

	return fieldPath
}

// custom validator for directory paths
func dirPathValidator(fl validator.FieldLevel) bool {
	dirPath := fl.Field().String()
	logging.DebugBool(verboseMode, "dirPathValidator called with dirPath: '%s'", dirPath)

	// Check if the path is absolute or relative
	if !filepath.IsAbs(dirPath) {
		logging.DebugBool(verboseMode, "dirPathValidator - path is relative, making it absolute")
		// If relative, make it absolute based on the current working directory
		wd, err := os.Getwd()
		if err != nil {
			logging.DebugBool(verboseMode, "dirPathValidator - error getting working directory: %v, returning false", err)
			return false // If we can't determine the working dir, consider it invalid
		}
		dirPath = filepath.Join(wd, dirPath)
		logging.DebugBool(verboseMode, "dirPathValidator - absolute path: '%s'", dirPath)
	}
	// We don't check if the dir exists, only that the path is valid

	// Regular expression to check if it's a valid path (allows any characters)
	// Simplified regex, no need to check for starting slash separately.
	match, _ := regexp.MatchString(`(?:[^/]*/)*[^/]*$`, dirPath)
	logging.DebugBool(verboseMode, "dirPathValidator - path match regex: %t, returning %t", match, match)
	return match
}

// Define custom errors for specific conditions.
var (
	ErrRcloneNotInstalled   = errors.New("rclone is not installed")
	ErrRcloneConfigNotFound = errors.New("rclone config file not found")
	ErrSystemUserNotFound   = errors.New("system user not found")
)

// validateRcloneRemote checks if the given rclone remote exists.
func validateRcloneRemote(remoteName string) error {
	logging.DebugBool(verboseMode, "validateRcloneRemote called with remoteName: '%s'", remoteName)
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
	if err != nil {
		err := fmt.Errorf("%w: %v", ErrRcloneNotInstalled, err)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "validateRcloneRemote - rclone is installed")
	// Get the Saltbox user.
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: could not retrieve saltbox user: %v", err)
		logging.DebugBool(verboseMode, "validateRcloneRemote - error getting Saltbox user: %v", err)
		return ErrSystemUserNotFound
	}
	logging.DebugBool(verboseMode, "validateRcloneRemote - Saltbox user: '%s'", rcloneUser)

	// Check if the user exists on the system.
	_, err = user.Lookup(rcloneUser)
	if err != nil {
		logging.DebugBool(verboseMode, "validateRcloneRemote - error looking up user")
		var unknownUserError user.UnknownUserError
		if errors.As(err, &unknownUserError) {
			err := fmt.Errorf("%w: user '%s' does not exist", ErrSystemUserNotFound, rcloneUser)
			logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
			return err
		}
		// Some other error occurred during user lookup.
		err := fmt.Errorf("error looking up user '%s': %w", rcloneUser, err)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "validateRcloneRemote - user exists")

	// Define the rclone config path (standard location).
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	logging.DebugBool(verboseMode, "validateRcloneRemote - rcloneConfigPath: '%s'", rcloneConfigPath)

	// Check if the rclone config file exists
	_, err = os.Stat(rcloneConfigPath)
	if os.IsNotExist(err) {
		err := fmt.Errorf("%w: %v", ErrRcloneConfigNotFound, err)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "validateRcloneRemote - rclone config file exists")

	// Use context with timeout for external command execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "sudo",
		executor.WithArgs("-u", rcloneUser, "rclone", "config", "show"),
		executor.WithInheritEnv(fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath)),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		err := fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, result.Combined)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}
	output := result.Combined
	logging.DebugBool(verboseMode, "validateRcloneRemote - rclone config show output: '%s'", string(output))

	// Use a regular expression to search for the remote within the rclone config show output.
	remoteRegex := fmt.Sprintf(`(?m)^\[%s\]$`, regexp.QuoteMeta(remoteName))
	re, err := regexp.Compile(remoteRegex)
	if err != nil {
		err := fmt.Errorf("failed to compile regex for remote name: %w", err)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "validateRcloneRemote - remoteRegex: '%s'", remoteRegex)

	if !re.MatchString(string(output)) {
		err := fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
		logging.DebugBool(verboseMode, "validateRcloneRemote - %v", err)
		return err
	}

	logging.DebugBool(verboseMode, "validateRcloneRemote - rclone remote exists")
	return nil
}
