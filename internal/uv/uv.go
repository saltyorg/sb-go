package uv

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/runtime"
)

const (
	UVBinaryPath = "/usr/local/bin/uv"
	UVGitHubRepo = "astral-sh/uv"
)

var exactVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// DownloadAndInstallUV ensures that the exact uv release pinned into sb-go is installed.
func DownloadAndInstallUV(ctx context.Context, verbose bool) error {
	return EnsureVersion(ctx, runtime.UVVersion, UVBinaryPath, verbose)
}

// EnsureVersion downloads, verifies, and atomically installs an exact uv release.
func EnsureVersion(ctx context.Context, version, destPath string, verbose bool) error {
	if !exactVersionPattern.MatchString(version) {
		return fmt.Errorf("uv version must be an exact release, got %q", version)
	}

	installed, err := binaryVersion(ctx, destPath)
	if err == nil && installed == version {
		if verbose {
			fmt.Printf("uv %s is already installed at %s\n", version, destPath)
		}
		return nil
	}
	if err != nil {
		if _, statErr := os.Stat(destPath); statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("error checking installed uv: %w", statErr)
		}
		if verbose && !os.IsNotExist(err) {
			fmt.Printf("Replacing unusable uv at %s: %v\n", destPath, err)
		}
	}

	downloadURL := fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/uv-x86_64-unknown-linux-gnu.tar.gz",
		UVGitHubRepo,
		version,
	)
	if verbose {
		fmt.Println("Downloading uv from", downloadURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("error creating uv download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error downloading uv: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error downloading uv: received status code %d", resp.StatusCode)
	}

	tarball, err := os.CreateTemp("", "uv-*.tar.gz")
	if err != nil {
		return fmt.Errorf("error creating uv download file: %w", err)
	}
	tarballPath := tarball.Name()
	defer func() { _ = os.Remove(tarballPath) }()
	if _, err := io.Copy(tarball, resp.Body); err != nil {
		_ = tarball.Close()
		return fmt.Errorf("error writing uv download: %w", err)
	}
	if err := tarball.Close(); err != nil {
		return fmt.Errorf("error closing uv download: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("error creating uv destination directory: %w", err)
	}
	staged, err := os.CreateTemp(filepath.Dir(destPath), ".uv-*")
	if err != nil {
		return fmt.Errorf("error creating staged uv binary: %w", err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("error closing staged uv binary: %w", err)
	}
	defer func() { _ = os.Remove(stagedPath) }()

	if err := extractUVBinary(tarballPath, stagedPath, verbose); err != nil {
		return fmt.Errorf("error extracting uv binary: %w", err)
	}
	if err := os.Chmod(stagedPath, 0755); err != nil {
		return fmt.Errorf("error setting uv permissions: %w", err)
	}
	stagedVersion, err := binaryVersion(ctx, stagedPath)
	if err != nil {
		return fmt.Errorf("error verifying staged uv: %w", err)
	}
	if stagedVersion != version {
		return fmt.Errorf("downloaded uv reports version %s, expected %s", stagedVersion, version)
	}
	if err := os.Rename(stagedPath, destPath); err != nil {
		return fmt.Errorf("error activating uv %s: %w", version, err)
	}

	if verbose {
		fmt.Printf("Installed uv %s to %s\n", version, destPath)
	}
	return nil
}

func binaryVersion(ctx context.Context, path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	result, err := executor.Run(ctx, path, executor.WithArgs("--version"))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(string(result.Combined)))
	if len(fields) < 2 || fields[0] != "uv" {
		return "", fmt.Errorf("unexpected uv version output %q", strings.TrimSpace(string(result.Combined)))
	}
	return fields[1], nil
}

func extractUVBinary(tarballPath, destPath string, verbose bool) error {
	file, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("error opening tarball: %w", err)
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %w", err)
		}
		if !strings.HasSuffix(header.Name, "/uv") && header.Name != "uv" {
			continue
		}
		if verbose {
			fmt.Printf("Extracting %s to %s\n", header.Name, destPath)
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("error opening staged uv binary: %w", err)
		}
		_, copyErr := io.Copy(outFile, tr)
		closeErr := outFile.Close()
		if copyErr != nil {
			return fmt.Errorf("error writing uv binary: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("error closing uv binary: %w", closeErr)
		}
		return nil
	}
	return fmt.Errorf("uv binary not found in tarball")
}

func InstallPythonAt(ctx context.Context, version, installDir string, reinstall, verbose bool) error {
	args := []string{"python", "install", "--managed-python", "--no-bin", "--install-dir", installDir}
	if reinstall {
		args = append(args, "--reinstall")
	}
	args = append(args, version)
	if err := executor.RunVerbose(ctx, UVBinaryPath, args, verbose); err != nil {
		return fmt.Errorf("error installing Python %s: %w", version, err)
	}
	return nil
}

func FindPythonAt(ctx context.Context, version, installDir string) (string, error) {
	result, err := executor.Run(ctx, UVBinaryPath,
		executor.WithArgs("python", "find", "--managed-python", "--no-project", "--no-python-downloads", version),
		executor.WithInheritEnv("UV_PYTHON_INSTALL_DIR="+installDir),
	)
	if err != nil {
		return "", fmt.Errorf("error finding Python %s: %w", version, err)
	}
	return strings.TrimSpace(string(result.Combined)), nil
}

func CreateVenvWithPython(ctx context.Context, venvPath, pythonPath string, verbose bool) error {
	if err := os.MkdirAll(filepath.Dir(venvPath), 0755); err != nil {
		return fmt.Errorf("error creating venv parent: %w", err)
	}
	args := []string{"venv", "--python", pythonPath, "--no-project", "--no-python-downloads", venvPath}
	if err := executor.RunVerbose(ctx, UVBinaryPath, args, verbose); err != nil {
		return fmt.Errorf("error creating venv: %w", err)
	}
	return nil
}

func SyncRequirements(ctx context.Context, pythonPath, requirementsPath string, verbose bool, stdout, stderr io.Writer) error {
	options := syncRequirementsOptions(pythonPath, requirementsPath, verbose, stdout, stderr)
	if _, err := executor.Run(ctx, UVBinaryPath, options...); err != nil {
		return fmt.Errorf("error syncing requirements: %w", err)
	}
	return nil
}

func syncRequirementsOptions(pythonPath, requirementsPath string, verbose bool, stdout, stderr io.Writer) []executor.Option {
	options := []executor.Option{
		executor.WithArgs("pip", "sync", "--no-progress", "--python", pythonPath, "--require-hashes", requirementsPath),
	}
	if stdout != nil {
		options = append(options, executor.WithOutputMode(executor.OutputModeStream), executor.WithStdout(stdout))
	}
	if stderr != nil {
		options = append(options, executor.WithStderr(stderr))
	}
	if verbose && stdout == nil {
		options = append(options, executor.WithOutputMode(executor.OutputModeStream))
	}
	return options
}

func CheckPackages(ctx context.Context, pythonPath string) error {
	_, err := executor.Run(ctx, UVBinaryPath, executor.WithArgs("pip", "check", "--python", pythonPath))
	if err != nil {
		return fmt.Errorf("installed Python packages are incompatible: %w", err)
	}
	return nil
}

// Compatibility wrappers retained for callers outside the reconciler.
func InstallPython(ctx context.Context, version string, verbose bool) error {
	return InstallPythonAt(ctx, version, constants.PythonInstallDir, false, verbose)
}

func FindPythonBinary(ctx context.Context, version string) (string, error) {
	return FindPythonAt(ctx, version, constants.PythonInstallDir)
}

func CreateVenv(ctx context.Context, venvPath, pythonVersion string, verbose bool) error {
	pythonPath, err := FindPythonBinary(ctx, pythonVersion)
	if err != nil {
		return err
	}
	return CreateVenvWithPython(ctx, venvPath, pythonPath, verbose)
}

func ListInstalledPythons(ctx context.Context) ([]string, error) {
	result, err := executor.Run(ctx, UVBinaryPath,
		executor.WithArgs("python", "list", "--only-installed", "--install-dir", constants.PythonInstallDir),
	)
	if err != nil {
		return nil, fmt.Errorf("error listing installed Pythons: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.Combined)), "\n")
	return lines, nil
}

func UninstallPython(ctx context.Context, version string, verbose bool) error {
	return executor.RunVerbose(ctx, UVBinaryPath,
		[]string{"python", "uninstall", "--install-dir", constants.PythonInstallDir, version}, verbose)
}
