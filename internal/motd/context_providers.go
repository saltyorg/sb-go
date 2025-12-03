package motd

import (
	"context"
	"fmt"
	"os"
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
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in queue info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in Plex info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in Emby info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in Jellyfin info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in Sabnzbd info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in NZBGet info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in qBittorrent info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in rTorrent info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in Traefik info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
		ch <- GetTraefikInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}

// GetSystemdServicesInfoWithContext provides systemd services info with context/timeout support
func GetSystemdServicesInfoWithContext(ctx context.Context, verbose bool) string {
	ch := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Panic in systemd services info provider: %v\n", r)
				}
				ch <- "" // Return empty on panic to hide this section
			}
		}()
		ch <- GetSystemdServicesInfo(ctx, verbose)
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "" // Return an empty string on timeout to hide this section
	}
}
