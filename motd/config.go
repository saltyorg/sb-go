package motd

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MOTDConfig represents the configuration for app queue status in the MOTD
type MOTDConfig struct {
	Sonarr  []AppInstance  `yaml:"sonarr"`
	Radarr  []AppInstance  `yaml:"radarr"`
	Lidarr  []AppInstance  `yaml:"lidarr"`
	Readarr []AppInstance  `yaml:"readarr"`
	Plex    []PlexInstance `yaml:"plex"`
}

// AppInstance represents a single app instance configuration
type AppInstance struct {
	Name   string `yaml:"name"`   // Optional friendly name
	URL    string `yaml:"url"`    // Base URL for the application
	APIKey string `yaml:"apikey"` // API key for authentication
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
