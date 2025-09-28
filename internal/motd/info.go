package motd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	timepkg "time"

	"github.com/charmbracelet/lipgloss"
)

// GetDistribution returns the Ubuntu distribution version with a codename
func GetDistribution() string {
	distroInfo := ExecCommand("lsb_release", "-ds")
	codename := ExecCommand("lsb_release", "-cs")

	if codename != "" && distroInfo != "Not available" {
		distroInfo = distroInfo + " (" + codename + ")"
	}
	return distroInfo
}

// GetKernel returns the kernel version
func GetKernel() string {
	return ExecCommand("uname", "-r")
}

// GetUptime returns the system uptime
func GetUptime() string {
	uptimeInfo := ExecCommand("uptime", "-p")
	if uptimeInfo != "Not available" {
		// Remove "up " prefix from uptime
		uptimeInfo = strings.TrimPrefix(uptimeInfo, "up ")

		// Color the entire uptime string
		coloredUptimeInfo := ValueStyle.Render(uptimeInfo)
		return coloredUptimeInfo
	}
	return "Not available"
}

// GetCpuAverages returns the system load averages
func GetCpuAverages() string {
	// Try to read from /proc/loadavg first
	content, err := os.ReadFile("/proc/loadavg")
	if err == nil {
		fields := strings.Fields(string(content))
		if len(fields) >= 3 {
			return fmt.Sprintf("%s: %s | %s: %s | %s: %s",
				DefaultStyle.Render("1 min"), GreenStyle.Render(fields[0]),
				DefaultStyle.Render("5 min"), GreenStyle.Render(fields[1]),
				DefaultStyle.Render("15 min"), GreenStyle.Render(fields[2]),
			)
		}
	}

	// Fallback to uptime command if /proc/loadavg can't be read
	loadInfo := ExecCommand("uptime")
	if loadInfo != "Not available" {
		// Extract load averages from uptime output
		// Typical output: "... load average: 0.00, 0.01, 0.05"
		if idx := strings.Index(loadInfo, "load average:"); idx != -1 {
			loadPart := loadInfo[idx+14:]
			loads := strings.Split(loadPart, ", ")
			if len(loads) >= 3 {
				return fmt.Sprintf("%s: %s | %s: %s | %s: %s",
					DefaultStyle.Render("1 min"), GreenStyle.Render(strings.TrimSpace(loads[0])),
					DefaultStyle.Render("5 min"), GreenStyle.Render(strings.TrimSpace(loads[1])),
					DefaultStyle.Render("15 min"), GreenStyle.Render(strings.TrimSpace(loads[2])),
				)
			}
		}
	}

	return DefaultStyle.Render("Not available")
}

// GetLastLogin returns the last login information
func GetLastLogin() string {
	// Try the last command to get the most recent login
	lastOutput := ExecCommand("last", "-1")
	if lastOutput != "Not available" && lastOutput != "" {
		// First check if "still logged in" is present
		stillLoggedIn := strings.Contains(lastOutput, "still logged in")

		fields := strings.Fields(lastOutput)
		if len(fields) >= 5 {
			user := fields[0]
			// Color the username
			coloredUser := ValueStyle.Render(user)

			// Extract from IP
			fromIP := fields[2]

			// Initialize variables for the time components
			var day, month, date, time, logoutTime, duration string

			// Find the start of the date/time information
			timeIndex := 3 // Default position for date/time components

			// Check if the IP address actually contains an IP (sometimes it might not)
			if !strings.Contains(fromIP, ".") && !strings.Contains(fromIP, ":") {
				timeIndex = 2
				fromIP = "local"
			}

			// Color the IP address
			coloredIP := ValueStyle.Render(fromIP)

			// Extract date and time if we have enough fields
			if len(fields) >= timeIndex+4 {
				day = fields[timeIndex]
				month = fields[timeIndex+1]
				date = fields[timeIndex+2]
				time = fields[timeIndex+3]

				// Color the date/time components
				dateTimeStr := fmt.Sprintf("%s %s %s %s", day, month, date, time)
				coloredDateTime := ValueStyle.Render(dateTimeStr)

				// For entries that are still logged in
				if stillLoggedIn {
					return fmt.Sprintf("%s at %s (still logged in) from %s",
						coloredUser, coloredDateTime, coloredIP)
				}

				// For entries that have logged out, find the logout time and duration
				dashIndex := -1
				for i, field := range fields {
					if field == "-" {
						dashIndex = i
						break
					}
				}

				if dashIndex != -1 && dashIndex+1 < len(fields) {
					logoutTime = fields[dashIndex+1]
					// Color the logout time
					coloredLogoutTime := ValueStyle.Render(logoutTime)

					// Find the duration which is in parentheses
					for i, field := range fields {
						if strings.HasPrefix(field, "(") {
							// Start building duration from this field
							duration = strings.TrimPrefix(field, "(")

							// May need to join with the next field if it's a multipart duration
							if !strings.HasSuffix(duration, ")") && i+1 < len(fields) {
								duration += " " + strings.TrimSuffix(fields[i+1], ")")
							} else {
								duration = strings.TrimSuffix(duration, ")")
							}
							break
						}
					}

					return fmt.Sprintf("%s at %s until %s (%s) from %s",
						coloredUser, coloredDateTime, coloredLogoutTime, duration, coloredIP)
				}

				// If we couldn't parse the logout info but have login info
				return fmt.Sprintf("%s at %s from %s",
					coloredUser, coloredDateTime, coloredIP)
			}
		}
	}

	// Fallback methods similar to before...
	lastlogOutput := ExecCommand("lastlog", "-u", "root")
	if lastlogOutput != "Not available" && lastlogOutput != "" {
		lines := strings.Split(lastlogOutput, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				user := fields[0]
				// Color the username
				coloredUser := ValueStyle.Render(user)

				loginInfo := strings.Join(fields[3:], " ")
				// Color the login info (date/time/IP)
				coloredLoginInfo := ValueStyle.Render(loginInfo)

				return fmt.Sprintf("%s %s", coloredUser, coloredLoginInfo)
			}
		}
	}

	// Additional fallback with the who command
	whoOutput := ExecCommand("who", "-u")
	if whoOutput != "Not available" && whoOutput != "" {
		lines := strings.Split(whoOutput, "\n")
		if len(lines) > 0 {
			fields := strings.Fields(lines[0])
			if len(fields) >= 5 {
				user := fields[0]
				// Color the username
				coloredUser := ValueStyle.Render(user)

				month := fields[2]
				day := fields[3]
				time := fields[4]
				// Color the date/time components
				dateTimeStr := fmt.Sprintf("%s %s %s", month, day, time)
				coloredDateTime := ValueStyle.Render(dateTimeStr)

				return fmt.Sprintf("%s at %s (still logged in)", coloredUser, coloredDateTime)
			}
		}
	}

	return "No recent logins"
}

// GetUserSessions returns the number of active user sessions
func GetUserSessions() string {
	// Use the 'who' command to get user sessions
	whoOutput := ExecCommand("who")
	if whoOutput == "Not available" || whoOutput == "" {
		return "No active sessions"
	}

	lines := strings.Split(whoOutput, "\n")
	count := len(lines)

	// Color the session count
	coloredCount := ValueStyle.Render(fmt.Sprintf("%d", count))

	if count == 1 {
		return fmt.Sprintf("%s active session", coloredCount)
	} else {
		return fmt.Sprintf("%s active sessions", coloredCount)
	}
}

// GetProcessCount returns the number of running processes
func GetProcessCount() string {
	// Method 1: Count directories in /proc that are numeric
	entries, err := os.ReadDir("/proc")
	if err == nil {
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if the directory name is a number (process ID)
				if _, err := strconv.Atoi(entry.Name()); err == nil {
					count++
				}
			}
		}
		// Color the process count
		coloredCount := ValueStyle.Render(fmt.Sprintf("%d", count))
		return fmt.Sprintf("%s running processes", coloredCount)
	}

	// Method 2: Use ps command (fallback)
	psOutput := ExecCommand("ps", "ax")
	if psOutput != "Not available" {
		lines := strings.Split(psOutput, "\n")
		// Subtract 1 for the header line
		count := len(lines) - 1
		if count <= 0 {
			count = 0
		}
		// Color the process count
		coloredCount := ValueStyle.Render(fmt.Sprintf("%d", count))
		return fmt.Sprintf("%s running processes", coloredCount)
	}

	return "Not available"
}

// GetAptStatus returns the apt package status
func GetAptStatus(verbose bool) string {
	if verbose {
		fmt.Printf("DEBUG: Starting GetAptStatus\n")
	}

	// Check the updates-available file, which is updated by the daily apt update
	updatesFile := "/var/lib/update-notifier/updates-available"
	if verbose {
		fmt.Printf("DEBUG: Reading updates file: %s\n", updatesFile)
	}
	data, err := os.ReadFile(updatesFile)

	if err == nil && len(data) > 0 {
		if verbose {
			fmt.Printf("DEBUG: Successfully read updates file, parsing content (%d bytes)\n", len(data))
		}
		content := string(data)
		lines := strings.Split(content, "\n")

		// Look specifically for the line with "updates can be applied immediately"
		// that doesn't mention ESM
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if verbose {
				fmt.Printf("DEBUG: Parsing line: '%s'\n", trimmed)
			}
			if strings.Contains(trimmed, "can be applied immediately") &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") {
				if verbose {
					fmt.Printf("DEBUG: Found matching update line: '%s'\n", trimmed)
				}

				// Color the number of updates
				re := regexp.MustCompile(`(\d+)`)
				matches := re.FindStringSubmatch(trimmed)
				if len(matches) > 1 {
					// Get the number and color it
					number := matches[1]
					coloredNumber := ValueStyle.Render(number)

					// Replace the number in the original text
					coloredLine := re.ReplaceAllString(trimmed, coloredNumber)

					// Extract just the main update count message, removing any instruction text
					if idx := strings.Index(coloredLine, "."); idx != -1 {
						if verbose {
							fmt.Printf("DEBUG: Returning from updates file: '%s'\n", coloredLine[:idx+1])
						}
						return coloredLine[:idx+1] // Include the period
					}
					if verbose {
						fmt.Printf("DEBUG: Returning from updates file: '%s'\n", coloredLine)
					}
					return coloredLine
				}

				// If we can't extract the number, return the original text
				if idx := strings.Index(trimmed, "."); idx != -1 {
					if verbose {
						fmt.Printf("DEBUG: Returning from updates file (no number match): '%s'\n", trimmed[:idx+1])
					}
					return trimmed[:idx+1] // Include the period
				}
				if verbose {
					fmt.Printf("DEBUG: Returning from updates file (no period): '%s'\n", trimmed)
				}
				return trimmed
			}
		}

		if verbose {
			fmt.Printf("DEBUG: No immediate update lines found in file, checking for 'up to date' messages\n")
		}

		// If we found no updates but the file exists, check if it explicitly says that the system is up to date
		for _, line := range lines {
			if strings.Contains(line, "up to date") || strings.Contains(line, "Up to date") {
				return "System is up to date"
			}
		}
	}

	if verbose {
		fmt.Printf("DEBUG: Updates file not found or empty (%v), falling back to apt-check command\n", err)
	}

	// Fallback to apt-check command
	if verbose {
		fmt.Printf("DEBUG: Executing apt-check command: /usr/lib/update-notifier/apt-check --human-readable --no-esm-messages\n")
	}
	start := timepkg.Now()
	output := ExecCommand("/usr/lib/update-notifier/apt-check", "--human-readable", "--no-esm-messages")
	if verbose {
		fmt.Printf("DEBUG: apt-check command completed in %v\n", timepkg.Since(start))
	}
	if verbose {
		if output == "Not available" {
			fmt.Printf("DEBUG: apt-check command returned 'Not available'\n")
		} else if output == "" {
			fmt.Printf("DEBUG: apt-check command returned empty output\n")
		} else {
			fmt.Printf("DEBUG: apt-check command completed successfully, parsing output\n")
		}
	}
	if output != "Not available" && output != "" {
		lines := strings.Split(output, "\n")

		// First look for the regular update line
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "packages can be updated") &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") {

				// Color the number of updates
				re := regexp.MustCompile(`(\d+)`)
				matches := re.FindStringSubmatch(trimmed)
				if len(matches) > 1 {
					// Get the number and color it
					number := matches[1]
					coloredNumber := ValueStyle.Render(number)

					// Replace the number in the original text
					return re.ReplaceAllString(trimmed, coloredNumber)
				}

				return trimmed
			}
		}

		// If no specific update line found, use any non-ESM line that mentions updates
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" &&
				!strings.Contains(trimmed, "ESM") &&
				!strings.Contains(trimmed, "esm") &&
				(strings.Contains(trimmed, "update") || strings.Contains(trimmed, "package")) {

				// Color the number of updates if present
				re := regexp.MustCompile(`(\d+)`)
				matches := re.FindStringSubmatch(trimmed)
				if len(matches) > 1 {
					// Get the number and color it
					number := matches[1]
					coloredNumber := ValueStyle.Render(number)

					// Replace the number in the original text
					return re.ReplaceAllString(trimmed, coloredNumber)
				}

				return trimmed
			}
		}
	}

	// Additional fallback using apt list
	if verbose {
		fmt.Printf("DEBUG: apt-check failed, trying final fallback: apt list --upgradable\n")
	}
	start = timepkg.Now()
	output = ExecCommand("apt", "list", "--upgradable")
	if verbose {
		fmt.Printf("DEBUG: apt list --upgradable completed in %v\n", timepkg.Since(start))
	}
	if verbose {
		if output == "Not available" {
			fmt.Printf("DEBUG: apt list --upgradable returned 'Not available'\n")
		} else if output == "" {
			fmt.Printf("DEBUG: apt list --upgradable returned empty output\n")
		} else {
			fmt.Printf("DEBUG: apt list --upgradable completed successfully\n")
		}
	}
	if output != "Not available" && output != "" {
		lines := strings.Split(output, "\n")
		if len(lines) > 1 {
			updateCount := 0
			for _, line := range lines {
				if strings.Contains(line, "/") && !strings.Contains(line, "Listing") {
					updateCount++
				}
			}

			if updateCount > 0 {
				// Color the update count
				coloredCount := ValueStyle.Render(fmt.Sprintf("%d", updateCount))
				return fmt.Sprintf("%s updates can be applied immediately", coloredCount)
			}
		}
	}

	// If we can't determine the status, return a neutral message
	if verbose {
		fmt.Printf("DEBUG: All apt status methods failed, returning 'Update status unknown'\n")
	}
	return "Update status unknown"
}

// GetRebootRequired checks if a system reboot is required
// Returns empty string if no reboot is required, which will hide the field entirely
func GetRebootRequired() string {
	// Method 1: Go native implementation
	// Checks if the reboot-required file exists
	rebootFile := "/var/run/reboot-required"
	_, err := os.Stat(rebootFile)
	if err == nil {
		// File exists, a reboot is required

		// Try to get the specific packages requiring reboot
		pkgFile := "/var/run/reboot-required.pkgs"
		pkgData, err := os.ReadFile(pkgFile)
		if err == nil && len(pkgData) > 0 {
			pkgs := strings.Split(string(pkgData), "\n")
			// Filter out empty lines
			var validPkgs []string
			for _, pkg := range pkgs {
				if pkg != "" {
					validPkgs = append(validPkgs, pkg)
				}
			}

			if len(validPkgs) > 0 {
				if len(validPkgs) == 1 {
					// Use yellow for the entire message when reboot is required
					return YellowStyle.Render(fmt.Sprintf("Reboot required (package: %s)", validPkgs[0]))
				} else {
					// Use yellow for the entire message with package count
					return YellowStyle.Render(fmt.Sprintf("Reboot required (%d packages)", len(validPkgs)))
				}
			}
		}

		// If we couldn't get package details, just return that a reboot is required
		return YellowStyle.Render("Reboot required")
	}

	// Method 2: Fallback to the update-motd script
	output := ExecCommand("/usr/lib/update-notifier/update-motd-reboot-required")
	if output != "Not available" && output != "" && !strings.Contains(output, "No reboot") {
		// If the output contains "reboot required", color it yellow
		if strings.Contains(strings.ToLower(output), "reboot required") {
			return YellowStyle.Render(strings.TrimSpace(output))
		}
		return strings.TrimSpace(output)
	}

	// Return an empty string if no reboot is required
	return ""
}

// GetCpuInfo returns information about the CPU model and core count
func GetCpuInfo() string {
	// Try to read from /proc/cpuinfo
	content, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return DefaultStyle.Render("Not available")
	}

	// Parse the cpuinfo content
	cpuInfo := string(content)
	lines := strings.Split(cpuInfo, "\n")

	modelName := ""
	cpuCores := 0
	physicalIds := make(map[string]bool)

	for _, line := range lines {
		// Extract CPU model name
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				modelName = strings.TrimSpace(parts[1])
			}
		}

		// Count physical cores by looking at "processor" entries
		if strings.HasPrefix(line, "processor") {
			cpuCores++
		}

		// Track physical IDs to count actual CPUs
		if strings.HasPrefix(line, "physical id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 2 {
				physicalId := strings.TrimSpace(parts[1])
				physicalIds[physicalId] = true
			}
		}
	}

	// Calculate the number of physical CPUs
	numPhysicalCPUs := len(physicalIds)
	if numPhysicalCPUs == 0 {
		numPhysicalCPUs = 1 // Default to 1 if we can't determine
	}

	// Calculate threads per core (logical cores / physical cores)
	threadsPerCore := cpuCores
	if numPhysicalCPUs > 0 && cpuCores > 0 {
		threadsPerCore = cpuCores / numPhysicalCPUs
	}

	// Format the result
	if modelName != "" {
		// Use colored values for the numbers
		cpuCoresStr := ValueStyle.Render(fmt.Sprintf("%d", cpuCores))
		if numPhysicalCPUs > 1 || threadsPerCore > 1 {
			// Show more detailed info if we have multiple physical CPUs or threads
			return fmt.Sprintf("%s (%s cores, %s CPUs)",
				DefaultStyle.Render(modelName),
				cpuCoresStr,
				ValueStyle.Render(fmt.Sprintf("%d", numPhysicalCPUs)))
		} else {
			// Simple output for a single CPU
			return fmt.Sprintf("%s (%s cores)",
				DefaultStyle.Render(modelName),
				cpuCoresStr)
		}
	}

	// Fallback if we couldn't parse model name but have core count
	if cpuCores > 0 {
		return fmt.Sprintf("%s cores", ValueStyle.Render(fmt.Sprintf("%d", cpuCores)))
	}

	// Fallback to lscpu if we couldn't parse /proc/cpuinfo
	lscpuOutput := ExecCommand("lscpu")
	if lscpuOutput != "Not available" {
		lines := strings.Split(lscpuOutput, "\n")
		modelLine := ""
		coresLine := ""

		for _, line := range lines {
			if strings.HasPrefix(line, "Model name:") {
				modelLine = line
			}
			if strings.HasPrefix(line, "CPU(s):") {
				coresLine = line
			}
		}

		if modelLine != "" && coresLine != "" {
			modelParts := strings.SplitN(modelLine, ":", 2)
			coresParts := strings.SplitN(coresLine, ":", 2)

			if len(modelParts) >= 2 && len(coresParts) >= 2 {
				model := strings.TrimSpace(modelParts[1])
				cores := strings.TrimSpace(coresParts[1])

				return fmt.Sprintf("%s (%s cores)",
					DefaultStyle.Render(model),
					ValueStyle.Render(cores))
			}
		}
	}

	return DefaultStyle.Render("Not available")
}

// GetGpuInfo returns information about the GPU(s) in the system
func GetGpuInfo() string {
	var gpus []string

	// List of GPU vendors/models to exclude (IPMI, server management, etc.)
	excludedGPUs := []string{
		"ASPEED",         // ASPEED BMC/IPMI controllers
		"Matrox MGA",     // Matrox G200/G400 series (server management)
		"Cirrus Logic",   // Cirrus Logic CL-GD series (legacy/server)
		"XGI",            // XGI Volari series (legacy)
		"Silicon Motion", // SM750/SM712 (embedded/server)
		"Hisilicon",      // HiSilicon Hi171x series (server BMC)
		"ServerEngines",  // ServerEngines Pilot series
		"Nuvoton",        // Nuvoton WPCM450 (server management)
		"Pilot",          // Pilot series BMC controllers
	}

	// Use lspci to detect GPUs (works for NVIDIA, AMD, Intel, etc.)
	lspciOutput := ExecCommand("lspci")
	if lspciOutput != "Not available" {
		lines := strings.Split(lspciOutput, "\n")
		for _, line := range lines {
			// Look for VGA compatible controller or 3D controller
			if strings.Contains(line, "VGA compatible controller") ||
				strings.Contains(line, "3D controller") ||
				strings.Contains(line, "Display controller") {
				// Extract GPU name after the colon, skipping the PCI address
				parts := strings.SplitN(line, ":", 3)
				var gpuInfo string

				if len(parts) >= 3 {
					// Skip the PCI address (first part) and device type (second part)
					gpuInfo = strings.TrimSpace(parts[2])
				} else if len(parts) >= 2 {
					// Fallback: clean up device type descriptors from the second part
					gpuInfo = strings.TrimSpace(parts[1])
					gpuInfo = strings.ReplaceAll(gpuInfo, "VGA compatible controller:", "")
					gpuInfo = strings.ReplaceAll(gpuInfo, "3D controller:", "")
					gpuInfo = strings.ReplaceAll(gpuInfo, "Display controller:", "")
					gpuInfo = strings.TrimSpace(gpuInfo)
				}

				if gpuInfo != "" {
					// Check if this GPU should be excluded
					shouldExclude := false
					for _, excluded := range excludedGPUs {
						if strings.Contains(strings.ToUpper(gpuInfo), strings.ToUpper(excluded)) {
							shouldExclude = true
							break
						}
					}

					if !shouldExclude {
						gpus = append(gpus, DefaultStyle.Render(gpuInfo))
					}
				}
			}
		}
	}

	// Return empty string if no GPUs found to hide the section
	if len(gpus) == 0 {
		return ""
	}

	if len(gpus) == 1 {
		return gpus[0]
	}

	// Multiple GPUs - show each on a clean line
	return strings.Join(gpus, "\n")
}

// GetMemoryInfo returns the system memory usage in a simple text format
func GetMemoryInfo() string {
	// Try to read from /proc/meminfo first
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return DefaultStyle.Render("Not available")
	}

	// Parse the meminfo content
	memInfo := make(map[string]uint64)
	lines := strings.Split(string(content), "\n")

	re := regexp.MustCompile(`^(\S+):\s+(\d+)`)
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			key := matches[1]
			valueStr := matches[2]
			value, err := strconv.ParseUint(valueStr, 10, 64)
			if err == nil {
				memInfo[key] = value
			}
		}
	}

	// Check if we have the required fields
	if memTotal, ok := memInfo["MemTotal"]; ok {
		var memAvailable, memFree, memCached uint64
		memUsed := memTotal // Start with the total, then subtract

		if free, ok := memInfo["MemFree"]; ok {
			memFree = free
			memUsed -= free
		}

		if cached, ok := memInfo["Cached"]; ok {
			memCached = cached
		}

		if buffers, ok := memInfo["Buffers"]; ok {
			memCached += buffers
		}

		if avail, ok := memInfo["MemAvailable"]; ok {
			memAvailable = avail
		} else {
			// Fallback calculation if MemAvailable is not present
			memAvailable = memFree + memCached
		}

		// Convert all values to GB for consistent formatting
		totalGB := float64(memTotal) / 1024.0 / 1024.0
		usedGB := float64(memTotal-memAvailable) / 1024.0 / 1024.0
		freeGB := float64(memFree) / 1024.0 / 1024.0
		cachedGB := float64(memCached) / 1024.0 / 1024.0
		availableGB := float64(memAvailable) / 1024.0 / 1024.0

		// Format as a simple text string
		return fmt.Sprintf("%s used, %s free, %s cached, %s available, %s total",
			ValueStyle.Render(fmt.Sprintf("%.1fG", usedGB)),
			ValueStyle.Render(fmt.Sprintf("%.1fG", freeGB)),
			ValueStyle.Render(fmt.Sprintf("%.1fG", cachedGB)),
			ValueStyle.Render(fmt.Sprintf("%.1fG", availableGB)),
			ValueStyle.Render(fmt.Sprintf("%.1fG", totalGB)))
	}

	return DefaultStyle.Render("Not available")
}

// GetDockerInfo returns information about Docker containers
func GetDockerInfo() string {
	var output strings.Builder

	// Check if Docker service is running
	statusOutput := ExecCommand("systemctl", "is-active", "docker")
	if statusOutput != "active" {
		// Check if Docker is installed but not running
		installedCheck := ExecCommand("which", "docker")
		if installedCheck != "Not available" {
			return DefaultStyle.Render("Docker is installed but not running")
		}
		return DefaultStyle.Render("Docker is not installed or not detected")
	}

	// Get container list with detailed format
	containerOutput := ExecCommand("docker", "ps", "-a", "--format", "{{.Names}}|{{.Status}}|{{.State}}")
	if containerOutput == "Not available" || containerOutput == "" {
		return DefaultStyle.Render("Docker is running but no containers found")
	}

	containerLines := strings.Split(containerOutput, "\n")
	if len(containerLines) == 0 || (len(containerLines) == 1 && containerLines[0] == "") {
		return DefaultStyle.Render("Docker is running but no containers found")
	}

	// Process container statuses
	var problemContainers []string
	runningCount := 0
	totalCount := 0

	// Sort containers alphabetically by name
	sort.Strings(containerLines)

	for _, line := range containerLines {
		if line == "" {
			continue
		}

		totalCount++

		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		statusText := parts[1]
		stateText := parts[2]

		// Determine the container state based on the state field directly
		isProblematic := false
		status := ""

		// Use the State field (3rd column) to determine the basic state
		switch stateText {
		case "running":
			runningCount++
			// Check for health status
			if strings.Contains(statusText, "unhealthy") {
				isProblematic = true
				status = "running (unhealthy)"
			} else if strings.Contains(statusText, "healthy") {
				status = "running (healthy)"
			} else {
				status = "running"
			}
		case "restarting":
			isProblematic = true
			status = "restarting"
		case "exited":
			// Extract exit code for stopped containers
			exitCodeMatch := regexp.MustCompile(`Exited \((\d+)\)`).FindStringSubmatch(statusText)
			if len(exitCodeMatch) > 1 && exitCodeMatch[1] != "0" {
				isProblematic = true
				status = fmt.Sprintf("stopped (error: %s)", exitCodeMatch[1])
			} else {
				status = "stopped"
				isProblematic = true // Consider stopped containers as problematic
			}
		case "created":
			isProblematic = true
			status = "created"
		case "paused":
			isProblematic = true
			status = "paused"
		case "dead":
			isProblematic = true
			status = "dead"
		default:
			// For any other state, consider it problematic
			isProblematic = true
			status = stateText // Use the raw state
		}

		// Only add problematic containers to the result
		if isProblematic {
			// Use DefaultStyle for the container name and the specific status style
			formattedLine := fmt.Sprintf("%s: %s", DefaultStyle.Render(name), RedStyle.Render(status))
			problemContainers = append(problemContainers, formattedLine)
		}
	}

	// Create a simple summary line - always show total and running
	if len(problemContainers) > 0 {
		// Color the counts - yellow for totalCount when there are issues
		coloredTotalCount := YellowStyle.Render(fmt.Sprintf("%d", totalCount))
		coloredRunningCount := ValueStyle.Render(fmt.Sprintf("%d", runningCount))
		coloredProblemCount := YellowStyle.Render(fmt.Sprintf("%d", len(problemContainers)))

		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%s containers (%s running, %s need attention)",
			coloredTotalCount, coloredRunningCount, coloredProblemCount)))
	} else {
		// Color counts - normal ValueStyle when all is good
		coloredTotalCount := ValueStyle.Render(fmt.Sprintf("%d", totalCount))
		coloredRunningCount := ValueStyle.Render(fmt.Sprintf("%d", runningCount))

		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%s containers (%s running)",
			coloredTotalCount, coloredRunningCount)))
	}

	// If there are problematic containers, add them to the output
	if len(problemContainers) > 0 {
		for _, container := range problemContainers {
			output.WriteString(fmt.Sprintf("\n%s", container))
		}
	}

	return output.String()
}

// GetDiskInfo returns the disk usage for all real partitions with visual bars
func GetDiskInfo() string {
	var output strings.Builder

	// Constants for disk usage bar
	const (
		maxUsageThreshold = 90 // Percentage at which disk usage is considered high
		barWidth          = 50 // Width of the usage bar in characters
	)

	// Run df command to get disk usage with the proper exclusions
	dfOutput := ExecCommand("df", "-H", "-x", "tmpfs", "-x", "overlay", "-x", "fuse.mergerfs", "-x", "fuse.rclone",
		"--output=target,pcent,size")
	if dfOutput == "Not available" {
		return DefaultStyle.Render("Not available")
	}

	// Process df output
	lines := strings.Split(dfOutput, "\n")
	if len(lines) <= 1 { // If there's only one line (the header), then no valid partitions
		return DefaultStyle.Render("No valid disk partitions found")
	}

	// Skip the header line
	lines = lines[1:]

	type partitionInfo struct {
		mountPoint   string
		usagePercent int
		size         string
		formattedBar string
		percentStyle lipgloss.Style // Store the style directly with each partition
	}

	var partitions []partitionInfo

	// Process each partition
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Skip specific mount points
		mountPoint := fields[0]
		if strings.HasPrefix(mountPoint, "/dev") ||
			strings.HasPrefix(mountPoint, "/sys") ||
			strings.HasPrefix(mountPoint, "/proc") ||
			strings.HasPrefix(mountPoint, "/run") {
			continue
		}

		// Get percentage (remove the '%' character)
		usagePercentStr := strings.TrimSuffix(fields[1], "%")
		usagePercent, err := strconv.Atoi(usagePercentStr)
		if err != nil {
			continue
		}

		// Get size
		size := fields[2]

		// Calculate the bar
		usedWidth := (usagePercent * barWidth) / 100
		unusedWidth := barWidth - usedWidth

		// Choose a color based on a usage threshold
		var usedBarStyle lipgloss.Style
		var percentStyle lipgloss.Style

		if usagePercent >= maxUsageThreshold {
			usedBarStyle = RedStyle
			percentStyle = RedStyle
		} else {
			usedBarStyle = GreenStyle
			percentStyle = ValueStyle
		}

		// Create the usage bar
		usedBar := strings.Repeat("=", usedWidth)
		unusedBar := strings.Repeat("=", unusedWidth)

		// Style the bars
		styledUsedBar := usedBarStyle.Render(usedBar)
		styledUnusedBar := DimStyle.Render(unusedBar)

		// Create the complete bar
		completeBar := fmt.Sprintf("[%s%s]", styledUsedBar, styledUnusedBar)

		// Add to partition slice
		partitions = append(partitions, partitionInfo{
			mountPoint:   mountPoint,
			usagePercent: usagePercent,
			size:         size,
			formattedBar: completeBar,
			percentStyle: percentStyle,
		})
	}

	if len(partitions) == 0 {
		return DefaultStyle.Render("No valid disk partitions found")
	}

	// Format the results
	for i, p := range partitions {
		// Format the percentage and size first while preserving alignment
		percentStr := fmt.Sprintf("%3d%%", p.usagePercent)
		sizeStr := fmt.Sprintf("%4s", p.size)

		// Then color them
		coloredPercent := p.percentStyle.Render(percentStr)
		coloredSize := ValueStyle.Render(sizeStr)

		// For the first partition, add it directly to the output
		if i == 0 {
			// Format using the original format with wide fixed spacing and mountpoint
			infoLine := fmt.Sprintf("%-30s%s used out of %s", p.mountPoint, coloredPercent, coloredSize)
			output.WriteString(DefaultStyle.Render(infoLine))
			output.WriteString(fmt.Sprintf("\n%s", p.formattedBar))
		} else {
			// For later partitions, add line breaks before
			infoLine := fmt.Sprintf("%-30s%s used out of %s", p.mountPoint, coloredPercent, coloredSize)
			output.WriteString(fmt.Sprintf("\n%s", DefaultStyle.Render(infoLine)))
			output.WriteString(fmt.Sprintf("\n%s", p.formattedBar))
		}
	}

	return output.String()
}

// GetTraefikInfo returns information about Traefik router status
func GetTraefikInfo() string {
	var output strings.Builder

	// Check if Docker service is running
	statusOutput := ExecCommand("systemctl", "is-active", "docker")
	if statusOutput != "active" {
		return DefaultStyle.Render("Docker service is not running")
	}

	// Check if Traefik container is running
	containerStatus := ExecCommand("docker", "ps", "--filter", "name=^traefik$", "--format", "{{.Names}}")
	if containerStatus == "Not available" || containerStatus == "" {
		return DefaultStyle.Render("Traefik container is not running")
	}

	// Check if Traefik API is accessible
	routersOutput := ExecCommand("curl", "-s", "--connect-timeout", "3", "http://traefik:8080/api/http/routers")
	if routersOutput == "Not available" || strings.Contains(routersOutput, "Connection refused") || strings.Contains(routersOutput, "curl:") {
		return DefaultStyle.Render("Traefik container is running but API is not accessible")
	}

	// If we get here, the API call succeeded, but check if it's valid JSON
	if strings.TrimSpace(routersOutput) == "" || routersOutput == "[]" {
		return DefaultStyle.Render("Traefik is running with no routers configured")
	}

	// Parse JSON properly
	type Router struct {
		Name   string   `json:"name"`
		Status string   `json:"status"`
		Error  []string `json:"error,omitempty"`
	}

	var routers []Router
	if err := json.Unmarshal([]byte(routersOutput), &routers); err != nil {
		return DefaultStyle.Render("Failed to parse Traefik router response")
	}

	totalRouters := len(routers)
	if totalRouters == 0 {
		return DefaultStyle.Render("Traefik is running with no routers configured")
	}

	var problemRouters []string
	healthyRouters := 0

	for _, router := range routers {
		if len(router.Error) > 0 {
			problemRouters = append(problemRouters, fmt.Sprintf("%s: %s",
				DefaultStyle.Render(router.Name),
				RedStyle.Render(router.Error[0])))
		} else if router.Status == "disabled" {
			problemRouters = append(problemRouters, fmt.Sprintf("%s: %s",
				DefaultStyle.Render(router.Name),
				RedStyle.Render("router is disabled")))
		} else {
			healthyRouters++
		}
	}

	// Create a summary line
	if len(problemRouters) > 0 {
		coloredTotalCount := YellowStyle.Render(fmt.Sprintf("%d", totalRouters))
		coloredHealthyCount := ValueStyle.Render(fmt.Sprintf("%d", healthyRouters))
		coloredProblemCount := YellowStyle.Render(fmt.Sprintf("%d", len(problemRouters)))

		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%s routers (%s active, %s need attention)",
			coloredTotalCount, coloredHealthyCount, coloredProblemCount)))

		// Add each problematic router on its own line
		for _, problem := range problemRouters {
			output.WriteString(fmt.Sprintf("\n%s", problem))
		}
	} else {
		// All routers are healthy
		coloredTotalCount := ValueStyle.Render(fmt.Sprintf("%d", totalRouters))
		coloredHealthyCount := ValueStyle.Render(fmt.Sprintf("%d", healthyRouters))

		output.WriteString(DefaultStyle.Render(fmt.Sprintf("%s routers (%s active)",
			coloredTotalCount, coloredHealthyCount)))
	}

	return output.String()
}
