package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAptGetArgsAppliesConffilePolicy(t *testing.T) {
	input := []string{"install", "-y", "curl"}
	want := []string{
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
		"install", "-y", "curl",
	}

	got := aptGetArgs(input)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aptGetArgs() = %#v, want %#v", got, want)
	}

	if !reflect.DeepEqual(input, []string{"install", "-y", "curl"}) {
		t.Fatalf("aptGetArgs mutated its input: %#v", input)
	}
}

func TestNonInteractiveEnvironment(t *testing.T) {
	want := []string{
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=l",
	}

	if got := nonInteractiveEnvironment(); !reflect.DeepEqual(got, want) {
		t.Fatalf("nonInteractiveEnvironment() = %#v, want %#v", got, want)
	}
}

func TestRunAptGetUsesDirectNonInteractiveCommand(t *testing.T) {
	testBin := t.TempDir()
	aptGetPath := filepath.Join(testBin, "apt-get")
	aptGetScript := `#!/bin/sh
printf 'args:%s\n' "$*" >&2
printf 'DEBIAN_FRONTEND:%s\n' "$DEBIAN_FRONTEND" >&2
printf 'NEEDRESTART_MODE:%s\n' "$NEEDRESTART_MODE" >&2
exit 23
`
	if err := os.WriteFile(aptGetPath, []byte(aptGetScript), 0o755); err != nil {
		t.Fatalf("write fake apt-get: %v", err)
	}
	t.Setenv("PATH", testBin)

	err := RunAptGet(context.Background(), []string{"install", "-y", "curl"}, false)
	if err == nil {
		t.Fatal("RunAptGet() unexpectedly succeeded")
	}

	errText := err.Error()
	for _, want := range []string{
		"args:-o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold install -y curl",
		"DEBIAN_FRONTEND:noninteractive",
		"NEEDRESTART_MODE:l",
	} {
		if !strings.Contains(errText, want) {
			t.Errorf("RunAptGet() error does not contain %q:\n%s", want, errText)
		}
	}
}

func TestIsPackageInstalled(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		exitCode  int
		installed bool
		wantError bool
	}{
		{name: "installed", output: "ii ", installed: true},
		{name: "not fully installed", output: "iU ", installed: false},
		{name: "missing", exitCode: 1, installed: false},
		{name: "query failure", exitCode: 2, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testBin := t.TempDir()
			script := "#!/bin/sh\nprintf '%s' '" + test.output + "'\nexit " + fmt.Sprint(test.exitCode) + "\n"
			if err := os.WriteFile(filepath.Join(testBin, "dpkg-query"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", testBin)

			installed, err := IsPackageInstalled(context.Background(), "libpq-dev")
			if (err != nil) != test.wantError {
				t.Fatalf("IsPackageInstalled() error = %v, wantError %v", err, test.wantError)
			}
			if installed != test.installed {
				t.Fatalf("IsPackageInstalled() = %v, want %v", installed, test.installed)
			}
		})
	}
}

// TestInstallPackage_NonExistentPackage tests that we get proper error information
// when trying to install a package that doesn't exist
func TestInstallPackage_NonExistentPackage(t *testing.T) {
	// Use a package name that definitely doesn't exist
	nonExistentPackage := "notathinginvalid-doesnotexist-12345"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the install function with verbose=false to test stderr capture
	installFn := InstallPackage(ctx, []string{nonExistentPackage}, false)

	// Execute the installation
	err := installFn()

	// We expect an error
	if err == nil {
		t.Fatal("Expected error when installing non-existent package, but got nil")
	}

	errMsg := err.Error()

	// Validate that the error message contains the package name
	if !strings.Contains(errMsg, nonExistentPackage) {
		t.Errorf("Error message should contain package name '%s', got: %s", nonExistentPackage, errMsg)
	}

	// Validate that the error message contains exit code information
	// The executor returns lowercase "exit code" or "exit status"
	if !strings.Contains(strings.ToLower(errMsg), "exit") {
		t.Errorf("Error message should contain exit code information, got: %s", errMsg)
	}

	// Validate that the error message contains stderr output with apt error details
	// Common apt error messages for non-existent packages:
	// - "Unable to locate package"
	// - "E: Unable to locate package"
	// - "Package" (at minimum)
	if !strings.Contains(errMsg, "Stderr:") {
		t.Errorf("Error message should contain 'Stderr:' section, got: %s", errMsg)
	}

	// Check for common apt error indicators
	hasAptError := strings.Contains(strings.ToLower(errMsg), "unable to locate") ||
		strings.Contains(strings.ToLower(errMsg), "package") ||
		strings.Contains(errMsg, "E:")

	if !hasAptError {
		t.Errorf("Error message should contain apt error details (e.g., 'Unable to locate package'), got: %s", errMsg)
	}

	t.Logf("Error message (as user would see it):\n%s", errMsg)
}

// TestInstallPackage_VerboseMode tests that verbose mode streams output directly
func TestInstallPackage_VerboseMode(t *testing.T) {
	// This test validates that in verbose mode, output goes to stdout/stderr
	// We can't easily capture this in a unit test, but we can verify it doesn't buffer
	nonExistentPackage := "notathinginvalid-verbose-test"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the install function with verbose=true
	installFn := InstallPackage(ctx, []string{nonExistentPackage}, true)

	// Execute the installation
	err := installFn()

	// We expect an error
	if err == nil {
		t.Fatal("Expected error when installing non-existent package, but got nil")
	}

	errMsg := err.Error()

	// The error message should contain the package name
	if !strings.Contains(errMsg, nonExistentPackage) {
		t.Errorf("Error message should contain package name '%s', got: %s", nonExistentPackage, errMsg)
	}

	// Validate that the error message contains exit information
	if !strings.Contains(strings.ToLower(errMsg), "exit") {
		t.Errorf("Error message should contain exit code information, got: %s", errMsg)
	}

	// Note: In verbose mode with OutputModeStream, stderr still gets captured by RunVerbose
	// and included in the error message. This is expected behavior.

	t.Logf("Verbose mode error message:\n%s", errMsg)
}

func TestAddAptRepositoriesRejectsUnsupportedReleaseBeforeMutation(t *testing.T) {
	root := t.TempDir()
	sourcesFile := filepath.Join(root, "sources.list")
	sourcesDir := filepath.Join(root, "sources.list.d")
	if err := os.Mkdir(sourcesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAptTestFile(t, sourcesFile, "deb http://mirror.example.invalid/ubuntu focal main\n")
	writeAptTestFile(t, filepath.Join(sourcesDir, "ubuntu.sources"), "official source content\n")
	writeAptTestFile(t, filepath.Join(sourcesDir, "third-party.list"), "third-party source content\n")
	before := snapshotAptTestTree(t, root)

	err := addAptRepositoriesForRelease("focal", sourcesFile, sourcesDir, false)
	if err == nil || err.Error() != "unsupported Ubuntu release: focal" {
		t.Fatalf("addAptRepositoriesForRelease() error = %v, want unsupported focal error", err)
	}

	after := snapshotAptTestTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("unsupported release mutated apt sources:\nbefore: %#v\nafter:  %#v", before, after)
	}
}

func TestAddAptRepositoriesPreservesOfficialDeb822Sources(t *testing.T) {
	for _, release := range []string{"noble", "resolute"} {
		t.Run(release, func(t *testing.T) {
			root := t.TempDir()
			sourcesFile := filepath.Join(root, "sources.list")
			sourcesDir := filepath.Join(root, "sources.list.d")
			if err := os.Mkdir(sourcesDir, 0o755); err != nil {
				t.Fatal(err)
			}
			officialSources := fmt.Sprintf("Types: deb\nURIs: http://archive.ubuntu.com/ubuntu/\nSuites: %s %s-updates\n", release, release)
			ubuntuSourcesFile := filepath.Join(sourcesDir, "ubuntu.sources")
			writeAptTestFile(t, ubuntuSourcesFile, officialSources)
			thirdPartyFile := filepath.Join(sourcesDir, "third-party.list")
			writeAptTestFile(t, thirdPartyFile, "deb https://packages.example.invalid stable main\n")

			if err := addAptRepositoriesForRelease(release, sourcesFile, sourcesDir, false); err != nil {
				t.Fatalf("addAptRepositoriesForRelease() error = %v", err)
			}

			if got := readAptTestFile(t, ubuntuSourcesFile); got != officialSources {
				t.Fatalf("ubuntu.sources was changed:\ngot:  %q\nwant: %q", got, officialSources)
			}
			if _, err := os.Stat(thirdPartyFile); !os.IsNotExist(err) {
				t.Fatalf("third-party source was not removed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(sourcesDir, "ubuntu-archive.sources")); !os.IsNotExist(err) {
				t.Fatalf("unexpected ubuntu-archive.sources: %v", err)
			}
		})
	}
}

func TestAddAptRepositoriesAddsOfficialArchiveForCustomDeb822Mirror(t *testing.T) {
	for _, release := range []string{"noble", "resolute"} {
		t.Run(release, func(t *testing.T) {
			root := t.TempDir()
			sourcesFile := filepath.Join(root, "sources.list")
			sourcesDir := filepath.Join(root, "sources.list.d")
			if err := os.Mkdir(sourcesDir, 0o755); err != nil {
				t.Fatal(err)
			}
			customSources := fmt.Sprintf("Types: deb\nURIs: https://mirror.example.invalid/ubuntu/\nSuites: %s\n", release)
			ubuntuSourcesFile := filepath.Join(sourcesDir, "ubuntu.sources")
			writeAptTestFile(t, ubuntuSourcesFile, customSources)

			if err := addAptRepositoriesForRelease(release, sourcesFile, sourcesDir, false); err != nil {
				t.Fatalf("addAptRepositoriesForRelease() error = %v", err)
			}

			if got := readAptTestFile(t, ubuntuSourcesFile); got != customSources {
				t.Fatalf("custom ubuntu.sources was changed:\ngot:  %q\nwant: %q", got, customSources)
			}
			wantArchive := fmt.Sprintf(
				"Types: deb\n"+
					"URIs: http://archive.ubuntu.com/ubuntu/\n"+
					"Suites: %s %s-updates %s-backports\n"+
					"Components: main restricted universe multiverse\n"+
					"Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n"+
					"\n"+
					"Types: deb\n"+
					"URIs: http://security.ubuntu.com/ubuntu/\n"+
					"Suites: %s-security\n"+
					"Components: main restricted universe multiverse\n"+
					"Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n",
				release, release, release, release,
			)
			archiveSourcesFile := filepath.Join(sourcesDir, "ubuntu-archive.sources")
			if got := readAptTestFile(t, archiveSourcesFile); got != wantArchive {
				t.Fatalf("ubuntu-archive.sources = %q, want %q", got, wantArchive)
			}
		})
	}
}

func TestAddAptRepositoriesRetainsJammyLegacyConfiguration(t *testing.T) {
	root := t.TempDir()
	sourcesFile := filepath.Join(root, "sources.list")
	sourcesDir := filepath.Join(root, "sources.list.d")
	if err := os.Mkdir(sourcesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAptTestFile(t, sourcesFile, "# existing configuration\n")
	writeAptTestFile(t, filepath.Join(sourcesDir, "ubuntu.sources"), "DEB822 data not used on Jammy\n")
	writeAptTestFile(t, filepath.Join(sourcesDir, "third-party.list"), "third-party source content\n")

	if err := addAptRepositoriesForRelease("jammy", sourcesFile, sourcesDir, false); err != nil {
		t.Fatalf("addAptRepositoriesForRelease() error = %v", err)
	}

	wantSources := "# existing configuration\n" +
		"deb http://archive.ubuntu.com/ubuntu/ jammy main\n" +
		"deb http://archive.ubuntu.com/ubuntu/ jammy universe\n" +
		"deb http://archive.ubuntu.com/ubuntu/ jammy restricted\n" +
		"deb http://archive.ubuntu.com/ubuntu/ jammy multiverse\n"
	if got := readAptTestFile(t, sourcesFile); got != wantSources {
		t.Fatalf("sources.list = %q, want %q", got, wantSources)
	}
	entries, err := os.ReadDir(sourcesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("sources.list.d entries = %v, want none", entries)
	}
}

func writeAptTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAptTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func snapshotAptTestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[relativePath] = fmt.Sprintf("%#o:%s", info.Mode().Perm(), content)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
