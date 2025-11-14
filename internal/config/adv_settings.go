package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/saltyorg/sb-go/internal/logging"

	"github.com/go-playground/validator/v10"
)

// AdvSettingsConfig represents the advanced settings configuration.
type AdvSettingsConfig struct {
	DNS     DNSConfig     `yaml:"dns"`
	Docker  DockerConfig  `yaml:"docker"`
	GPU     GPUConfig     `yaml:"gpu"`
	Mounts  MountsConfig  `yaml:"mounts"`
	System  SystemConfig  `yaml:"system"`
	Traefik TraefikConfig `yaml:"traefik"`
}

// DNSConfig holds DNS-related settings.
type DNSConfig struct {
	IPv4    AnsibleBool `yaml:"ipv4" validate:"required,ansiblebool"`
	IPv6    AnsibleBool `yaml:"ipv6" validate:"required,ansiblebool"`
	Proxied AnsibleBool `yaml:"proxied" validate:"required,ansiblebool"`
}

// DockerConfig holds Docker-related settings.
type DockerConfig struct {
	JSONDriver AnsibleBool `yaml:"json_driver" validate:"required,ansiblebool"`
}

// GPUConfig holds GPU-related settings.
type GPUConfig struct {
	Intel AnsibleBool `yaml:"intel" validate:"required,ansiblebool"`
}

// MountsConfig holds mount-related settings.
type MountsConfig struct {
	IPv4Only AnsibleBool `yaml:"ipv4_only" validate:"required,ansiblebool"`
}

// SystemConfig holds system-related settings.
type SystemConfig struct {
	Timezone string `yaml:"timezone" validate:"required,timezone_or_auto"`
}

// TraefikConfig holds Traefik-related settings.
type TraefikConfig struct {
	Cert       CertConfig       `yaml:"cert"`
	ErrorPages AnsibleBool      `yaml:"error_pages" validate:"required,ansiblebool"`
	HSTS       AnsibleBool      `yaml:"hsts" validate:"required,ansiblebool"`
	Metrics    AnsibleBool      `yaml:"metrics" validate:"required,ansiblebool"`
	Provider   string           `yaml:"provider" validate:"required"`
	Subdomains SubdomainsConfig `yaml:"subdomains"`
}

// CertConfig holds certificate-related settings.
type CertConfig struct {
	HTTPValidation AnsibleBool `yaml:"http_validation" validate:"required,ansiblebool"`
	ZeroSSL        AnsibleBool `yaml:"zerossl" validate:"required,ansiblebool"`
}

// SubdomainsConfig holds subdomain settings.
type SubdomainsConfig struct {
	Dash    string `yaml:"dash" validate:"required"`
	Metrics string `yaml:"metrics" validate:"required"`
}

// AnsibleBool is a custom type to handle Ansible boolean values.
type AnsibleBool string

// UnmarshalYAML implements custom unmarshalling for AnsibleBool.
func (a *AnsibleBool) UnmarshalYAML(unmarshal func(any) error) error {
	logging.DebugBool(verboseMode, "AnsibleBool.UnmarshalYAML called")
	var s string
	if err := unmarshal(&s); err != nil {
		logging.DebugBool(verboseMode, "AnsibleBool.UnmarshalYAML - error unmarshaling to string: %v", err)
		return err // If it's not unmarshalled as string, it's an error
	}
	normalizedVal := strings.ToLower(s)
	logging.DebugBool(verboseMode, "AnsibleBool.UnmarshalYAML - normalized value: '%s'", normalizedVal)
	switch normalizedVal {
	case "yes", "true", "on", "1", "no", "false", "off", "0":
		*a = AnsibleBool(normalizedVal)
		logging.DebugBool(verboseMode, "AnsibleBool.UnmarshalYAML - valid value, set to: '%s'", *a)
		return nil // Valid value
	default:
		err := fmt.Errorf("invalid Ansible boolean value: %s", s) // Return error
		logging.DebugBool(verboseMode, "AnsibleBool.UnmarshalYAML - %v", err)
		return err
	}
}

// ValidateAdvSettingsConfig validates the AdvSettingsConfig struct.
func ValidateAdvSettingsConfig(config *AdvSettingsConfig, inputMap map[string]any) error {
	logging.DebugBool(verboseMode, "\nDEBUG: ValidateAdvSettingsConfig called with config: %+v, inputMap: %+v", config, inputMap)
	validate := validator.New()

	// Register custom validators (from generic.go).
	logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - registering custom validators")
	RegisterCustomValidators(validate)

	// Validate the overall structure.
	logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - validating struct: %+v", config)
	if err := validate.Struct(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - struct validation error: %v", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "AdvSettingsConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - validation error on field '%s', tag '%s', value '%v', param '%s'", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - %v", err)
					return err
				case "ansiblebool":
					err := fmt.Errorf("field '%s' must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s", fieldPath, e.Value())
					logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - %v", err)
					return err
				case "timezone_or_auto":
					err := fmt.Errorf("field '%s' must be a valid timezone or 'auto', got: %s", fieldPath, e.Value())
					logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - %v", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - %v", err)
					return err
				}
			}
		}
		return err
	}

	// Check for extra fields.
	logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - checking for extra fields")
	if err := checkExtraFields(inputMap, config); err != nil {
		logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - checkExtraFields returned error: %v", err)
		return err
	}

	logging.DebugBool(verboseMode, "ValidateAdvSettingsConfig - validation successful")
	return nil
}
