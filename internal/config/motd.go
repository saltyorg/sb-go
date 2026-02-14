package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MOTDConfig represents the MOTD configuration structure
type MOTDConfig struct {
	Sonarr      *AppSection         `yaml:"sonarr"`
	Radarr      *AppSection         `yaml:"radarr"`
	Lidarr      *AppSection         `yaml:"lidarr"`
	Readarr     *AppSection         `yaml:"readarr"`
	Plex        *PlexSection        `yaml:"plex"`
	Jellyfin    *JellyfinSection    `yaml:"jellyfin"`
	Emby        *EmbySection        `yaml:"emby"`
	Sabnzbd     *AppSection         `yaml:"sabnzbd"`
	Nzbget      *UserPassAppSection `yaml:"nzbget"`
	Qbittorrent *UserPassAppSection `yaml:"qbittorrent"`
	Rtorrent    *UserPassAppSection `yaml:"rtorrent"`
	Systemd     *SystemdConfig      `yaml:"systemd"`
	Colors      *MOTDColors         `yaml:"colors"`
}

// AppSection wraps app instances with a section-level enabled toggle
type AppSection struct {
	Enabled   *bool         `yaml:"enabled,omitempty"`
	Instances []AppInstance `yaml:"instances"`
}

// UnmarshalYAML implements custom unmarshalling to support both old and new config formats.
// Old format: direct array of instances
// New format: object with enabled and instances fields
func (s *AppSection) UnmarshalYAML(value *yaml.Node) error {
	// Check if this is a sequence (old format: direct array of instances)
	if value.Kind == yaml.SequenceNode {
		var instances []AppInstance
		if err := value.Decode(&instances); err != nil {
			return err
		}
		s.Instances = instances
		s.Enabled = nil // defaults to enabled
		return nil
	}

	// Otherwise, decode as the new format (mapping with enabled/instances)
	type rawAppSection AppSection
	return value.Decode((*rawAppSection)(s))
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (s *AppSection) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// PlexSection wraps Plex instances with a section-level enabled toggle
type PlexSection struct {
	Enabled   *bool          `yaml:"enabled,omitempty"`
	Instances []PlexInstance `yaml:"instances"`
}

// UnmarshalYAML implements custom unmarshalling to support both old and new config formats.
func (s *PlexSection) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var instances []PlexInstance
		if err := value.Decode(&instances); err != nil {
			return err
		}
		s.Instances = instances
		s.Enabled = nil
		return nil
	}
	type rawPlexSection PlexSection
	return value.Decode((*rawPlexSection)(s))
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (s *PlexSection) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// JellyfinSection wraps Jellyfin instances with a section-level enabled toggle
type JellyfinSection struct {
	Enabled   *bool              `yaml:"enabled,omitempty"`
	Instances []JellyfinInstance `yaml:"instances"`
}

// UnmarshalYAML implements custom unmarshalling to support both old and new config formats.
func (s *JellyfinSection) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var instances []JellyfinInstance
		if err := value.Decode(&instances); err != nil {
			return err
		}
		s.Instances = instances
		s.Enabled = nil
		return nil
	}
	type rawJellyfinSection JellyfinSection
	return value.Decode((*rawJellyfinSection)(s))
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (s *JellyfinSection) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// EmbySection wraps Emby instances with a section-level enabled toggle
type EmbySection struct {
	Enabled   *bool          `yaml:"enabled,omitempty"`
	Instances []EmbyInstance `yaml:"instances"`
}

// UnmarshalYAML implements custom unmarshalling to support both old and new config formats.
func (s *EmbySection) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var instances []EmbyInstance
		if err := value.Decode(&instances); err != nil {
			return err
		}
		s.Instances = instances
		s.Enabled = nil
		return nil
	}
	type rawEmbySection EmbySection
	return value.Decode((*rawEmbySection)(s))
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (s *EmbySection) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// UserPassAppSection wraps user/pass app instances with a section-level enabled toggle
type UserPassAppSection struct {
	Enabled   *bool                 `yaml:"enabled,omitempty"`
	Instances []UserPassAppInstance `yaml:"instances"`
}

// UnmarshalYAML implements custom unmarshalling to support both old and new config formats.
func (s *UserPassAppSection) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var instances []UserPassAppInstance
		if err := value.Decode(&instances); err != nil {
			return err
		}
		s.Instances = instances
		s.Enabled = nil
		return nil
	}
	type rawUserPassAppSection UserPassAppSection
	return value.Decode((*rawUserPassAppSection)(s))
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (s *UserPassAppSection) IsEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}

// SystemdConfig represents configuration for the systemd services section
type SystemdConfig struct {
	Enabled            *bool             `yaml:"enabled,omitempty"`
	AdditionalServices []string          `yaml:"additional_services"`
	DisplayNames       map[string]string `yaml:"display_names"`
}

// IsEnabled returns true if the section is enabled (defaults to true if not set)
func (c *SystemdConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
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
