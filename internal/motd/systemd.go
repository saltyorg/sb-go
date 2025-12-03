package motd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/systemd"
)

// defaultStripPrefixes are the prefixes to strip from service names by default
var defaultStripPrefixes = []string{"saltbox_managed_"}

// GetSystemdServicesInfo returns formatted information about systemd services.
// It uses the default filters (saltbox_managed_* prefix and docker exact match)
// plus any additional services specified in the MOTD config file.
func GetSystemdServicesInfo(ctx context.Context, verbose bool) string {
	// Load config if available
	var additionalServices []string
	stripPrefixes := defaultStripPrefixes
	configPath := constants.SaltboxMOTDConfigPath

	if _, err := os.Stat(configPath); err == nil {
		cfg, err := config.LoadConfig(configPath)
		if err == nil && cfg.Systemd != nil {
			additionalServices = cfg.Systemd.AdditionalServices
			if len(cfg.Systemd.StripPrefixes) > 0 {
				stripPrefixes = cfg.Systemd.StripPrefixes
			}
		}
	}

	filters := systemd.FiltersWithAdditional(additionalServices)

	services, err := systemd.GetFilteredServices(ctx, filters)
	if err != nil {
		if verbose {
			return DefaultStyle.Render(fmt.Sprintf("Error getting services: %v", err))
		}
		return ""
	}

	if len(services) == 0 {
		return ""
	}

	// Build display names and find max length for alignment
	displayNames := make([]string, len(services))
	maxNameLen := 0
	for i, svc := range services {
		displayNames[i] = getDisplayName(svc.Name, stripPrefixes)
		if len(displayNames[i]) > maxNameLen {
			maxNameLen = len(displayNames[i])
		}
	}

	var lines []string
	for i, svc := range services {
		line := formatServiceLine(svc, displayNames[i], maxNameLen)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// getDisplayName returns the display name for a service after stripping configured prefixes.
func getDisplayName(name string, stripPrefixes []string) string {
	for _, prefix := range stripPrefixes {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

// formatServiceLine formats a single service line with status and runtime.
func formatServiceLine(svc systemd.ServiceInfo, displayName string, maxNameLen int) string {
	// Pad display name for alignment
	padding := maxNameLen - len(displayName)
	paddedName := displayName + strings.Repeat(" ", padding)

	// Format status with color
	var statusStr string
	switch svc.Active {
	case "active":
		// Show sub status for active services (e.g., "active/running")
		status := svc.Active
		if svc.Sub != "" && svc.Sub != svc.Active {
			status = fmt.Sprintf("%s/%s", svc.Active, svc.Sub)
		}
		if svc.Runtime != "" {
			statusStr = SuccessStyle.Render(fmt.Sprintf("%s • %s", status, svc.Runtime))
		} else {
			statusStr = SuccessStyle.Render(status)
		}
	case "failed":
		statusStr = ErrorStyle.Render("failed")
	case "inactive":
		statusStr = WarningStyle.Render("inactive")
	default:
		statusStr = WarningStyle.Render(svc.Active)
	}

	return fmt.Sprintf("%s   %s", DefaultStyle.Render(paddedName), statusStr)
}
