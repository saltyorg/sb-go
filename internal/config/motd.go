package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/logging"

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
	Systemd     *SystemdConfig        `yaml:"systemd"`
	Colors      *MOTDColors           `yaml:"colors"`
}

// SystemdConfig represents configuration for the systemd services section
type SystemdConfig struct {
	AdditionalServices []string `yaml:"additional_services"`
	StripPrefixes      []string `yaml:"strip_prefixes"`
}

// MOTDColors represents customizable color scheme for MOTD
type MOTDColors struct {
	Text        *TextColors        `yaml:"text"`
	Status      *StatusColors      `yaml:"status"`
	ProgressBar *ProgressBarColors `yaml:"progress_bar"`
}

// TextColors represents customizable colors for text elements
type TextColors struct {
	Label   string `yaml:"label" validate:"omitempty,hexcolor"`
	Value   string `yaml:"value" validate:"omitempty,hexcolor"`
	AppName string `yaml:"app_name" validate:"omitempty,hexcolor"`
}

// StatusColors represents customizable colors for status messages
type StatusColors struct {
	Warning string `yaml:"warning" validate:"omitempty,hexcolor"`
	Success string `yaml:"success" validate:"omitempty,hexcolor"`
	Error   string `yaml:"error" validate:"omitempty,hexcolor"`
}

// ProgressBarColors represents customizable colors for progress bars
type ProgressBarColors struct {
	Low      string `yaml:"low" validate:"omitempty,hexcolor"`
	High     string `yaml:"high" validate:"omitempty,hexcolor"`
	Critical string `yaml:"critical" validate:"omitempty,hexcolor"`
}

// AppInstance represents an app instance in the MOTD configuration
type AppInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	APIKey  string `yaml:"apikey" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

// PlexInstance represents a Plex server instance in the MOTD configuration
type PlexInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

// JellyfinInstance represents a Jellyfin server instance in the MOTD configuration
type JellyfinInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

// EmbyInstance represents an Emby server instance in the MOTD configuration
type EmbyInstance struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url" validate:"omitempty,url"`
	Token   string `yaml:"token" validate:"required_with=URL"`
	Timeout int    `yaml:"timeout" validate:"omitempty,gt=0"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

// UserPassAppInstance represents an app instance requiring user/pass auth in the MOTD configuration
type UserPassAppInstance struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url" validate:"omitempty,url"`
	User     string `yaml:"user" validate:"required_with=URL"`
	Password string `yaml:"password" validate:"required_with=URL"`
	Timeout  int    `yaml:"timeout" validate:"omitempty,gt=0"`
	Enabled  *bool  `yaml:"enabled,omitempty"`
}

// IsEnabled returns true if the instance is enabled (defaults to true if not set)
func (i AppInstance) IsEnabled() bool {
	return i.Enabled == nil || *i.Enabled
}

// IsEnabled returns true if the instance is enabled (defaults to true if not set)
func (i PlexInstance) IsEnabled() bool {
	return i.Enabled == nil || *i.Enabled
}

// IsEnabled returns true if the instance is enabled (defaults to true if not set)
func (i JellyfinInstance) IsEnabled() bool {
	return i.Enabled == nil || *i.Enabled
}

// IsEnabled returns true if the instance is enabled (defaults to true if not set)
func (i EmbyInstance) IsEnabled() bool {
	return i.Enabled == nil || *i.Enabled
}

// IsEnabled returns true if the instance is enabled (defaults to true if not set)
func (i UserPassAppInstance) IsEnabled() bool {
	return i.Enabled == nil || *i.Enabled
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
	logging.DebugBool(verboseMode, "\nDEBUG: ValidateMOTDConfig called with config: %+v, inputMap: %+v", config, inputMap)
	validate := validator.New()

	// Register custom validators
	logging.DebugBool(verboseMode, "ValidateMOTDConfig - registering custom validators")
	if err := RegisterCustomValidators(validate); err != nil {
		return err
	}

	// Validate the overall structure
	logging.DebugBool(verboseMode, "ValidateMOTDConfig - validating struct: %+v", config)
	if err := validate.Struct(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateMOTDConfig - struct validation error: %v", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "MOTDConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				logging.DebugBool(verboseMode, "ValidateMOTDConfig - validation error on field '%s', tag '%s', value '%v', param '%s'", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					logging.DebugBool(verboseMode, "ValidateMOTDConfig - %v", err)
					return err
				case "url":
					err := fmt.Errorf("field '%s' must be a valid URL", fieldPath)
					logging.DebugBool(verboseMode, "ValidateMOTDConfig - %v", err)
					return err
				case "required_with":
					err := fmt.Errorf("field '%s' is required when %s is provided", fieldPath, e.Param())
					logging.DebugBool(verboseMode, "ValidateMOTDConfig - %v", err)
					return err
				case "gt":
					err := fmt.Errorf("field '%s' must be greater than %s", fieldPath, e.Param())
					logging.DebugBool(verboseMode, "ValidateMOTDConfig - %v", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					logging.DebugBool(verboseMode, "ValidateMOTDConfig - %v", err)
					return err
				}
			}
		}
		return err
	}

	// Additional validation for nested objects
	logging.DebugBool(verboseMode, "ValidateMOTDConfig - validating nested configurations")
	if err := validateMOTDNestedConfigs(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateMOTDConfig - validateMOTDNestedConfigs returned error: %v", err)
		return err
	}

	// Check for extra fields
	logging.DebugBool(verboseMode, "ValidateMOTDConfig - checking for extra fields")
	if err := checkExtraFields(inputMap, config); err != nil {
		logging.DebugBool(verboseMode, "ValidateMOTDConfig - checkExtraFields returned error: %v", err)
		return err
	}

	logging.DebugBool(verboseMode, "ValidateMOTDConfig - validation successful")
	return nil
}

// validateMOTDNestedConfigs performs additional validation on nested configurations
func validateMOTDNestedConfigs(config *MOTDConfig) error {
	logging.DebugBool(verboseMode, "validateMOTDNestedConfigs called with config: %+v", config)

	// Additional validation for Sonarr instances
	for _, instance := range config.Sonarr {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Sonarr instance: %+v", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("sonarr instance '%s' has URL but no API key", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("sonarr instance '%s' has API key but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Radarr instances
	for _, instance := range config.Radarr {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Radarr instance: %+v", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("radarr instance '%s' has URL but no API key", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("radarr instance '%s' has API key but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Lidarr instances
	for _, instance := range config.Lidarr {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Lidarr instance: %+v", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("lidarr instance '%s' has URL but no API key", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("lidarr instance '%s' has API key but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Readarr instances
	for _, instance := range config.Readarr {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Readarr instance: %+v", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("readarr instance '%s' has URL but no API key", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("readarr instance '%s' has API key but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Plex instances
	for _, instance := range config.Plex {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Plex instance: %+v", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("plex instance '%s' has URL but no token", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("plex instance '%s' has token but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}

		// If URL is provided, validate it's parseable
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Plex instance '%s': %v", instance.Name, err)
				logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
				return err
			}
		}
	}

	// Additional validation for Jellyfin instances
	for _, instance := range config.Jellyfin {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Jellyfin instance: %+v", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("jellyfin instance '%s' has URL but no token", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("jellyfin instance '%s' has token but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Jellyfin instance '%s': %v", instance.Name, err)
				logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
				return err
			}
		}
	}

	// Additional validation for Emby instances
	for _, instance := range config.Emby {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Emby instance: %+v", instance)
		if instance.URL != "" && instance.Token == "" {
			err := fmt.Errorf("emby instance '%s' has URL but no token", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.Token != "" && instance.URL == "" {
			err := fmt.Errorf("emby instance '%s' has token but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.URL != "" {
			_, err := url.Parse(instance.URL)
			if err != nil {
				err := fmt.Errorf("invalid URL for Emby instance '%s': %v", instance.Name, err)
				logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
				return err
			}
		}
	}

	// Additional validation for Sabnzbd instances
	for _, instance := range config.Sabnzbd {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Sabnzbd instance: %+v", instance)
		if instance.URL != "" && instance.APIKey == "" {
			err := fmt.Errorf("sabnzbd instance '%s' has URL but no API key", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.APIKey != "" && instance.URL == "" {
			err := fmt.Errorf("sabnzbd instance '%s' has API key but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Nzbget instances
	for _, instance := range config.Nzbget {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Nzbget instance: %+v", instance)
		if instance.URL != "" && (instance.User == "" || instance.Password == "") {
			err := fmt.Errorf("nzbget instance '%s' has URL but is missing user or password", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("nzbget instance '%s' has user/password but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	// Additional validation for Qbittorrent instances
	for _, instance := range config.Qbittorrent {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating Qbittorrent instance: %+v", instance)
		if instance.URL != "" && (instance.User == "" || instance.Password == "") {
			err := fmt.Errorf("qbittorrent instance '%s' has URL but is missing user or password", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("qbittorrent instance '%s' has user/password but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	for _, instance := range config.Rtorrent {
		if !instance.IsEnabled() {
			continue // Skip validation for disabled instances
		}
		logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - validating rTorrent instance: %+v", instance)
		// It is valid to have a URL without user/pass for rTorrent,
		// but not the other way around.
		if instance.URL == "" && (instance.User != "" || instance.Password != "") {
			err := fmt.Errorf("rtorrent instance '%s' has user/password but no URL", instance.Name)
			logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - %v", err)
			return err
		}
	}

	logging.DebugBool(verboseMode, "validateMOTDNestedConfigs - nested validation successful")
	return nil
}
