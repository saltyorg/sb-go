package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/host"
)

const (
	legacyDeadsnakesPythonMinor      = "3.12"
	aptSourcesDirectory              = "/etc/apt/sources.list.d"
	deadsnakesLaunchpadContentMarker = "ppa.launchpadcontent.net/deadsnakes/ppa"
	deadsnakesLegacyLaunchpadMarker  = "ppa.launchpad.net/deadsnakes/ppa"
)

type installedPackage struct {
	name    string
	version string
}

// installedDeadsnakesPackages returns fully installed legacy Python packages
// identified by their apt origin or, after source removal, deadsnakes' legacy
// Ubuntu package-version suffix.
func installedDeadsnakesPackages(ctx context.Context) ([]string, error) {
	result, err := executor.Run(ctx, "dpkg-query",
		executor.WithArgs("-W", "-f=${binary:Package}\t${source:Package}\t${db:Status-Abbrev}\t${Version}\n"),
		executor.WithOutputMode(executor.OutputModeCapture))
	if err != nil {
		return nil, fmt.Errorf("query installed packages: %w", err)
	}

	candidates, err := parseInstalledLegacyPythonPackages(string(result.Stdout))
	if err != nil {
		return nil, fmt.Errorf("parse installed packages: %w", err)
	}

	packages := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		fromDeadsnakes, err := installedVersionHasDeadsnakesOrigin(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if fromDeadsnakes {
			packages = append(packages, candidate.name)
		}
	}
	return packages, nil
}

func parseInstalledLegacyPythonPackages(output string) ([]installedPackage, error) {
	var packages []installedPackage
	for lineNumber, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) != 4 {
			return nil, fmt.Errorf("line %d: expected package, source, status, and version fields", lineNumber+1)
		}

		binaryPackage := strings.TrimSpace(fields[0])
		sourcePackage := strings.TrimSpace(fields[1])
		status := strings.TrimSpace(fields[2])
		version := strings.TrimSpace(fields[3])
		if binaryPackage == "" {
			return nil, fmt.Errorf("line %d: package name is empty", lineNumber+1)
		}
		if status == "ii" && version == "" {
			return nil, fmt.Errorf("line %d: installed package version is empty", lineNumber+1)
		}
		if status == "ii" && isLegacyDeadsnakesPackage(binaryPackage, sourcePackage) {
			packages = append(packages, installedPackage{name: binaryPackage, version: version})
		}
	}

	sort.Slice(packages, func(i, j int) bool { return packages[i].name < packages[j].name })
	return packages, nil
}

func installedVersionHasDeadsnakesOrigin(ctx context.Context, pkg installedPackage) (bool, error) {
	result, err := executor.Run(ctx, "apt-cache",
		executor.WithArgs("policy", pkg.name),
		executor.WithOutputMode(executor.OutputModeCapture))
	if err != nil {
		return false, fmt.Errorf("query apt origin for %s: %w", pkg.name, err)
	}

	hasDeadsnakesOrigin, hasOtherOrigin, found := policyVersionOrigins(string(result.Stdout), pkg.version)
	if !found {
		return false, fmt.Errorf("apt policy for %s does not describe installed version %s", pkg.name, pkg.version)
	}
	if hasOtherOrigin {
		return false, nil
	}
	return hasDeadsnakesOrigin || hasLegacyDeadsnakesVersionSuffix(pkg.version), nil
}

func policyVersionOrigins(output, installedVersion string) (bool, bool, bool) {
	lines := strings.Split(output, "\n")
	installedLine := -1
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "***" && fields[1] == installedVersion {
			installedLine = index
			break
		}
	}
	if installedLine == -1 {
		return false, false, false
	}

	hasDeadsnakesOrigin := false
	hasOtherOrigin := false
	for _, line := range lines[installedLine+1:] {
		if strings.HasPrefix(line, "     ") && !strings.HasPrefix(line, "        ") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}

		origin := strings.ToLower(strings.Join(fields[1:], " "))
		if strings.Contains(origin, "/var/lib/dpkg/status") {
			continue
		}
		if containsDeadsnakesRepository(origin) {
			hasDeadsnakesOrigin = true
		} else {
			hasOtherOrigin = true
		}
	}
	return hasDeadsnakesOrigin, hasOtherOrigin, true
}

func hasLegacyDeadsnakesVersionSuffix(version string) bool {
	for _, marker := range []string{"+focal", "+jammy"} {
		markerIndex := strings.LastIndex(version, marker)
		if markerIndex == -1 {
			continue
		}

		revision := version[markerIndex+len(marker):]
		if revision == "" {
			return false
		}
		for _, character := range revision {
			if character < '0' || character > '9' {
				return false
			}
		}
		return true
	}
	return false
}

func containsDeadsnakesRepository(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, deadsnakesLaunchpadContentMarker) ||
		strings.Contains(value, deadsnakesLegacyLaunchpadMarker)
}

func isLegacyDeadsnakesPackage(binaryPackage, sourcePackage string) bool {
	if sourcePackage == "python"+legacyDeadsnakesPythonMinor {
		return true
	}

	packageWithoutArchitecture, _, _ := strings.Cut(binaryPackage, ":")
	pythonPrefix := "python" + legacyDeadsnakesPythonMinor
	libpythonPrefix := "libpython" + legacyDeadsnakesPythonMinor
	idlePrefix := "idle-python" + legacyDeadsnakesPythonMinor

	return packageWithoutArchitecture == pythonPrefix ||
		strings.HasPrefix(packageWithoutArchitecture, pythonPrefix+"-") ||
		strings.HasPrefix(packageWithoutArchitecture, libpythonPrefix) ||
		packageWithoutArchitecture == idlePrefix ||
		strings.HasPrefix(packageWithoutArchitecture, idlePrefix+"-")
}

func findDeadsnakesRepositoryFiles(sourcesDirectory string) ([]string, error) {
	entries, err := os.ReadDir(sourcesDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", sourcesDirectory, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(sourcesDirectory, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read apt source %s: %w", path, err)
		}
		if containsDeadsnakesRepository(string(content)) {
			files = append(files, path)
		}
	}

	sort.Strings(files)
	return files, nil
}

// RemoveDeadsnakesRepositories removes apt source files containing deadsnakes
// entries and refreshes the apt package lists when any files were removed.
func RemoveDeadsnakesRepositories(ctx context.Context, verbose bool) (bool, error) {
	return removeDeadsnakesRepositories(
		aptSourcesDirectory,
		verbose,
		host.UpdatePackageLists(ctx, verbose),
	)
}

func removeDeadsnakesRepositories(
	sourcesDirectory string,
	verbose bool,
	updatePackageLists func() error,
) (bool, error) {
	files, err := findDeadsnakesRepositoryFiles(sourcesDirectory)
	if err != nil {
		return false, err
	}
	if len(files) == 0 {
		return false, nil
	}

	for _, path := range files {
		if verbose {
			fmt.Printf("Removing deadsnakes repository file: %s\n", path)
		}
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("remove %s: %w", path, err)
		}
	}

	if verbose {
		fmt.Printf("Removed %d deadsnakes repository file(s), running apt update...\n", len(files))
	}
	if err := updatePackageLists(); err != nil {
		return true, fmt.Errorf("update apt package lists after removing deadsnakes repositories: %w", err)
	}

	remainingFiles, err := findDeadsnakesRepositoryFiles(sourcesDirectory)
	if err != nil {
		return true, fmt.Errorf("verify deadsnakes repository removal: %w", err)
	}
	if len(remainingFiles) != 0 {
		return true, fmt.Errorf("deadsnakes repository files remain after cleanup: %s", strings.Join(remainingFiles, ", "))
	}

	return true, nil
}

func removeDeadsnakesPackages(ctx context.Context, packages []string, verbose bool) error {
	return removeDeadsnakesPackagesWith(
		ctx,
		packages,
		verbose,
		host.WaitForAptLock,
		host.RunAptGet,
	)
}

func removeDeadsnakesPackagesWith(
	ctx context.Context,
	packages []string,
	verbose bool,
	waitForAptLock func(context.Context, bool) error,
	runAptGet func(context.Context, []string, bool) error,
) error {
	if len(packages) == 0 {
		return nil
	}

	if err := waitForAptLock(ctx, verbose); err != nil {
		return fmt.Errorf("wait for apt lock: %w", err)
	}

	args := append([]string{"remove", "-y"}, packages...)
	if verbose {
		fmt.Printf("Running command: apt-get %s\n", strings.Join(args, " "))
	}
	if err := runAptGet(ctx, args, verbose); err != nil {
		return fmt.Errorf("remove deadsnakes Python packages: %w", err)
	}

	if verbose {
		fmt.Println("Running command: apt-get autoremove -y")
	}
	if err := runAptGet(ctx, []string{"autoremove", "-y"}, verbose); err != nil {
		return fmt.Errorf("autoremove deadsnakes Python dependencies: %w", err)
	}

	return nil
}

// ShouldCleanupDeadsnakes reports whether this Ubuntu release may have the
// legacy deadsnakes Python installation managed by Saltbox.
func ShouldCleanupDeadsnakes() (bool, error) {
	return shouldCleanupDeadsnakesFromFile("/etc/os-release")
}

func shouldCleanupDeadsnakesFromFile(path string) (bool, error) {
	osRelease, err := host.ParseOSRelease(path)
	if err != nil {
		return false, fmt.Errorf("parse OS release: %w", err)
	}

	versionID, ok := osRelease["VERSION_ID"]
	if !ok {
		return false, nil
	}
	return versionID == "20.04" || versionID == "22.04", nil
}

// CleanupDeadsnakesIfNeeded removes the legacy deadsnakes Python 3.12
// packages and apt repositories from Ubuntu 20.04 and 22.04 systems.
func CleanupDeadsnakesIfNeeded(ctx context.Context, verbose bool) (bool, error) {
	shouldCleanup, err := ShouldCleanupDeadsnakes()
	if err != nil {
		return false, fmt.Errorf("check whether deadsnakes cleanup is needed: %w", err)
	}
	if !shouldCleanup {
		return false, nil
	}
	return cleanupDeadsnakes(
		ctx,
		verbose,
		installedDeadsnakesPackages,
		removeDeadsnakesPackages,
		RemoveDeadsnakesRepositories,
	)
}

func cleanupDeadsnakes(
	ctx context.Context,
	verbose bool,
	queryPackages func(context.Context) ([]string, error),
	removePackages func(context.Context, []string, bool) error,
	removeRepositories func(context.Context, bool) (bool, error),
) (bool, error) {
	packages, err := queryPackages(ctx)
	if err != nil {
		return false, err
	}

	cleanedUp := len(packages) > 0
	if err := removePackages(ctx, packages, verbose); err != nil {
		return false, err
	}

	remainingPackages, err := queryPackages(ctx)
	if err != nil {
		return cleanedUp, fmt.Errorf("verify deadsnakes package removal: %w", err)
	}
	if len(remainingPackages) != 0 {
		return cleanedUp, fmt.Errorf("deadsnakes Python packages remain after cleanup: %s", strings.Join(remainingPackages, ", "))
	}

	repositoriesRemoved, err := removeRepositories(ctx, verbose)
	if err != nil {
		return cleanedUp, fmt.Errorf("remove deadsnakes repositories: %w", err)
	}

	return cleanedUp || repositoriesRemoved, nil
}
