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
	debugPrintf("\nDEBUG: ValidateMOTDConfig called with config: %+v, inputMap: %+v\n", config, inputMap)
	validate := validator.New()

	// Register custom validators
	debugPrintf("DEBUG: ValidateMOTDConfig - registering custom validators\n")
	RegisterCustomValidators(validate)

	// Validate the overall structure
	debugPrintf("DEBUG: ValidateMOTDConfig - validating struct: %+v\n", config)
	if err := validate.Struct(config); err != nil {
		debugPrintf("DEBUG: ValidateMOTDConfig - struct validation error: %v\n", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "MOTDConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				debugPrintf("DEBUG: ValidateMOTDConfig - validation error on field '%s', tag '%s', value '%v', param '%s'\n", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					debugPrintf("DEBUG: ValidateMOTDConfig - %v\n", err)
					return err
				case "url":
					err := fmt.Errorf("field '%s' must be a valid URL", fieldPath)
					debugPrintf("DEBUG: ValidateMOTDConfig - %v\n", err)
					return err
				case "required_with":
					err := fmt.Errorf("field '%s' is required when %s is provided", fieldPath, e.Param())
					debugPrintf("DEBUG: ValidateMOTDConfig - %v\n", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					debugPrintf("DEBUG: ValidateMOTDConfig - %v\n", err)
					return err
				}
			}
		}
		return err
	}

	// Additional validation for nested objects
	debugPrintf("DEBUG: ValidateMOTDConfig - validating nested configurations\n")
	if err := validateMOTDNestedConfigs(config); err != nil {
		debugPrintf("DEBUG: ValidateMOTDConfig - validateMOTDNestedConfigs returned error: %v\n", err)
		return err
	}

	// Check for extra fields
	debugPrintf("DEBUG: ValidateMOTDConfig - checking for extra fields\n")
	if err := checkExtraFields(inputMap, config); err != nil {
		debugPrintf("DEBUG: ValidateMOTDConfig - checkExtraFields returned error: %v\n", err)
		return err
	}

	debugPrintf("DEBUG: ValidateMOTDConfig - validation successful\n")
	return nil
}

// validateMOTDNestedConfigs performs additional validation on nested configurations
func validateMOTDNestedConfigs(config *MOTDConfig) error {
	debugPrintf("DEBUG: validateMOTDNestedConfigs called with config: %+v\n", config)

	// Additional validation for Sonarr instances
	for _, instance := range config.Sonarr {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Sonarr instance: %+v\n", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("sonarr instance '%s' has URL but no API key", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("sonarr instance '%s' has API key but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Radarr instances
	for _, instance := range config.Radarr {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Radarr instance: %+v\n", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("radarr instance '%s' has URL but no API key", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("radarr instance '%s' has API key but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Lidarr instances
	for _, instance := range config.Lidarr {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Lidarr instance: %+v\n", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("lidarr instance '%s' has URL but no API key", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("lidarr instance '%s' has API key but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Readarr instances
	for _, instance := range config.Readarr {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Readarr instance: %+v\n", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("readarr instance '%s' has URL but no API key", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("readarr instance '%s' has API key but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Plex instances
	for _, instance := range config.Plex {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Plex instance: %+v\n", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("plex instance '%s' has URL but no token", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("plex instance '%s' has token but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}

		// If URL is provided, validate it's parseable
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Plex instance '%s': %v", instance.Name, err)
				debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
				return err
			}
		}
	}

	debugPrintf("DEBUG: validateMOTDNestedConfigs - nested validation successful\n")
	return nil
}
