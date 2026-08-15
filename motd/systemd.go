package motd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
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
	configPath := layout.SaltboxMOTDConfigPath

	if _, err := os.Stat(configPath); err == nil {
		cfg, err := LoadConfig(configPath)
		if err == nil && cfg.Systemd != nil {
			// Check if section is disabled
			if !cfg.Systemd.IsEnabled() {
				return ""
			}
			additionalServices = cfg.Systemd.AdditionalServices
			userDisplayNames = cfg.Systemd.DisplayNames
		}
	}

	filters := host.FiltersWithAdditional(additionalServices)

	services, err := host.GetFilteredServices(ctx, filters)
	if err != nil {
		return ErrorStyle.Render(formatProviderError(fmt.Errorf("error getting services: %w", err)))
	}

	if len(services) == 0 {
		return ""
	}

	// Build display names and create index for sorting
	type serviceWithDisplay struct {
		service     host.ServiceInfo
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
	// For unmapped saltbox_managed_ services, strip the prefix for a cleaner display
	if after, ok := strings.CutPrefix(name, "saltbox_managed_"); ok {
		return after
	}
	// No mapping found, return original name
	return name
}

// formatServiceLine formats a single service line with status and runtime.
func formatServiceLine(svc host.ServiceInfo, displayName string, maxNameLen int) string {
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
		if svc.TimerActive != "" {
			timerState := svc.TimerActive
			if svc.TimerSub != "" && svc.TimerSub != svc.TimerActive {
				timerState = fmt.Sprintf("%s/%s", svc.TimerActive, svc.TimerSub)
			}

			status := fmt.Sprintf("scheduled • timer %s", timerState)
			if svc.TimerActive != "active" {
				status = fmt.Sprintf("inactive • timer %s", timerState)
			}
			if svc.TimerNextIn != "" {
				status = fmt.Sprintf("%s • next in %s", status, svc.TimerNextIn)
			}
			if svc.TimerActive == "active" {
				statusStr = SuccessStyle.Render(status)
			} else {
				statusStr = WarningStyle.Render(status)
			}
			break
		}

		statusStr = WarningStyle.Render("inactive")
	default:
		statusStr = WarningStyle.Render(svc.Active)
	}

	return fmt.Sprintf("%s   %s", DefaultStyle.Render(paddedName), statusStr)
}
