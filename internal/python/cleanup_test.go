package python

import (
	"slices"
	"testing"

	"github.com/saltyorg/sb-go/internal/constants"
)

// TestDeadsnakesPackages tests that the correct package list is generated
func TestDeadsnakesPackages(t *testing.T) {
	pythonVersion := constants.AnsibleVenvPythonVersion
	packages := DeadsnakesPackages(pythonVersion)

	// Check that we have the expected packages
	expectedPackages := []string{
		"python" + pythonVersion,
		"python" + pythonVersion + "-dev",
		"python" + pythonVersion + "-distutils",
		"python" + pythonVersion + "-venv",
		"libpython" + pythonVersion,
		"libpython" + pythonVersion + "-dev",
		"libpython" + pythonVersion + "-minimal",
		"libpython" + pythonVersion + "-stdlib",
		"python" + pythonVersion + "-minimal",
	}

	if len(packages) != len(expectedPackages) {
		t.Errorf("Expected %d packages, got %d", len(expectedPackages), len(packages))
	}

	// Check each expected package is in the list
	for _, expected := range expectedPackages {
		found := slices.Contains(packages, expected)
		if !found {
			t.Errorf("Expected package %s not found in list", expected)
		}
	}
}

// TestDeadsnakesPackagesWithVersion tests package generation with different versions
func TestDeadsnakesPackagesWithVersion(t *testing.T) {
	testCases := []struct {
		version  string
		expected string
	}{
		{"3.10", "python3.10"},
		{"3.11", "python3.11"},
		{constants.AnsibleVenvPythonVersion, "python" + constants.AnsibleVenvPythonVersion},
	}

	for _, tc := range testCases {
		packages := DeadsnakesPackages(tc.version)
		if len(packages) == 0 {
			t.Errorf("Expected packages for version %s, got empty list", tc.version)
		}
		// Check first package matches expected
		if packages[0] != tc.expected {
			t.Errorf("Expected first package to be %s, got %s", tc.expected, packages[0])
		}
	}
}
