package motd

import "fmt"

// GenerateExampleConfig returns a YAML string with an example MOTD configuration
// where all sections are disabled and contain placeholder values.
// Color values are taken from the actual defaults defined in this package.
func GenerateExampleConfig() string {
	return fmt.Sprintf(`# Saltbox MOTD Configuration
# All sections are disabled by default. Set enabled: true and fill in the values to use.

# Sonarr instances for download queue information
sonarr:
  enabled: false
  instances:
    - name: Sonarr
      url: http://localhost:8989
      apikey: your-api-key-here
      timeout: 5

# Radarr instances for download queue information
radarr:
  enabled: false
  instances:
    - name: Radarr
      url: http://localhost:7878
      apikey: your-api-key-here
      timeout: 5

# Lidarr instances for download queue information
lidarr:
  enabled: false
  instances:
    - name: Lidarr
      url: http://localhost:8686
      apikey: your-api-key-here
      timeout: 5

# Readarr instances for download queue information
readarr:
  enabled: false
  instances:
    - name: Readarr
      url: http://localhost:8787
      apikey: your-api-key-here
      timeout: 5

# Plex instances for streaming information
plex:
  enabled: false
  instances:
    - name: Plex
      url: http://localhost:32400
      token: your-plex-token-here
      timeout: 5

# Jellyfin instances for streaming information
jellyfin:
  enabled: false
  instances:
    - name: Jellyfin
      url: http://localhost:8096
      token: your-api-token-here
      timeout: 5

# Emby instances for streaming information
emby:
  enabled: false
  instances:
    - name: Emby
      url: http://localhost:8096
      token: your-api-token-here
      timeout: 5

# SABnzbd instances for download information
sabnzbd:
  enabled: false
  instances:
    - name: SABnzbd
      url: http://localhost:8080
      apikey: your-api-key-here
      timeout: 5

# NZBGet instances for download information
nzbget:
  enabled: false
  instances:
    - name: NZBGet
      url: http://localhost:6789
      user: your-username
      password: your-password
      timeout: 5

# qBittorrent instances for torrent information
qbittorrent:
  enabled: false
  instances:
    - name: qBittorrent
      url: http://localhost:8080
      user: your-username
      password: your-password
      timeout: 5

# rTorrent instances for torrent information
rtorrent:
  enabled: false
  instances:
    - name: rTorrent
      url: http://localhost:8080
      user: your-username  # optional
      password: your-password  # optional
      timeout: 5

# Systemd services monitoring
systemd:
  enabled: false
  additional_services: []
  display_names: {}

# Custom color scheme (all values are hex colors)
# These are the default values - uncomment and modify to customize
colors:
  text:
    label: "%s"
    value: "%s"
    app_name: "%s"
  status:
    warning: "%s"
    success: "%s"
    error: "%s"
  progress_bar:
    low: "%s"
    high: "%s"
    critical: "%s"
`, defaultKey, defaultValue, defaultAppName,
		defaultWarning, defaultSuccess, defaultError,
		defaultProgressBarLow, defaultProgressBarHigh, defaultProgressBarCritical)
}
