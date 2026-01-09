package systemd

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
)

// ServiceFilter defines a filter for matching systemd service names.
type ServiceFilter struct {
	Pattern  string
	IsPrefix bool // true = prefix match (e.g., "saltbox_managed_")
	IsExact  bool // true = exact match (e.g., "docker")
}

// ServiceInfo holds information about a systemd service.
type ServiceInfo struct {
	Name    string // Service name without .service suffix
	Active  string // active, inactive, failed
	Sub     string // running, dead, failed, exited, etc.
	Runtime string // e.g., "3d 2h" (only for active services)
}

// DefaultFilters are the default service filters used by MOTD and logs commands.
var DefaultFilters = []ServiceFilter{
	{Pattern: "saltbox_managed_", IsPrefix: true},
	{Pattern: "docker", IsExact: true},
}

// GetFilteredServices retrieves systemd services matching the given filters.
func GetFilteredServices(ctx context.Context, filters []ServiceFilter) ([]ServiceInfo, error) {
	serviceMap := make(map[string]ServiceInfo)

	// Build systemctl patterns for each filter
	for _, filter := range filters {
		var pattern string
		if filter.IsPrefix {
			pattern = filter.Pattern + "*"
		} else if filter.IsExact {
			pattern = filter.Pattern + ".service"
		} else {
			continue
		}

		result, err := executor.Run(ctx, "systemctl",
			executor.WithArgs("list-units", pattern, "--type=service", "--all", "--no-pager"),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list systemd services for pattern %s: %w", pattern, err)
		}

		parseSystemctlOutput(string(result.Combined), filter, serviceMap)
	}

	// Convert map to sorted slice
	services := make([]ServiceInfo, 0, len(serviceMap))
	for _, service := range serviceMap {
		services = append(services, service)
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	// Fetch runtime for active services
	for i := range services {
		if services[i].Active == "active" {
			runtime, err := GetServiceRuntime(ctx, services[i].Name)
			if err == nil {
				services[i].Runtime = runtime
			}
		}
	}

	return services, nil
}

// parseSystemctlOutput parses the output of systemctl list-units and adds matching services to the map.
func parseSystemctlOutput(output string, filter ServiceFilter, serviceMap map[string]ServiceInfo) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines, headers, footers
		if trimmed == "" || strings.HasPrefix(trimmed, "UNIT") ||
			strings.HasPrefix(trimmed, "Legend:") || strings.HasPrefix(trimmed, "LOAD") ||
			strings.Contains(line, "loaded units listed") ||
			strings.HasPrefix(trimmed, "To show all") {
			continue
		}

		// Extract service name and status (columns: UNIT LOAD ACTIVE SUB DESCRIPTION)
		// Lines may start with ● for failed services
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			fieldOffset := 0
			if fields[0] == "●" {
				fieldOffset = 1
			}
			if len(fields) < fieldOffset+4 {
				continue
			}
			serviceName := strings.TrimPrefix(fields[fieldOffset], "●")
			serviceName = strings.TrimSpace(serviceName)

			if !strings.HasSuffix(serviceName, ".service") {
				continue
			}

			serviceName = strings.TrimSuffix(serviceName, ".service")

			// Verify the service matches our filter criteria
			if !matchesFilter(serviceName, filter) {
				continue
			}

			serviceMap[serviceName] = ServiceInfo{
				Name:   serviceName,
				Active: fields[fieldOffset+2],
				Sub:    fields[fieldOffset+3],
			}
		}
	}
}

// matchesFilter checks if a service name matches the given filter.
func matchesFilter(serviceName string, filter ServiceFilter) bool {
	if filter.IsPrefix {
		return strings.HasPrefix(serviceName, filter.Pattern)
	}
	if filter.IsExact {
		return serviceName == filter.Pattern
	}
	return false
}

// GetServiceRuntime gets the runtime duration for an active service.
func GetServiceRuntime(ctx context.Context, serviceName string) (string, error) {
	serviceUnit := serviceName
	if !strings.HasSuffix(serviceUnit, ".service") {
		serviceUnit = serviceUnit + ".service"
	}

	result, err := executor.Run(ctx, "systemctl",
		executor.WithArgs("show", serviceUnit, "--property=ActiveEnterTimestamp", "--no-pager"),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(string(result.Combined))
	// Output format: ActiveEnterTimestamp=Mon 2025-11-10 10:11:29 CET
	parts := strings.SplitN(output, "=", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("no timestamp found")
	}

	timestampStr := parts[1]
	var startTime time.Time
	formats := []string{
		"Mon 2006-01-02 15:04:05 MST",
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339,
	}

	for _, format := range formats {
		t, err := time.Parse(format, timestampStr)
		if err == nil {
			startTime = t
			break
		}
	}

	if startTime.IsZero() {
		return "", fmt.Errorf("failed to parse timestamp: %s", timestampStr)
	}

	duration := time.Since(startTime)
	return FormatDuration(duration), nil
}

// FormatDuration formats a duration into a human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dd", days)
}

// FiltersWithAdditional returns the default filters plus additional exact-match services.
func FiltersWithAdditional(additionalServices []string) []ServiceFilter {
	filters := make([]ServiceFilter, len(DefaultFilters))
	copy(filters, DefaultFilters)

	for _, svc := range additionalServices {
		svc = strings.TrimSuffix(svc, ".service")
		if svc != "" {
			filters = append(filters, ServiceFilter{
				Pattern: svc,
				IsExact: true,
			})
		}
	}

	return filters
}
