package motd

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// GetDistributionWithContext provides distribution info with context/timeout support
func GetDistributionWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in distribution provider (%v)", r)
			}
		}()
		ch <- GetDistribution(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Distribution info timed out"
	}
}

// GetKernelWithContext provides kernel info with context/timeout support
func GetKernelWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in kernel provider (%v)", r)
			}
		}()
		ch <- GetKernel(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Kernel info timed out"
	}
}

// GetUptimeWithContext provides uptime info with context/timeout support
func GetUptimeWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in uptime provider (%v)", r)
			}
		}()
		ch <- GetUptime(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Uptime info timed out"
	}
}

// GetCpuAveragesWithContext provides CPU load info with context/timeout support
func GetCpuAveragesWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in CPU averages provider (%v)", r)
			}
		}()
		ch <- GetCpuAverages(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "CPU load info timed out"
	}
}

// GetLastLoginWithContext provides last login info with context/timeout support
func GetLastLoginWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in last login provider (%v)", r)
			}
		}()
		ch <- GetLastLogin(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Last login info timed out"
	}
}

// GetUserSessionsWithContext provides user sessions info with context/timeout support
func GetUserSessionsWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in user sessions provider (%v)", r)
			}
		}()
		ch <- GetUserSessions(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "User sessions info timed out"
	}
}

// GetProcessCountWithContext provides process count info with context/timeout support
func GetProcessCountWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in process count provider (%v)", r)
			}
		}()
		ch <- GetProcessCount(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Process count info timed out"
	}
}

// GetAptStatusWithContext provides package status info with context/timeout support
func GetAptStatusWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in apt status provider (%v)", r)
			}
		}()
		ch <- GetAptStatus(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Package status info timed out"
	}
}

// GetRebootRequiredWithContext provides reboot status info with context/timeout support
func GetRebootRequiredWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in reboot status provider (%v)", r)
			}
		}()
		ch <- GetRebootRequired(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Reboot status info timed out"
	}
}

// GetCpuInfoWithContext provides CPU info with context/timeout support
func GetCpuInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in CPU info provider (%v)", r)
			}
		}()
		ch <- GetCpuInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("CPU info timed out")
	}
}

// GetGpuInfoWithContext provides GPU info with context/timeout support
func GetGpuInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in GPU info provider (%v)", r)
			}
		}()
		ch <- GetGpuInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("GPU info timed out")
	}
}

// GetMemoryInfoWithContext provides memory usage info with context/timeout support
func GetMemoryInfoWithContext(ctx context.Context, _ bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in memory info provider (%v)", r)
			}
		}()
		ch <- GetMemoryInfo()
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("Memory information timed out")
	}
}

// GetDockerInfoWithContext provides Docker container info with context/timeout support
func GetDockerInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in Docker info provider (%v)", r)
			}
		}()
		ch <- GetDockerInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("Docker info timed out")
	}
}

// GetDiskInfoWithContext provides disk usage info with context/timeout support
func GetDiskInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Sprintf("Error: panic in disk info provider (%v)", r)
			}
		}()
		ch <- GetDiskInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("Disk information timed out")
	}
}

// GetQueueInfoWithContext provides queue info with context/timeout support
func GetQueueInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Queue info", GetQueueInfo)
}

func runSectionProvider(ctx context.Context, verbose bool, name string, provider func(context.Context, bool) string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Panic in %s provider: %v\n", strings.ToLower(name), r)
			}
			out = ErrorStyle.Render(fmt.Sprintf("%s provider panic: %v", name, r))
		}
	}()

	out = provider(ctx, verbose)
	if out == "" && ctx.Err() != nil {
		return ErrorStyle.Render(fmt.Sprintf("%s request ended early: %v", name, ctx.Err()))
	}
	return out
}

// GetPlexInfoWithContext provides Plex info with context/timeout support
func GetPlexInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Plex info", GetPlexInfo)
}

// GetEmbyInfoWithContext provides Emby info with context/timeout support
func GetEmbyInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Emby info", GetEmbyInfo)
}

// GetJellyfinInfoWithContext provides Jellyfin info with context/timeout support
func GetJellyfinInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Jellyfin info", GetJellyfinInfo)
}

// GetSabnzbdInfoWithContext provides Sabnzbd info with context/timeout support
func GetSabnzbdInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "SABnzbd info", GetSabnzbdInfo)
}

// GetNzbgetInfoWithContext provides NZBGet info with context/timeout support
func GetNzbgetInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "NZBGet info", GetNzbgetInfo)
}

// GetQbittorrentInfoWithContext provides qBittorrent info with context/timeout support
func GetQbittorrentInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "qBittorrent info", GetQbittorrentInfo)
}

// GetRtorrentInfoWithContext provides rTorrent info with context/timeout support
func GetRtorrentInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "rTorrent info", GetRtorrentInfo)
}

// GetTraefikInfoWithContext provides Traefik router status info with context/timeout support
func GetTraefikInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Traefik info", GetTraefikInfo)
}

// GetSystemdServicesInfoWithContext provides systemd services info with context/timeout support
func GetSystemdServicesInfoWithContext(ctx context.Context, verbose bool) string {
	return runSectionProvider(ctx, verbose, "Systemd services info", GetSystemdServicesInfo)
}
