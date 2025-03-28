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
	Port        string      `yaml:"port" validate:"required"` // Keep as string
}

// ValidateBackupConfig validates the BackupConfig struct.
func ValidateBackupConfig(config *BackupConfig, inputMap map[string]interface{}) error {
	validate := validator.New()

	// Register custom validators (from generic.go).
	RegisterCustomValidators(validate)

	// Validate the overall structure.
	if err := validate.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				lowercaseField := strings.ToLower(e.Field())
				switch e.Tag() {
				case "required":
					return fmt.Errorf("field '%s' is required", lowercaseField)
				case "ansiblebool":
					return fmt.Errorf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s", lowercaseField, e.Value())
				case "cron_special_time":
					return fmt.Errorf("field '%s' must be a valid Ansible cron special time (annually, daily, hourly, monthly, reboot, weekly, yearly), got: %s", lowercaseField, e.Value())
				default:
					return fmt.Errorf("field '%s' is invalid: %s", lowercaseField, e.Error())
				}
			}
		}
		return err
	}

	return checkExtraFields(inputMap, config) // Use the function from generic.go
}
