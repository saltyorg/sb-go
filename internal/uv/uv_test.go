package uv

import (
	"context"
	"os"
	"testing"
)

// TestUVBinaryPath tests that the UV binary path constant is correct
func TestUVBinaryPath(t *testing.T) {
	expected := "/usr/local/bin/uv"
	if UVBinaryPath != expected {
		t.Errorf("Expected UVBinaryPath to be %s, got %s", expected, UVBinaryPath)
	}
}

// TestExtractUVBinary tests the extraction logic with a mock
func TestExtractUVBinary(t *testing.T) {
	// This is a basic structure test - full integration testing would require
	// a real tarball or mock file system
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

// TestDownloadAndInstallUV tests the download and install with context
func TestDownloadAndInstallUV(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Only run if we're on Linux and have write access to /usr/local/bin
	if os.Getenv("CI") == "" {
		t.Skip("Skipping download test outside CI environment")
	}

	ctx := context.Background()
	err := DownloadAndInstallUV(ctx, true)
	if err != nil {
		t.Errorf("Expected no error downloading and installing uv, got: %v", err)
	}

	// Verify the binary exists
	if _, err := os.Stat(UVBinaryPath); os.IsNotExist(err) {
		t.Errorf("Expected uv binary to exist at %s after installation", UVBinaryPath)
	}
}
