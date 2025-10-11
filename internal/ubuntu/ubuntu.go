package ubuntu

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
)

// CheckSupport checks if the OS is Ubuntu and if it is one of the supported versions.
// Returns an error message if not supported, or nil if supported.
func CheckSupport(supportedVersions []string) error {
	// Check if OS is Linux
	osName, err := getOSName()
	if err != nil {
		return fmt.Errorf("error getting OS name: %w", err)
	}
	if osName != "linux" {
		return fmt.Errorf("not running on Linux (detected OS: %s)", osName)
	}

	// Parse /etc/os-release
	osRelease, err := ParseOSRelease("/etc/os-release")
	if err != nil {
		return fmt.Errorf("error parsing /etc/os-release: %w", err)
	}

	// Check if ID is ubuntu
	if osRelease["ID"] != "ubuntu" {
		return fmt.Errorf("not an Ubuntu distribution (detected ID: %s)", osRelease["ID"])
	}

	// Check if VERSION_ID is supported
	versionID, ok := osRelease["VERSION_ID"]
	if !ok {
		return fmt.Errorf("a Ubuntu version ID not found in /etc/os-release")
	}

	if slices.Contains(supportedVersions, versionID) {
		return nil // Supported version
	}

	return fmt.Errorf("unsupported Ubuntu version (detected version: %s, supported versions: %s)",
		versionID, strings.Join(supportedVersions, ", "))
}

// getOSName returns the lowercase OS name.
func getOSName() (string, error) {
	file, err := os.Open("/proc/sys/kernel/ostype")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	return strings.ToLower(scanner.Text()), scanner.Err()
}

// ParseOSRelease parses the /etc/os-release file and returns a map of key-value pairs.
func ParseOSRelease(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	osRelease := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := parts[0]
			value := strings.Trim(parts[1], "\"") // Remove quotes
			osRelease[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return osRelease, nil
}
