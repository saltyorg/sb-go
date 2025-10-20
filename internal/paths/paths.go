package paths

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// inventoryConfig represents relevant configuration from the Saltbox inventory file.
type inventoryConfig struct {
	ServerAppdataPath string `yaml:"server_appdata_path"`
}

// loadServerAppdataPath loads the server_appdata_path from the Saltbox inventory localhost.yml.
// It returns the server_appdata_path value if found, otherwise returns the default "/opt".
// This function gracefully handles missing files and parsing errors by returning the default.
func loadServerAppdataPath(inventoryPath string) string {
	const defaultPath = "/opt"

	// Check if the inventory file exists
	data, err := os.ReadFile(inventoryPath)
	if err != nil {
		// File doesn't exist or can't be read - use default
		return defaultPath
	}

	// Parse the YAML file
	var config inventoryConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		// YAML parsing error - use default
		return defaultPath
	}

	// If server_appdata_path is not set or empty, use default
	if config.ServerAppdataPath == "" {
		return defaultPath
	}

	return config.ServerAppdataPath
}

// These paths are configurable based on server_appdata_path from inventory.
// They default to /opt but can be overridden via inventories/host_vars/localhost.yml.
var (
	SaltboxFactsPath   string
	SandboxRepoPath    string
	SaltboxModRepoPath string
)

func init() {
	const saltboxInventoryPath = "/srv/git/saltbox/inventories/host_vars/localhost.yml"

	// Load the server_appdata_path from inventory, defaults to "/opt" if not found
	basePath := loadServerAppdataPath(saltboxInventoryPath)

	// Initialize the configurable paths
	SaltboxFactsPath = filepath.Join(basePath, "saltbox")
	SandboxRepoPath = filepath.Join(basePath, "sandbox")
	SaltboxModRepoPath = filepath.Join(basePath, "saltbox_mod")
}
