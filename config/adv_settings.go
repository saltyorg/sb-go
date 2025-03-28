package config

import (
	"errors"
	"fmt"
	"strings"

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
func (a *AnsibleBool) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err // If it's not unmarshalled as string, it's an error
	}
	normalizedVal := strings.ToLower(s)
	switch normalizedVal {
	case "yes", "true", "on", "1", "no", "false", "off", "0":
		*a = AnsibleBool(normalizedVal)
		return nil // Valid value
	default:
		return fmt.Errorf("invalid Ansible boolean value: %s", s) // Return error
	}
}

// ValidateAdvSettingsConfig validates the AdvSettingsConfig struct.
func ValidateAdvSettingsConfig(config *AdvSettingsConfig, inputMap map[string]interface{}) error {
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
				case "timezone_or_auto":
					return fmt.Errorf("field '%s' must be a valid timezone or 'auto', got: %s", lowercaseField, e.Value())
				default:
					return fmt.Errorf("field '%s' is invalid: %s", lowercaseField, e.Error())
				}
			}
		}
		return err
	}

	return checkExtraFields(inputMap, config) // Use the function from generic.go
}
