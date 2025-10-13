package uv

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
)

const (
	// UVBinaryPath is where uv will be installed
	UVBinaryPath = "/usr/local/bin/uv"
	// UVGitHubRepo is the GitHub repository for uv
	UVGitHubRepo = "astral-sh/uv"
	// UVVersion is the version of uv to download (can be "latest" or specific version like "0.1.0")
	UVVersion = "latest"
)

// DownloadAndInstallUV downloads uv from GitHub releases and installs it to /usr/local/bin
func DownloadAndInstallUV(ctx context.Context, verbose bool) error {
	// Check if uv is already installed
	if _, err := os.Stat(UVBinaryPath); err == nil {
		if verbose {
			fmt.Println("uv is already installed at", UVBinaryPath)
		}
		return nil
	}

	// Get the latest release URL
	var downloadURL string
	if UVVersion == "latest" {
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/latest/download/uv-x86_64-unknown-linux-gnu.tar.gz", UVGitHubRepo)
	} else {
		downloadURL = fmt.Sprintf("https://github.com/%s/releases/download/%s/uv-x86_64-unknown-linux-gnu.tar.gz", UVGitHubRepo, UVVersion)
	}

	if verbose {
		fmt.Println("Downloading uv from", downloadURL)
	}

	// Download the tarball
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("error downloading uv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error downloading uv: received status code %d", resp.StatusCode)
	}

	// Create a temporary file for the tarball
	tmpFile, err := os.CreateTemp("", "uv-*.tar.gz")
	if err != nil {
		return fmt.Errorf("error creating temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write the response body to the temporary file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("error writing to temporary file: %w", err)
	}
	tmpFile.Close()

	// Extract the tarball
	if err := extractUVBinary(tmpPath, UVBinaryPath, verbose); err != nil {
		return fmt.Errorf("error extracting uv binary: %w", err)
	}

	// Set executable permissions
	if err := os.Chmod(UVBinaryPath, 0755); err != nil {
		return fmt.Errorf("error setting permissions on uv binary: %w", err)
	}

	if verbose {
		fmt.Println("Successfully installed uv to", UVBinaryPath)
	}

	return nil
}

// extractUVBinary extracts the uv binary from the tarball
func extractUVBinary(tarballPath, destPath string, verbose bool) error {
	file, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("error opening tarball: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Find and extract the uv binary
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %w", err)
		}

		// Look for the uv binary (it should be in a path like uv-x86_64-unknown-linux-gnu/uv)
		if strings.HasSuffix(header.Name, "/uv") || header.Name == "uv" {
			if verbose {
				fmt.Printf("Extracting %s to %s\n", header.Name, destPath)
			}

			outFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("error creating output file: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tr); err != nil {
				return fmt.Errorf("error writing binary: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("uv binary not found in tarball")
}

// InstallPython installs a specific Python version using uv
func InstallPython(ctx context.Context, version string, verbose bool) error {
	env := os.Environ()
	env = append(env, fmt.Sprintf("UV_PYTHON_INSTALL_DIR=%s", constants.PythonInstallDir))

	cmd := exec.CommandContext(ctx, UVBinaryPath, "python", "install", version)
	cmd.Env = env

	if verbose {
		fmt.Printf("Installing Python %s using uv\n", version)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error installing Python %s: %w", version, err)
	}

	if verbose {
		fmt.Printf("Successfully installed Python %s\n", version)
	}

	return nil
}

// ListInstalledPythons lists all Python versions installed by uv
func ListInstalledPythons(ctx context.Context) ([]string, error) {
	env := os.Environ()
	env = append(env, fmt.Sprintf("UV_PYTHON_INSTALL_DIR=%s", constants.PythonInstallDir))

	cmd := exec.CommandContext(ctx, UVBinaryPath, "python", "list", "--only-installed")
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error listing installed Pythons: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var versions []string
	for _, line := range lines {
		if line != "" {
			versions = append(versions, line)
		}
	}

	return versions, nil
}

// FindPythonBinary returns the path to a specific Python version installed by uv
func FindPythonBinary(ctx context.Context, version string) (string, error) {
	env := os.Environ()
	env = append(env, fmt.Sprintf("UV_PYTHON_INSTALL_DIR=%s", constants.PythonInstallDir))

	cmd := exec.CommandContext(ctx, UVBinaryPath, "python", "find", version)
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error finding Python %s: %w", version, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// CreateVenv creates a virtual environment using Python's built-in venv module
// with a specific Python version installed by uv
func CreateVenv(ctx context.Context, venvPath, pythonVersion string, verbose bool) error {
	// Ensure the parent directory exists
	parentDir := filepath.Dir(venvPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("error creating parent directory: %w", err)
	}

	// Find the Python binary installed by uv
	pythonBinary, err := FindPythonBinary(ctx, pythonVersion)
	if err != nil {
		return fmt.Errorf("error finding Python binary: %w", err)
	}

	if verbose {
		fmt.Printf("Found Python binary at: %s\n", pythonBinary)
		fmt.Printf("Creating virtual environment at %s with Python %s using python -m venv\n", venvPath, pythonVersion)
	}

	// Use python -m venv instead of uv venv for better compatibility
	cmd := exec.CommandContext(ctx, pythonBinary, "-m", "venv", venvPath)

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error creating venv: %w", err)
	}

	if verbose {
		fmt.Printf("Successfully created virtual environment at %s\n", venvPath)
	}

	return nil
}

// UninstallPython removes a specific Python version installed by uv
func UninstallPython(ctx context.Context, version string, verbose bool) error {
	env := os.Environ()
	env = append(env, fmt.Sprintf("UV_PYTHON_INSTALL_DIR=%s", constants.PythonInstallDir))

	cmd := exec.CommandContext(ctx, UVBinaryPath, "python", "uninstall", version)
	cmd.Env = env

	if verbose {
		fmt.Printf("Uninstalling Python %s using uv\n", version)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error uninstalling Python %s: %w", version, err)
	}

	if verbose {
		fmt.Printf("Successfully uninstalled Python %s\n", version)
	}

	return nil
}
