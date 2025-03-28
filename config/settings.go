package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/saltyorg/sb-go/utils" // Import the utils package
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

	// Check for predefined values.
	switch strings.ToLower(value) {
	case "dropbox", "google", "sftp", "nfs":
		return true
	}

	// Check for absolute path and file existence.
	if strings.HasPrefix(value, "/") {
		_, err := os.Stat(value)
		return err == nil // Valid if the file exists
	}

	return false
}

// ValidateSettingsConfig validates the SettingsConfig struct.
func ValidateSettingsConfig(config *SettingsConfig, inputMap map[string]interface{}) error {
	validate := validator.New()
	RegisterCustomValidators(validate) //From generic.go

	err := validate.RegisterValidation("dirpath", dirPathValidator) // Register the dirpath validator
	if err != nil {
		return fmt.Errorf("failed to register dirpath validator: %w", err)
	}
	err = validate.RegisterValidation("rclone_template", rcloneTemplateValidator)
	if err != nil {
		return fmt.Errorf("failed to register rclone template validator: %w", err)
	}
	// Validate the struct.
	if err := validate.Struct(config); err != nil {
		return formatValidationError(err) // Use a helper function
	}

	// Now, validate nested structs explicitly.
	for _, remote := range config.Rclone.Remotes {
		if err := validate.Struct(remote); err != nil {
			return formatValidationError(err)
		}
		if err := validate.Struct(remote.Settings); err != nil {
			return formatValidationError(err)
		}
		if err := validate.Struct(remote.Settings.VFSCache); err != nil {
			return formatValidationError(err)
		}

		// Additional validation for rclone remote existence (except for NFS).
		if strings.ToLower(remote.Settings.Template) != "nfs" {
			// Split the remote string into name and path.
			parts := strings.SplitN(remote.Remote, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid remote format: '%s', expected 'remote:path'", remote.Remote)
			}
			remoteName := parts[0]

			if err := validateRcloneRemote(remoteName); err != nil {
				//Only return if rclone and the user and the config all exist
				if errors.Is(err, ErrRcloneNotInstalled) || errors.Is(err, ErrSystemUserNotFound) || errors.Is(err, ErrRcloneConfigNotFound) {
					fmt.Printf("\nWarning: rclone remote validation skipped: %v\n", err)
				} else {
					return err
				}
			}
		}
	}

	return checkExtraFields(inputMap, config)
}

// formatValidationError formats validation errors for better readability.
func formatValidationError(err error) error {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		var sb strings.Builder
		for _, e := range validationErrors {
			lowercaseField := strings.ToLower(e.Field())
			switch e.Tag() {
			case "required":
				sb.WriteString(fmt.Sprintf("field '%s' is required\n", lowercaseField))
			case "ansiblebool":
				sb.WriteString(fmt.Sprintf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s\n", lowercaseField, e.Value()))
			case "dirpath":
				sb.WriteString(fmt.Sprintf("field '%s' must be a valid directory path, got: %s\n", lowercaseField, e.Value()))
			case "rclone_template":
				sb.WriteString(fmt.Sprintf("field '%s' must be one of 'dropbox', 'google', 'sftp', 'nfs', or a valid absolute file path, got: %s\n", lowercaseField, e.Value()))
			default:
				sb.WriteString(fmt.Sprintf("field '%s' is invalid: %s\n", lowercaseField, e.Error()))
			}
		}
		return fmt.Errorf(sb.String())
	}
	return err // Return the original error if it's not a validator.ValidationErrors
}

// custom validator for directory paths
func dirPathValidator(fl validator.FieldLevel) bool {
	dirPath := fl.Field().String()

	// Check if the path is absolute or relative
	if !filepath.IsAbs(dirPath) {
		// If relative, make it absolute based on the current working directory
		wd, err := os.Getwd()
		if err != nil {
			return false // If we can't determine the working dir, consider it invalid
		}
		dirPath = filepath.Join(wd, dirPath)
	}
	// We don't check if the dir exists, only that the path is valid

	// Regular expression to check if it's a valid path (allows any characters)
	// Simplified regex, no need to check for starting slash separately.
	match, _ := regexp.MatchString(`(?:[^/]*/)*[^/]*$`, dirPath)
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
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRcloneNotInstalled, err)
	}
	// Get the Saltbox user.
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		fmt.Printf("\nWarning: rclone remote validation skipped: could not retrieve saltbox user: %v\n", err)
		return ErrSystemUserNotFound
	}

	// Check if the user exists on the system.
	_, err = user.Lookup(rcloneUser)
	if err != nil {
		var unknownUserError user.UnknownUserError
		if errors.As(err, &unknownUserError) {
			return fmt.Errorf("%w: user '%s' does not exist", ErrSystemUserNotFound, rcloneUser)
		}
		// Some other error occurred during user lookup.
		return fmt.Errorf("error looking up user '%s': %w", rcloneUser, err)
	}

	// Define the rclone config path (standard location).
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)

	// Check if the rclone config file exists
	_, err = os.Stat(rcloneConfigPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrRcloneConfigNotFound, err)
	}

	cmd := exec.Command("sudo", "-u", rcloneUser, "rclone", "config", "show")
	cmd.Env = append(os.Environ(), fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, output)
	}

	// Use a regular expression to search for the remote within the rclone config show output.
	remoteRegex := fmt.Sprintf(`(?m)^\[%s\]$`, regexp.QuoteMeta(remoteName))
	re, err := regexp.Compile(remoteRegex)
	if err != nil {
		return fmt.Errorf("failed to compile regex for remote name: %w", err)
	}

	if !re.MatchString(string(output)) {
		return fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
	}

	return nil
}
