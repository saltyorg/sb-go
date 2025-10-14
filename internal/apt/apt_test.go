package apt

import (
	"context"
	"strings"
	"testing"
	"time"
)

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

	// Validate that the error message contains "Exit code"
	if !strings.Contains(errMsg, "Exit code") {
		t.Errorf("Error message should contain 'Exit code', got: %s", errMsg)
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

	// In verbose mode, stderr is NOT captured (it goes directly to console)
	// So the error message should NOT contain "Stderr:" section
	if strings.Contains(errMsg, "Stderr:") {
		t.Errorf("In verbose mode, error message should NOT contain buffered 'Stderr:' section, got: %s", errMsg)
	}

	// But it should still contain the package name and exit code
	if !strings.Contains(errMsg, nonExistentPackage) {
		t.Errorf("Error message should contain package name '%s', got: %s", nonExistentPackage, errMsg)
	}

	if !strings.Contains(errMsg, "Exit code") {
		t.Errorf("Error message should contain 'Exit code', got: %s", errMsg)
	}

	t.Logf("Verbose mode error message:\n%s", errMsg)
}
