package motd

import (
	"context"
)

// GetDistributionWithContext provides distribution info with context/timeout support
func GetDistributionWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetDistribution()
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
		ch <- GetKernel()
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
		ch <- GetUptime()
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
		ch <- GetCpuAverages()
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
		ch <- GetLastLogin()
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
		ch <- GetUserSessions()
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
		ch <- GetProcessCount()
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
		ch <- GetAptStatus(verbose)
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
		ch <- GetRebootRequired()
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
		ch <- GetCpuInfo()
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
		ch <- GetGpuInfo()
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return DefaultStyle.Render("GPU info timed out")
	}
}

// GetMemoryInfoWithContext provides memory usage info with context/timeout support
func GetMemoryInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
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
		ch <- GetDockerInfo()
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
		ch <- GetDiskInfo()
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
	ch := make(chan string, 1)

	go func() {
		ch <- GetQueueInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetPlexInfoWithContext provides Plex info with context/timeout support
func GetPlexInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetPlexInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetEmbyInfoWithContext provides Emby info with context/timeout support
func GetEmbyInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetEmbyInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetJellyfinInfoWithContext provides Jellyfin info with context/timeout support
func GetJellyfinInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetJellyfinInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetSabnzbdInfoWithContext provides Sabnzbd info with context/timeout support
func GetSabnzbdInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetSabnzbdInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetNzbgetInfoWithContext provides NZBGet info with context/timeout support
func GetNzbgetInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetNzbgetInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetQbittorrentInfoWithContext provides qBittorrent info with context/timeout support
func GetQbittorrentInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetQbittorrentInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetRtorrentInfoWithContext provides rTorrent info with context/timeout support
func GetRtorrentInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetRtorrentInfo(verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetTraefikInfoWithContext provides Traefik router status info with context/timeout support
func GetTraefikInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetTraefikInfo()
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}
