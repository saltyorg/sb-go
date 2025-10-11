package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// MOTDConfig represents the MOTD configuration structure
type MOTDConfig struct {
	Sonarr      []AppInstance         `yaml:"sonarr"`
	Radarr      []AppInstance         `yaml:"radarr"`
	Lidarr      []AppInstance         `yaml:"lidarr"`
	Readarr     []AppInstance         `yaml:"readarr"`
	Plex        []PlexInstance        `yaml:"plex"`
	Jellyfin    []JellyfinInstance    `yaml:"jellyfin"`
	Emby        []EmbyInstance        `yaml:"emby"`
	Sabnzbd     []AppInstance         `yaml:"sabnzbd"`
	Nzbget      []UserPassAppInstance `yaml:"nzbget"`
	Qbittorrent []UserPassAppInstance `yaml:"qbittorrent"`
	Rtorrent    []UserPassAppInstance `yaml:"rtorrent"`
}

// AppInstance represents an app instance in the MOTD configuration
type AppInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	APIKey  string `yaml:"apikey" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
}

// PlexInstance represents a Plex server instance in the MOTD configuration
type PlexInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
}

// JellyfinInstance represents a Jellyfin server instance in the MOTD configuration
type JellyfinInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
}

// EmbyInstance represents an Emby server instance in the MOTD configuration
type EmbyInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
}

// UserPassAppInstance represents an app instance requiring user/pass auth in the MOTD configuration
type UserPassAppInstance struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url" validate:"omitempty,url"`
	User     string `yaml:"user" validate:"required_with=URL"`
	Password string `yaml:"password" validate:"required_with=URL"`
	Timeout  int    `yaml:"timeout" validate:"omitempty,gt=0"`
}

// LoadConfig loads the MOTD configuration from the specified file path
func LoadConfig(configPath string) (*MOTDConfig, error) {
	// Read the configuration file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	// Parse the YAML configuration
	var config MOTDConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// ValidateMOTDConfig validates the MOTD configuration
func ValidateMOTDConfig(config *MOTDConfig, inputMap map[string]any) error {
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
				case "gt":
					err := fmt.Errorf("field '%s' must be greater than %s", fieldPath, e.Param())
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

	// Additional validation for Jellyfin instances
	for _, instance := range config.Jellyfin {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Jellyfin instance: %+v\n", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("jellyfin instance '%s' has URL but no token", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("jellyfin instance '%s' has token but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Jellyfin instance '%s': %v", instance.Name, err)
				debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
				return err
			}
		}
	}

	// Additional validation for Emby instances
	for _, instance := range config.Emby {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Emby instance: %+v\n", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("emby instance '%s' has URL but no token", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("emby instance '%s' has token but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Emby instance '%s': %v", instance.Name, err)
				debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
				return err
			}
		}
	}

	// Additional validation for Sabnzbd instances
	for _, instance := range config.Sabnzbd {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Sabnzbd instance: %+v\n", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("sabnzbd instance '%s' has URL but no API key", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("sabnzbd instance '%s' has API key but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Nzbget instances
	for _, instance := range config.Nzbget {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Nzbget instance: %+v\n", instance)
		if instance.URL != "" && (instance.User == "" || instance.Password == "") {
			err := fmt.Errorf("nzbget instance '%s' has URL but is missing user or password", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("nzbget instance '%s' has user/password but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	// Additional validation for Qbittorrent instances
	for _, instance := range config.Qbittorrent {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating Qbittorrent instance: %+v\n", instance)
		if instance.URL != "" && (instance.User == "" || instance.Password == "") {
			err := fmt.Errorf("qbittorrent instance '%s' has URL but is missing user or password", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("qbittorrent instance '%s' has user/password but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	for _, instance := range config.Rtorrent {
		debugPrintf("DEBUG: validateMOTDNestedConfigs - validating rTorrent instance: %+v\n", instance)
		// It is valid to have a URL without user/pass for rTorrent,
		// but not the other way around.
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("rtorrent instance '%s' has user/password but no URL", instance.Name)
			debugPrintf("DEBUG: validateMOTDNestedConfigs - %v\n", err)
			return err
		}
	}

	debugPrintf("DEBUG: validateMOTDNestedConfigs - nested validation successful\n")
	return nil
}
