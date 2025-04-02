package motd

import (
	"context"
)

// GetDistributionWithContext provides distribution info with context/timeout support
func GetDistributionWithContext(ctx context.Context) string {
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
func GetKernelWithContext(ctx context.Context) string {
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
func GetUptimeWithContext(ctx context.Context) string {
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
func GetCpuAveragesWithContext(ctx context.Context) string {
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
func GetLastLoginWithContext(ctx context.Context) string {
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
func GetUserSessionsWithContext(ctx context.Context) string {
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
func GetProcessCountWithContext(ctx context.Context) string {
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
func GetAptStatusWithContext(ctx context.Context) string {
	ch := make(chan string, 1)

	go func() {
		ch <- GetAptStatus()
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return "Package status info timed out"
	}
}

// GetRebootRequiredWithContext provides reboot status info with context/timeout support
func GetRebootRequiredWithContext(ctx context.Context) string {
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
func GetCpuInfoWithContext(ctx context.Context) string {
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

// GetMemoryInfoWithContext provides memory usage info with context/timeout support
func GetMemoryInfoWithContext(ctx context.Context) string {
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
func GetDockerInfoWithContext(ctx context.Context) string {
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
func GetDiskInfoWithContext(ctx context.Context) string {
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

// GetEmptyLineWithContext provides empty line with context/timeout support
func GetEmptyLineWithContext() string {
	return GetEmptyLine()
}
