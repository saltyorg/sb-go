package layout

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
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
	if config.ServerAppdataPath == "" || !filepath.IsAbs(config.ServerAppdataPath) {
		return defaultPath
	}

	return filepath.Clean(config.ServerAppdataPath)
}

// Paths contains the installation paths derived from server_appdata_path.
// Callers receive this value by copy so they cannot mutate package state.
type Paths struct {
	SaltboxFactsPath   string
	SandboxRepoPath    string
	SaltboxModRepoPath string
}

func pathsForInventory(inventoryPath string) Paths {
	basePath := loadServerAppdataPath(inventoryPath)
	return Paths{
		SaltboxFactsPath:   filepath.Join(basePath, "saltbox"),
		SandboxRepoPath:    filepath.Join(basePath, "sandbox"),
		SaltboxModRepoPath: filepath.Join(basePath, "saltbox_mod"),
	}
}

var currentPaths = pathsForInventory(SaltboxInventoryConfigPath)

// Current returns the process's resolved installation layout.
func Current() Paths {
	return currentPaths
}
