package motd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/systemd"
)

// defaultDisplayNames maps service names to their display names
var defaultDisplayNames = map[string]string{
	"docker":                            "Docker",
	"saltbox_managed_docker_controller": "Saltbox Docker Controller",
	"saltbox_managed_docker_controller_helper": "Saltbox Docker Controller Helper",
	"saltbox_managed_docker_update_hosts":      "Saltbox Docker Hosts Manager",
	"saltbox_managed_mergerfs":                 "Mergerfs",
}

// GetSystemdServicesInfo returns formatted information about systemd services.
// It uses the default filters (saltbox_managed_* prefix and docker exact match)
// plus any additional services specified in the MOTD config file.
func GetSystemdServicesInfo(ctx context.Context, verbose bool) string {
	// Load config if available
	var additionalServices []string
	var userDisplayNames map[string]string
	configPath := constants.SaltboxMOTDConfigPath

	if _, err := os.Stat(configPath); err == nil {
		cfg, err := config.LoadConfig(configPath)
		if err == nil && cfg.Systemd != nil {
			additionalServices = cfg.Systemd.AdditionalServices
			userDisplayNames = cfg.Systemd.DisplayNames
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

	// Build display names and create index for sorting
	type serviceWithDisplay struct {
		service     systemd.ServiceInfo
		displayName string
	}
	servicesWithNames := make([]serviceWithDisplay, len(services))
	maxNameLen := 0
	for i, svc := range services {
		displayName := getDisplayName(svc.Name, userDisplayNames)
		servicesWithNames[i] = serviceWithDisplay{service: svc, displayName: displayName}
		if len(displayName) > maxNameLen {
			maxNameLen = len(displayName)
		}
	}

	// Sort by display name
	sort.Slice(servicesWithNames, func(i, j int) bool {
		return servicesWithNames[i].displayName < servicesWithNames[j].displayName
	})

	var lines []string
	for _, swd := range servicesWithNames {
		line := formatServiceLine(swd.service, swd.displayName, maxNameLen)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// getDisplayName returns the display name for a service.
// It checks user-configured display names first, then falls back to defaults.
func getDisplayName(name string, userDisplayNames map[string]string) string {
	// User config takes priority
	if displayName, ok := userDisplayNames[name]; ok {
		return displayName
	}
	// Fall back to built-in defaults
	if displayName, ok := defaultDisplayNames[name]; ok {
		return displayName
	}
	// No mapping found, return original name
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
