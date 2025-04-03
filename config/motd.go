package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
)

// MOTDConfig represents the MOTD configuration structure
type MOTDConfig struct {
	Sonarr  []MOTDAppInstance  `yaml:"sonarr"`
	Radarr  []MOTDAppInstance  `yaml:"radarr"`
	Lidarr  []MOTDAppInstance  `yaml:"lidarr"`
	Readarr []MOTDAppInstance  `yaml:"readarr"`
	Plex    []MOTDPlexInstance `yaml:"plex"`
}

// MOTDAppInstance represents an app instance in the MOTD configuration
type MOTDAppInstance struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url" validate:"omitempty,url"`
	APIKey string `yaml:"apikey" validate:"required_with=URL"`
}

// MOTDPlexInstance represents a Plex server instance in the MOTD configuration
type MOTDPlexInstance struct {
	Name  string `yaml:"name"`
	URL   string `yaml:"url" validate:"omitempty,url"`
	Token string `yaml:"token" validate:"required_with=URL"`
}

// ValidateMOTDConfig validates the MOTD configuration
func ValidateMOTDConfig(config *MOTDConfig, inputMap map[string]interface{}) error {
	validate := validator.New()

	// Register custom validators
	RegisterCustomValidators(validate)

	// Validate the overall structure
	if err := validate.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				lowercaseField := strings.ToLower(e.Field())
				switch e.Tag() {
				case "required":
					return fmt.Errorf("field '%s' is required", lowercaseField)
				case "url":
					return fmt.Errorf("field '%s' must be a valid URL", lowercaseField)
				case "required_with":
					return fmt.Errorf("field '%s' is required when %s is provided", lowercaseField, e.Param())
				default:
					return fmt.Errorf("field '%s' is invalid: %s", lowercaseField, e.Error())
				}
			}
		}
		return err
	}

	// Additional validation for nested objects
	if err := validateMOTDNestedConfigs(config); err != nil {
		return err
	}

	// Check for extra fields
	return checkExtraFields(inputMap, config)
}

// validateMOTDNestedConfigs performs additional validation on nested configurations
func validateMOTDNestedConfigs(config *MOTDConfig) error {
	// Additional validation for Sonarr instances
	for _, instance := range config.Sonarr {
		if instance.URL != "" && instance.APIKey == "" {
			return fmt.Errorf("sonarr instance '%s' has URL but no API key", instance.Name)
		}
		if instance.APIKey != "" && instance.URL == "" {
			return fmt.Errorf("sonarr instance '%s' has API key but no URL", instance.Name)
		}
	}

	// Additional validation for Radarr instances
	for _, instance := range config.Radarr {
		if instance.URL != "" && instance.APIKey == "" {
			return fmt.Errorf("radarr instance '%s' has URL but no API key", instance.Name)
		}
		if instance.APIKey != "" && instance.URL == "" {
			return fmt.Errorf("radarr instance '%s' has API key but no URL", instance.Name)
		}
	}

	// Additional validation for Lidarr instances
	for _, instance := range config.Lidarr {
		if instance.URL != "" && instance.APIKey == "" {
			return fmt.Errorf("lidarr instance '%s' has URL but no API key", instance.Name)
		}
		if instance.APIKey != "" && instance.URL == "" {
			return fmt.Errorf("lidarr instance '%s' has API key but no URL", instance.Name)
		}
	}

	// Additional validation for Readarr instances
	for _, instance := range config.Readarr {
		if instance.URL != "" && instance.APIKey == "" {
			return fmt.Errorf("readarr instance '%s' has URL but no API key", instance.Name)
		}
		if instance.APIKey != "" && instance.URL == "" {
			return fmt.Errorf("readarr instance '%s' has API key but no URL", instance.Name)
		}
	}

	// Additional validation for Plex instances
	for _, instance := range config.Plex {
		if instance.URL != "" && instance.Token == "" {
			return fmt.Errorf("plex instance '%s' has URL but no token", instance.Name)
		}
		if instance.Token != "" && instance.URL == "" {
			return fmt.Errorf("plex instance '%s' has token but no URL", instance.Name)
		}

		// If URL is provided, validate it's parseable
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				return fmt.Errorf("invalid URL for Plex instance '%s': %v", instance.Name, err)
			}
		}
	}

	return nil
}
