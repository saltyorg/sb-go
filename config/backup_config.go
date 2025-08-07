package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// BackupConfig represents the backup configuration.
type BackupConfig struct {
	Backup BackupSection `yaml:"backup"`
}

// BackupSection holds the various backup options.
type BackupSection struct {
	Cron           CronConfig           `yaml:"cron"`
	Local          LocalConfig          `yaml:"local"`
	Misc           MiscConfig           `yaml:"misc"`
	Rclone         RcloneConfig         `yaml:"rclone"`
	RestoreService RestoreServiceConfig `yaml:"restore_service"`
	Rsync          RsyncConfig          `yaml:"rsync"`
}

// CronConfig holds cron-related settings.
type CronConfig struct {
	CronTime string `yaml:"cron_time" validate:"required,cron_special_time"`
}

// LocalConfig holds local backup settings.
type LocalConfig struct {
	Destination string      `yaml:"destination" validate:"required"`
	Enable      AnsibleBool `yaml:"enable" validate:"required,ansiblebool"`
}

// MiscConfig holds miscellaneous backup settings.
type MiscConfig struct {
	Snapshot AnsibleBool `yaml:"snapshot" validate:"required,ansiblebool"`
}

// RcloneConfig holds rclone backup settings.
type RcloneConfig struct {
	Destination string      `yaml:"destination" validate:"required"`
	Enable      AnsibleBool `yaml:"enable" validate:"required,ansiblebool"`
	Template    string      `yaml:"template" validate:"required"`
}

// RestoreServiceConfig holds restore service settings.
type RestoreServiceConfig struct {
	Pass string `yaml:"pass"` // Optional
	User string `yaml:"user"` // Optional
}

// RsyncConfig holds rsync backup settings.
type RsyncConfig struct {
	Destination string      `yaml:"destination" validate:"required"`
	Enable      AnsibleBool `yaml:"enable" validate:"required,ansiblebool"`
	Port        string      `yaml:"port" validate:"required"`
}

// ValidateBackupConfig validates the BackupConfig struct.
func ValidateBackupConfig(config *BackupConfig, inputMap map[string]interface{}) error {
	debugPrintf("\nDEBUG: ValidateBackupConfig called with config: %+v, inputMap: %+v\n", config, inputMap)
	validate := validator.New()

	// Register custom validators (from generic.go).
	debugPrintf("DEBUG: ValidateBackupConfig - registering custom validators\n")
	RegisterCustomValidators(validate)

	// Validate the overall structure.
	debugPrintf("DEBUG: ValidateBackupConfig - validating struct: %+v\n", config)
	if err := validate.Struct(config); err != nil {
		debugPrintf("DEBUG: ValidateBackupConfig - struct validation error: %v\n", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "BackupConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				debugPrintf("DEBUG: ValidateBackupConfig - validation error on field '%s', tag '%s', value '%v', param '%s'\n", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					debugPrintf("DEBUG: ValidateBackupConfig - %v\n", err)
					return err
				case "ansiblebool":
					err := fmt.Errorf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s", fieldPath, e.Value())
					debugPrintf("DEBUG: ValidateBackupConfig - %v\n", err)
					return err
				case "cron_special_time":
					err := fmt.Errorf("field '%s' must be a valid Ansible cron special time (annually, daily, hourly, monthly, reboot, weekly, yearly), got: %s", fieldPath, e.Value())
					debugPrintf("DEBUG: ValidateBackupConfig - %v\n", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					debugPrintf("DEBUG: ValidateBackupConfig - %v\n", err)
					return err
				}
			}
		}
		return err
	}

	// Check for extra fields.
	debugPrintf("DEBUG: ValidateBackupConfig - checking for extra fields\n")
	if err := checkExtraFields(inputMap, config); err != nil {
		debugPrintf("DEBUG: ValidateBackupConfig - checkExtraFields returned error: %v\n", err)
		return err
	}

	debugPrintf("DEBUG: ValidateBackupConfig - validation successful\n")
	return nil
}
