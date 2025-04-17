package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/saltyorg/sb-go/utils"
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
	Remote   string         `yaml:"remote" validate:"required"` // This is now "remote:path"
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
	debugPrintf("DEBUG: rcloneTemplateValidator called with value: '%s'\n", value)

	// Check for predefined values.
	switch strings.ToLower(value) {
	case "dropbox", "google", "sftp", "nfs":
		debugPrintf("DEBUG: rcloneTemplateValidator - value is a predefined type, returning true\n")
		return true
	}

	// Check for absolute path and file existence.
	if strings.HasPrefix(value, "/") {
		debugPrintf("DEBUG: rcloneTemplateValidator - value is an absolute path, checking file existence\n")
		_, err := os.Stat(value)
		isValid := err == nil // Valid if the file exists
		debugPrintf("DEBUG: rcloneTemplateValidator - file exists: %t, returning %t\n", isValid, isValid)
		return isValid
	}

	debugPrintf("DEBUG: rcloneTemplateValidator - value is not a predefined type or absolute path, returning false\n")
	return false
}

// ValidateSettingsConfig validates the SettingsConfig struct.
func ValidateSettingsConfig(config *SettingsConfig, inputMap map[string]interface{}) error {
	debugPrintf("\nDEBUG: ValidateSettingsConfig called with config: %+v, inputMap: %+v\n", config, inputMap)
	validate := validator.New()
	debugPrintf("DEBUG: ValidateSettingsConfig - registering custom validators\n")
	RegisterCustomValidators(validate) //From generic.go

	debugPrintf("DEBUG: ValidateSettingsConfig - registering dirpath validator\n")
	err := validate.RegisterValidation("dirpath", dirPathValidator) // Register the dirpath validator
	if err != nil {
		err := fmt.Errorf("failed to register dirpath validator: %w", err)
		debugPrintf("DEBUG: ValidateSettingsConfig - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: ValidateSettingsConfig - registering rclone_template validator\n")
	err = validate.RegisterValidation("rclone_template", rcloneTemplateValidator)
	if err != nil {
		err := fmt.Errorf("failed to register rclone template validator: %w", err)
		debugPrintf("DEBUG: ValidateSettingsConfig - %v\n", err)
		return err
	}
	// Validate the struct.
	debugPrintf("DEBUG: ValidateSettingsConfig - validating struct: %+v\n", config)
	if err := validate.Struct(config); err != nil {
		debugPrintf("DEBUG: ValidateSettingsConfig - struct validation error: %v\n", err)
		return formatValidationError(err, config) // Pass the config to help with remote identification
	}

	// Check for extra fields
	debugPrintf("DEBUG: ValidateSettingsConfig - checking for extra fields\n")
	if err := checkExtraFields(inputMap, config); err != nil {
		debugPrintf("DEBUG: ValidateSettingsConfig - checkExtraFields returned error: %v\n", err)
		return err
	}

	// Now, validate nested structs explicitly.
	debugPrintf("DEBUG: ValidateSettingsConfig - validating nested structs\n")
	for i, remote := range config.Rclone.Remotes {
		debugPrintf("DEBUG: ValidateSettingsConfig - validating remote: %+v\n", remote)
		if err := validate.Struct(remote); err != nil {
			debugPrintf("DEBUG: ValidateSettingsConfig - remote validation error: %v\n", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}
		debugPrintf("DEBUG: ValidateSettingsConfig - validating remote.Settings: %+v\n", remote.Settings)
		if err := validate.Struct(remote.Settings); err != nil {
			debugPrintf("DEBUG: ValidateSettingsConfig - remote.Settings validation error: %v\n", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}
		debugPrintf("DEBUG: ValidateSettingsConfig - validating remote.Settings.VFSCache: %+v\n", remote.Settings.VFSCache)
		if err := validate.Struct(remote.Settings.VFSCache); err != nil {
			debugPrintf("DEBUG: ValidateSettingsConfig - remote.Settings.VFSCache validation error: %v\n", err)
			return formatRemoteValidationError(err, remote.Remote, i)
		}

		// Additional validation for rclone remote existence (except for NFS).
		if strings.ToLower(remote.Settings.Template) != "nfs" {
			debugPrintf("DEBUG: ValidateSettingsConfig - template is not NFS, validating rclone remote existence\n")
			// Split the remote string into name and path.
			parts := strings.SplitN(remote.Remote, ":", 2)
			remoteName := remote.Remote
			if len(parts) == 2 {
				remoteName = parts[0]
				debugPrintf("DEBUG: ValidateSettingsConfig - remote is in 'remote:path' format, remoteName: '%s'\n", remoteName)
			} else {
				debugPrintf("DEBUG: ValidateSettingsConfig - remote is a bare name: '%s'\n", remote.Remote)
			}
			debugPrintf("DEBUG: ValidateSettingsConfig - remoteName: '%s'\n", remoteName)

			if err := validateRcloneRemote(remoteName); err != nil {
				debugPrintf("DEBUG: ValidateSettingsConfig - validateRcloneRemote returned error: %v\n", err)
				//Only return if rclone and the user and the config all exist
				if errors.Is(err, ErrRcloneNotInstalled) || errors.Is(err, ErrSystemUserNotFound) || errors.Is(err, ErrRcloneConfigNotFound) {
					fmt.Printf("Warning: rclone remote validation skipped: %v\n", err)
				} else {
					return err
				}
			} else {
				debugPrintf("DEBUG: ValidateSettingsConfig - validateRcloneRemote successful\n")
			}
		} else {
			debugPrintf("DEBUG: ValidateSettingsConfig - template is NFS, skipping rclone remote existence validation\n")
		}
	}

	debugPrintf("DEBUG: ValidateSettingsConfig - validation successful\n")
	return nil
}

// formatValidationError formats validation errors for better readability.
func formatValidationError(err error, config *SettingsConfig) error {
	debugPrintf("DEBUG: formatValidationError called with error: %v\n", err)
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var sb strings.Builder

		// Group errors by remote
		remoteErrors := make(map[string][]string)
		var generalErrors []string

		for _, e := range validationErrors {
			// Get the full path to the field based on the namespace
			fieldPath := e.Namespace()

			debugPrintf("DEBUG: formatValidationError - validation error on field '%s', tag '%s', value '%v', param '%s'\n",
				fieldPath, e.Tag(), e.Value(), e.Param())

			// Check if this is a remote-related error
			remoteMatch := regexp.MustCompile(`Rclone\.Remotes\[(\d+)\]`).FindStringSubmatch(fieldPath)

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
			sb.WriteString(errMsg + "\n")
		}

		// Write remote-specific errors grouped by remote
		if len(remoteErrors) > 0 {
			for remoteIdentifier, errors := range remoteErrors {
				sb.WriteString(fmt.Sprintf("\nErrors for %s:\n", remoteIdentifier))

				for _, errMsg := range errors {
					sb.WriteString("  - " + errMsg + "\n")
				}
			}
		}

		formattedError := fmt.Errorf(sb.String())
		debugPrintf("DEBUG: formatValidationError - formatted error: %v\n", formattedError)
		return formattedError
	}
	debugPrintf("DEBUG: formatValidationError - error is not a validation error, returning original error\n")
	return err // Return the original error if it's not a validator.ValidationErrors
}

// formatRemoteValidationError formats validation errors specifically for remote validation
func formatRemoteValidationError(err error, remoteName string, remoteIndex int) error {
	debugPrintf("DEBUG: formatRemoteValidationError called with error: %v, remoteName: %s, remoteIndex: %d\n",
		err, remoteName, remoteIndex)

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var sb strings.Builder

		sb.WriteString(fmt.Sprintf("Errors for remote '%s':\n", remoteName))

		for _, e := range validationErrors {
			fieldPath := e.Namespace()
			debugPrintf("DEBUG: formatRemoteValidationError - validation error on field '%s', tag '%s', value '%v'\n",
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

			sb.WriteString("  - " + errorMsg + "\n")
		}

		formattedError := fmt.Errorf(sb.String())
		debugPrintf("DEBUG: formatRemoteValidationError - formatted error: %v\n", formattedError)
		return formattedError
	}

	return err
}

// convertToYAMLFieldPath converts internal struct field names to their YAML equivalents
func convertToYAMLFieldPath(fieldPath string) string {
	// First, handle rclone.remotes path pattern
	remotePathPattern := regexp.MustCompile(`Rclone\.Remotes\[(\d+)\]`)
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
	debugPrintf("DEBUG: dirPathValidator called with dirPath: '%s'\n", dirPath)

	// Check if the path is absolute or relative
	if !filepath.IsAbs(dirPath) {
		debugPrintf("DEBUG: dirPathValidator - path is relative, making it absolute\n")
		// If relative, make it absolute based on the current working directory
		wd, err := os.Getwd()
		if err != nil {
			debugPrintf("DEBUG: dirPathValidator - error getting working directory: %v, returning false\n", err)
			return false // If we can't determine the working dir, consider it invalid
		}
		dirPath = filepath.Join(wd, dirPath)
		debugPrintf("DEBUG: dirPathValidator - absolute path: '%s'\n", dirPath)
	}
	// We don't check if the dir exists, only that the path is valid

	// Regular expression to check if it's a valid path (allows any characters)
	// Simplified regex, no need to check for starting slash separately.
	match, _ := regexp.MatchString(`(?:[^/]*/)*[^/]*$`, dirPath)
	debugPrintf("DEBUG: dirPathValidator - path match regex: %t, returning %t\n", match, match)
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
	debugPrintf("DEBUG: validateRcloneRemote called with remoteName: '%s'\n", remoteName)
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
	if err != nil {
		err := fmt.Errorf("%w: %v", ErrRcloneNotInstalled, err)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateRcloneRemote - rclone is installed\n")
	// Get the Saltbox user.
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: could not retrieve saltbox user: %v\n", err)
		debugPrintf("DEBUG: validateRcloneRemote - error getting Saltbox user: %v\n", err)
		return ErrSystemUserNotFound
	}
	debugPrintf("DEBUG: validateRcloneRemote - Saltbox user: '%s'\n", rcloneUser)

	// Check if the user exists on the system.
	_, err = user.Lookup(rcloneUser)
	if err != nil {
		debugPrintf("DEBUG: validateRcloneRemote - error looking up user\n")
		var unknownUserError user.UnknownUserError
		if errors.As(err, &unknownUserError) {
			err := fmt.Errorf("%w: user '%s' does not exist", ErrSystemUserNotFound, rcloneUser)
			debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
			return err
		}
		// Some other error occurred during user lookup.
		err := fmt.Errorf("error looking up user '%s': %w", rcloneUser, err)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateRcloneRemote - user exists\n")

	// Define the rclone config path (standard location).
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	debugPrintf("DEBUG: validateRcloneRemote - rcloneConfigPath: '%s'\n", rcloneConfigPath)

	// Check if the rclone config file exists
	_, err = os.Stat(rcloneConfigPath)
	if os.IsNotExist(err) {
		err := fmt.Errorf("%w: %v", ErrRcloneConfigNotFound, err)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateRcloneRemote - rclone config file exists\n")

	cmd := exec.Command("sudo", "-u", rcloneUser, "rclone", "config", "show")
	cmd.Env = append(os.Environ(), fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath))
	debugPrintf("DEBUG: validateRcloneRemote - command: '%s'\n", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		err := fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, output)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateRcloneRemote - rclone config show output: '%s'\n", string(output))

	// Use a regular expression to search for the remote within the rclone config show output.
	remoteRegex := fmt.Sprintf(`(?m)^\[%s\]$`, regexp.QuoteMeta(remoteName))
	re, err := regexp.Compile(remoteRegex)
	if err != nil {
		err := fmt.Errorf("failed to compile regex for remote name: %w", err)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateRcloneRemote - remoteRegex: '%s'\n", remoteRegex)

	if !re.MatchString(string(output)) {
		err := fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
		debugPrintf("DEBUG: validateRcloneRemote - %v\n", err)
		return err
	}

	debugPrintf("DEBUG: validateRcloneRemote - rclone remote exists\n")
	return nil
}
