package python

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/host"
)

func TestParseInstalledLegacyPythonPackages(t *testing.T) {
	output := strings.Join([]string{
		"python3.12\tpython3.12\tii \t3.12.12-1+jammy1",
		"python3.12-venv\tpython3.12\trc \t3.12.12-1+jammy1",
		"python3.12-distutils\tpython3-stdlib-extensions\tii \t3.12.12-1+jammy1",
		"libpython3.12-stdlib:amd64\tpython3.12\tii \t3.12.12-1+jammy1",
		"libpython3.12t64:amd64\tpython3.12\tii \t3.12.12-1+jammy1",
		"idle-python3.12\tpython3.12\tii \t3.12.12-1+jammy1",
		"2to3.12\tpython3.12\tii \t3.12.12-1+jammy1",
		"python3.13\tpython3.13\tii \t3.13.7-1",
		"libpython3.13-stdlib:amd64\tpython3.13\tii \t3.13.7-1",
	}, "\n")

	packages, err := parseInstalledLegacyPythonPackages(output)
	if err != nil {
		t.Fatal(err)
	}
	want := []installedPackage{
		{name: "2to3.12", version: "3.12.12-1+jammy1"},
		{name: "idle-python3.12", version: "3.12.12-1+jammy1"},
		{name: "libpython3.12-stdlib:amd64", version: "3.12.12-1+jammy1"},
		{name: "libpython3.12t64:amd64", version: "3.12.12-1+jammy1"},
		{name: "python3.12", version: "3.12.12-1+jammy1"},
		{name: "python3.12-distutils", version: "3.12.12-1+jammy1"},
	}
	if !reflect.DeepEqual(packages, want) {
		t.Fatalf("parseInstalledLegacyPythonPackages() = %v, want %v", packages, want)
	}
}

func TestParseInstalledLegacyPythonPackagesRejectsMalformedOutput(t *testing.T) {
	if _, err := parseInstalledLegacyPythonPackages("python3.12\tpython3.12\tii "); err == nil {
		t.Fatal("parseInstalledLegacyPythonPackages() accepted malformed output")
	}
}

func TestLegacyDeadsnakesPackageMatchingIsIndependentOfManagedPython(t *testing.T) {
	tests := []struct {
		binary string
		source string
		want   bool
	}{
		{binary: "python3.12", source: "python3.12", want: true},
		{binary: "python3.12-tk", source: "python3-stdlib-extensions", want: true},
		{binary: "libpython3.12t64:amd64", source: "python3.12", want: true},
		{binary: "idle-python3.12", source: "python3.12", want: true},
		{binary: "2to3.12", source: "python3.12", want: true},
		{binary: "python3.13", source: "python3.13", want: false},
		{binary: "libpython3.13-stdlib:amd64", source: "python3.13", want: false},
		{binary: "unrelated", source: "unrelated", want: false},
	}

	for _, test := range tests {
		t.Run(test.binary, func(t *testing.T) {
			if got := isLegacyDeadsnakesPackage(test.binary, test.source); got != test.want {
				t.Fatalf("isLegacyDeadsnakesPackage(%q, %q) = %t, want %t", test.binary, test.source, got, test.want)
			}
		})
	}
}

func TestInstalledDeadsnakesPackagesReportsQueryFailure(t *testing.T) {
	testBin := t.TempDir()
	writeExecutable(t, filepath.Join(testBin, "dpkg-query"), "#!/bin/sh\nexit 2\n")
	t.Setenv("PATH", testBin)

	if _, err := installedDeadsnakesPackages(context.Background()); err == nil {
		t.Fatal("installedDeadsnakesPackages() ignored dpkg-query failure")
	}
}

func TestInstalledDeadsnakesPackagesRequiresConfirmedOrigin(t *testing.T) {
	testBin := t.TempDir()
	writeExecutable(t, filepath.Join(testBin, "dpkg-query"), `#!/bin/sh
printf 'python3.12\tpython3.12\tii \t3.12.12-1+jammy1\n'
printf 'python3.12-tk\tpython3-stdlib-extensions\tii \t3.12.12-1+jammy1\n'
`)
	writeExecutable(t, filepath.Join(testBin, "apt-cache"), `#!/bin/sh
if [ "$2" = "python3.12" ]; then
    printf 'python3.12:\n  Installed: 3.12.12-1+jammy1\n  Version table:\n *** 3.12.12-1+jammy1 500\n        500 https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy/main amd64 Packages\n        100 /var/lib/dpkg/status\n'
else
    printf 'python3.12-tk:\n  Installed: 3.12.12-1+jammy1\n  Version table:\n *** 3.12.12-1+jammy1 500\n        500 https://packages.example.invalid/python jammy/main amd64 Packages\n        100 /var/lib/dpkg/status\n'
fi
`)
	t.Setenv("PATH", testBin)

	packages, err := installedDeadsnakesPackages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(packages, []string{"python3.12"}) {
		t.Fatalf("installedDeadsnakesPackages() = %v, want [python3.12]", packages)
	}
}

func TestPolicyVersionHasDeadsnakesOrigin(t *testing.T) {
	const policy = `python3.12:
  Installed: 3.12.12-1+jammy1
 Candidate: 3.12.12-1+jammy1
  Version table:
 *** 3.12.12-1+jammy1 500
        500 https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy/main amd64 Packages
        100 /var/lib/dpkg/status
     3.12.11-1+jammy1 500
        500 https://packages.example.invalid/deadsnakes-snapshot jammy/main amd64 Packages
`

	hasDeadsnakes, hasOther, found := policyVersionOrigins(policy, "3.12.12-1+jammy1")
	if !found || !hasDeadsnakes || hasOther {
		t.Fatalf("installed policy version = (%t, %t, %t), want (true, false, true)", hasDeadsnakes, hasOther, found)
	}

	hasDeadsnakes, hasOther, found = policyVersionOrigins(policy, "3.12.10-1+jammy1")
	if found || hasDeadsnakes || hasOther {
		t.Fatalf("missing policy version = (%t, %t, %t), want (false, false, false)", hasDeadsnakes, hasOther, found)
	}
}

func TestPolicyVersionRejectsAmbiguousOrigin(t *testing.T) {
	const policy = `python3.12:
  Installed: 3.12.12-1+jammy1
  Version table:
 *** 3.12.12-1+jammy1 500
        500 https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy/main amd64 Packages
        500 https://packages.example.invalid/python jammy/main amd64 Packages
        100 /var/lib/dpkg/status
`

	hasDeadsnakes, hasOther, found := policyVersionOrigins(policy, "3.12.12-1+jammy1")
	if !found || !hasDeadsnakes || !hasOther {
		t.Fatalf("ambiguous policy version = (%t, %t, %t), want (true, true, true)", hasDeadsnakes, hasOther, found)
	}
}

func TestPolicyVersionRejectsNonDeadsnakesOrigin(t *testing.T) {
	const policy = `python3.12:
  Installed: 3.12.12-1+jammy1
  Version table:
 *** 3.12.12-1+jammy1 500
        500 https://packages.example.invalid/python jammy/main amd64 Packages
        100 /var/lib/dpkg/status
     3.12.11-1+jammy1 500
        500 https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy/main amd64 Packages
`

	hasDeadsnakes, hasOther, found := policyVersionOrigins(policy, "3.12.12-1+jammy1")
	if !found || hasDeadsnakes || !hasOther {
		t.Fatalf("policy version = (%t, %t, %t), want (false, true, true)", hasDeadsnakes, hasOther, found)
	}
}

func TestPolicyVersionRejectsNonCanonicalDeadsnakesName(t *testing.T) {
	const policy = `python3.12:
  Installed: 3.12.12-1+jammy1
  Version table:
 *** 3.12.12-1+jammy1 500
        500 https://packages.example.invalid/deadsnakes-snapshot jammy/main amd64 Packages
        100 /var/lib/dpkg/status
`

	hasDeadsnakes, hasOther, found := policyVersionOrigins(policy, "3.12.12-1+jammy1")
	if !found || hasDeadsnakes || !hasOther {
		t.Fatalf("noncanonical policy version = (%t, %t, %t), want (false, true, true)", hasDeadsnakes, hasOther, found)
	}
}

func TestInstalledDeadsnakesPackagesUsesVersionFallbackAfterRepositoryRemoval(t *testing.T) {
	testBin := t.TempDir()
	writeExecutable(t, filepath.Join(testBin, "dpkg-query"), `#!/bin/sh
printf 'python3.12\tpython3.12\tii \t3.12.12-1+jammy1\n'
printf 'python3.12-tk\tpython3.12-tk\tii \t99.0-1\n'
`)
	writeExecutable(t, filepath.Join(testBin, "apt-cache"), `#!/bin/sh
if [ "$2" = "python3.12" ]; then
    printf 'python3.12:\n  Installed: 3.12.12-1+jammy1\n  Version table:\n *** 3.12.12-1+jammy1 100\n        100 /var/lib/dpkg/status\n'
else
    printf 'python3.12-tk:\n  Installed: 99.0-1\n  Version table:\n *** 99.0-1 100\n        100 /var/lib/dpkg/status\n'
fi
`)
	t.Setenv("PATH", testBin)

	packages, err := installedDeadsnakesPackages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(packages, []string{"python3.12"}) {
		t.Fatalf("installedDeadsnakesPackages() = %v, want [python3.12]", packages)
	}
}

func TestLegacyDeadsnakesVersionSuffix(t *testing.T) {
	tests := map[string]bool{
		"3.12.12-1+jammy1":       true,
		"3.12.10-1+focal2":       true,
		"3.12.12-1ubuntu0.1":     false,
		"3.12.12-1+jammy":        false,
		"3.12.12-1+jammy1custom": false,
		"99.0-1":                 false,
	}

	for version, want := range tests {
		t.Run(version, func(t *testing.T) {
			if got := hasLegacyDeadsnakesVersionSuffix(version); got != want {
				t.Fatalf("hasLegacyDeadsnakesVersionSuffix(%q) = %t, want %t", version, got, want)
			}
		})
	}
}

func TestRemoveDeadsnakesPackages(t *testing.T) {
	packages := []string{"libpython3.12-stdlib:amd64", "python3.12"}
	waitCalls := 0
	var aptCalls [][]string

	err := removeDeadsnakesPackagesWith(
		context.Background(),
		packages,
		false,
		func(context.Context, bool) error {
			waitCalls++
			return nil
		},
		func(_ context.Context, args []string, _ bool) error {
			aptCalls = append(aptCalls, slices.Clone(args))
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if waitCalls != 1 {
		t.Fatalf("apt lock waits = %d, want 1", waitCalls)
	}
	wantCalls := [][]string{
		{"remove", "-y", "libpython3.12-stdlib:amd64", "python3.12"},
		{"autoremove", "-y"},
	}
	if !reflect.DeepEqual(aptCalls, wantCalls) {
		t.Fatalf("apt calls = %v, want %v", aptCalls, wantCalls)
	}
}

func TestRemoveDeadsnakesPackagesStopsAfterRemovalFailure(t *testing.T) {
	wantErr := errors.New("remove failed")
	aptCalls := 0

	err := removeDeadsnakesPackagesWith(
		context.Background(),
		[]string{"python3.12"},
		false,
		func(context.Context, bool) error { return nil },
		func(context.Context, []string, bool) error {
			aptCalls++
			return wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("removeDeadsnakesPackagesWith() error = %v, want %v", err, wantErr)
	}
	if aptCalls != 1 {
		t.Fatalf("apt calls = %d, want 1", aptCalls)
	}
}

func TestRemoveDeadsnakesRepositories(t *testing.T) {
	sourcesDirectory := t.TempDir()
	deadsnakesPath := filepath.Join(sourcesDirectory, "deadsnakes.list")
	unrelatedPath := filepath.Join(sourcesDirectory, "ubuntu.sources")
	writeFile(t, deadsnakesPath, "deb https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy main\n")
	writeFile(t, unrelatedPath, "Types: deb\nURIs: http://archive.ubuntu.com/ubuntu\n")

	updateCalls := 0
	removed, err := removeDeadsnakesRepositories(
		sourcesDirectory,
		false,
		func() error {
			updateCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("removeDeadsnakesRepositories() reported no removal")
	}
	if updateCalls != 1 {
		t.Fatalf("apt update calls = %d, want 1", updateCalls)
	}
	if _, err := os.Stat(deadsnakesPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deadsnakes source still exists: %v", err)
	}
	if _, err := os.Stat(unrelatedPath); err != nil {
		t.Fatalf("unrelated source was removed: %v", err)
	}

	removed, err = removeDeadsnakesRepositories(
		sourcesDirectory,
		false,
		func() error {
			updateCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("idempotent repository cleanup reported a removal")
	}
	if updateCalls != 1 {
		t.Fatalf("apt update calls after idempotent cleanup = %d, want 1", updateCalls)
	}
}

func TestFindDeadsnakesRepositoryFilesReportsReadFailure(t *testing.T) {
	sourcesDirectory := t.TempDir()
	if err := os.Symlink(filepath.Join(sourcesDirectory, "missing"), filepath.Join(sourcesDirectory, "broken.sources")); err != nil {
		t.Fatal(err)
	}

	if _, err := findDeadsnakesRepositoryFiles(sourcesDirectory); err == nil {
		t.Fatal("findDeadsnakesRepositoryFiles() ignored read failure")
	}
}

func TestRemoveDeadsnakesRepositoriesReportsUpdateFailure(t *testing.T) {
	sourcesDirectory := t.TempDir()
	writeFile(t, filepath.Join(sourcesDirectory, "deadsnakes.sources"), "URIs: https://ppa.launchpadcontent.net/DEADSNAKES/ppa/ubuntu\n")
	wantErr := errors.New("apt update failed")

	removed, err := removeDeadsnakesRepositories(
		sourcesDirectory,
		false,
		func() error { return wantErr },
	)
	if !removed {
		t.Fatal("removeDeadsnakesRepositories() did not report removal before update failure")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("removeDeadsnakesRepositories() error = %v, want %v", err, wantErr)
	}
}

func TestRemoveDeadsnakesRepositoriesVerifiesPostcondition(t *testing.T) {
	sourcesDirectory := t.TempDir()
	sourcePath := filepath.Join(sourcesDirectory, "deadsnakes.list")
	content := "deb https://ppa.launchpadcontent.net/deadsnakes/ppa/ubuntu jammy main\n"
	writeFile(t, sourcePath, content)

	removed, err := removeDeadsnakesRepositories(
		sourcesDirectory,
		false,
		func() error {
			writeFile(t, sourcePath, content)
			return nil
		},
	)
	if !removed {
		t.Fatal("removeDeadsnakesRepositories() did not report removal")
	}
	if err == nil || !strings.Contains(err.Error(), "remain after cleanup") {
		t.Fatalf("removeDeadsnakesRepositories() error = %v, want postcondition failure", err)
	}
}

func TestShouldCleanupDeadsnakesFromFile(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "20.04", want: true},
		{version: "22.04", want: true},
		{version: "24.04", want: false},
		{version: "26.04", want: false},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "os-release")
			writeFile(t, path, "ID=ubuntu\nVERSION_ID=\""+test.version+"\"\n")
			got, err := shouldCleanupDeadsnakesFromFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("shouldCleanupDeadsnakesFromFile() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCleanupDeadsnakesVerifiesPackagePostcondition(t *testing.T) {
	queryCalls := 0
	repositoriesCalled := false

	cleaned, err := cleanupDeadsnakes(
		context.Background(),
		false,
		func(context.Context) ([]string, error) {
			queryCalls++
			return []string{"python3.12"}, nil
		},
		func(context.Context, []string, bool) error { return nil },
		func(context.Context, bool) (bool, error) {
			repositoriesCalled = true
			return false, nil
		},
	)
	if !cleaned {
		t.Fatal("cleanupDeadsnakes() did not report the attempted package cleanup")
	}
	if err == nil || !strings.Contains(err.Error(), "remain after cleanup") {
		t.Fatalf("cleanupDeadsnakes() error = %v, want package postcondition failure", err)
	}
	if queryCalls != 2 {
		t.Fatalf("package query calls = %d, want 2", queryCalls)
	}
	if repositoriesCalled {
		t.Fatal("repository cleanup ran before package postcondition passed")
	}
}

func TestCleanupDeadsnakesReportsPackageAndRepositoryChanges(t *testing.T) {
	queryResults := [][]string{{"python3.12"}, nil}
	removedPackages := []string(nil)

	cleaned, err := cleanupDeadsnakes(
		context.Background(),
		false,
		func(context.Context) ([]string, error) {
			result := queryResults[0]
			queryResults = queryResults[1:]
			return result, nil
		},
		func(_ context.Context, packages []string, _ bool) error {
			removedPackages = slices.Clone(packages)
			return nil
		},
		func(context.Context, bool) (bool, error) { return true, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !cleaned {
		t.Fatal("cleanupDeadsnakes() reported no changes")
	}
	if !slices.Equal(removedPackages, []string{"python3.12"}) {
		t.Fatalf("removed packages = %v, want [python3.12]", removedPackages)
	}
}

func TestCleanupDeadsnakesContainerIntegration(t *testing.T) {
	if os.Getenv("SB_GO_DEADSNAKES_CONTAINER_TEST") != "1" {
		t.Skip("set SB_GO_DEADSNAKES_CONTAINER_TEST=1 inside an isolated container")
	}
	if os.Geteuid() != 0 {
		t.Fatal("container integration test must run as root")
	}

	removedPackages := strings.Fields(os.Getenv("SB_GO_DEADSNAKES_EXPECT_REMOVED"))
	if len(removedPackages) == 0 {
		t.Fatal("container integration package expectations must not be empty")
	}

	cleaned, err := CleanupDeadsnakesIfNeeded(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !cleaned {
		t.Fatal("CleanupDeadsnakesIfNeeded() reported no changes")
	}

	for _, packageName := range removedPackages {
		installed, err := host.IsPackageInstalled(context.Background(), packageName)
		if err != nil {
			t.Fatal(err)
		}
		if installed {
			t.Errorf("confirmed deadsnakes package %s remains installed", packageName)
		}
	}
	repositoryFiles, err := findDeadsnakesRepositoryFiles(aptSourcesDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(repositoryFiles) != 0 {
		t.Errorf("deadsnakes repository files remain: %v", repositoryFiles)
	}

	cleanedAgain, err := CleanupDeadsnakesIfNeeded(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if cleanedAgain {
		t.Fatal("second CleanupDeadsnakesIfNeeded() call was not idempotent")
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
