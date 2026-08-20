package python

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/layout"
	assetrelease "github.com/saltyorg/sb-go/release"
)

const (
	UVBinaryPath = "/usr/local/bin/uv"
	UVGitHubRepo = "astral-sh/uv"
)

var exactUVVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// DownloadAndInstallUV ensures that the exact uv release pinned into sb-go is installed.
func DownloadAndInstallUV(ctx context.Context, verbose bool) error {
	return EnsureVersion(ctx, buildinfo.Current().UVVersion, UVBinaryPath, verbose)
}

// EnsureVersion downloads, verifies, and atomically installs an exact uv release.
func EnsureVersion(ctx context.Context, version, destPath string, verbose bool) error {
	metadataURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", UVGitHubRepo, version)
	return ensureVersion(ctx, version, destPath, verbose, metadataURL, assetrelease.HTTPClient(30*time.Second))
}

func ensureVersion(ctx context.Context, version, destPath string, verbose bool, metadataURL string, client *http.Client) error {
	if !exactUVVersionPattern.MatchString(version) {
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

	asset, err := fetchUVAsset(ctx, client, metadataURL)
	if err != nil {
		return fmt.Errorf("error fetching uv %s release metadata: %w", version, err)
	}
	if verbose {
		fmt.Println("Downloading uv from", asset.URL)
	}
	tarballPath, err := assetrelease.DownloadVerified(ctx, client, asset, 128<<20, os.TempDir(), "uv-*.tar.gz")
	if err != nil {
		return fmt.Errorf("error downloading uv: %w", err)
	}
	defer func() { _ = os.Remove(tarballPath) }()

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
	if err := assetrelease.Activate(stagedPath, destPath, 0755); err != nil {
		return fmt.Errorf("error activating uv %s: %w", version, err)
	}

	if verbose {
		fmt.Printf("Installed uv %s to %s\n", version, destPath)
	}
	return nil
}

type uvRelease struct {
	Assets []struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		Digest             string `json:"digest"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchUVAsset(ctx context.Context, client *http.Client, metadataURL string) (assetrelease.Asset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return assetrelease.Asset{}, fmt.Errorf("create release metadata request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return assetrelease.Asset{}, fmt.Errorf("download release metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return assetrelease.Asset{}, fmt.Errorf("release metadata returned HTTP status %s", resp.Status)
	}
	var metadata uvRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&metadata); err != nil {
		return assetrelease.Asset{}, fmt.Errorf("decode release metadata: %w", err)
	}
	const assetName = "uv-x86_64-unknown-linux-gnu.tar.gz"
	for _, candidate := range metadata.Assets {
		if candidate.Name != assetName {
			continue
		}
		return assetrelease.NewAsset(candidate.ID, candidate.Name, candidate.BrowserDownloadURL, candidate.Size, candidate.Digest)
	}
	return assetrelease.Asset{}, fmt.Errorf("release is missing %s", assetName)
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
		if !header.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("uv archive entry is not a regular file")
		}
		if header.Size <= 0 || header.Size > 128<<20 {
			return fmt.Errorf("uv archive entry has invalid size %d", header.Size)
		}
		if verbose {
			fmt.Printf("Extracting %s to %s\n", header.Name, destPath)
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("error opening staged uv binary: %w", err)
		}
		written, copyErr := io.Copy(outFile, io.LimitReader(tr, header.Size+1))
		if copyErr != nil {
			_ = outFile.Close()
			return fmt.Errorf("error writing uv binary: %w", copyErr)
		}
		if err := outFile.Sync(); err != nil {
			_ = outFile.Close()
			return fmt.Errorf("error syncing uv binary: %w", err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("error closing uv binary: %w", err)
		}
		if written != header.Size {
			return fmt.Errorf("uv archive entry size mismatch: expected %d bytes, got %d", header.Size, written)
		}
		return nil
	}
	return fmt.Errorf("uv binary not found in tarball")
}

func InstallPythonAt(ctx context.Context, version, installDir string, reinstall, noCache, verbose bool) error {
	args := installPythonArgs(version, installDir, reinstall, noCache)
	if err := executor.RunVerbose(ctx, UVBinaryPath, args, verbose); err != nil {
		return fmt.Errorf("error installing Python %s: %w", version, err)
	}
	return nil
}

func installPythonArgs(version, installDir string, reinstall, noCache bool) []string {
	args := []string{"python", "install", "--managed-python", "--no-bin", "--install-dir", installDir}
	if reinstall {
		args = append(args, "--reinstall")
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	return append(args, version)
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

type SyncRequirementsOptions struct {
	NoCache bool
	Verbose bool
	Stdout  io.Writer
	Stderr  io.Writer
}

func SyncRequirements(ctx context.Context, pythonPath, requirementsPath string, settings SyncRequirementsOptions) error {
	executorOptions := syncRequirementsOptions(pythonPath, requirementsPath, settings)
	if _, err := executor.Run(ctx, UVBinaryPath, executorOptions...); err != nil {
		return fmt.Errorf("error syncing requirements: %w", err)
	}
	return nil
}

func syncRequirementsOptions(pythonPath, requirementsPath string, settings SyncRequirementsOptions) []executor.Option {
	args := []string{"pip", "sync", "--no-progress", "--python", pythonPath, "--require-hashes"}
	if settings.NoCache {
		args = append(args, "--no-cache")
	}
	args = append(args, requirementsPath)
	executorOptions := []executor.Option{
		executor.WithArgs(args...),
	}
	if settings.Stdout != nil {
		executorOptions = append(executorOptions, executor.WithOutputMode(executor.OutputModeStream), executor.WithStdout(settings.Stdout))
	}
	if settings.Stderr != nil {
		executorOptions = append(executorOptions, executor.WithStderr(settings.Stderr))
	}
	if settings.Verbose && settings.Stdout == nil {
		executorOptions = append(executorOptions, executor.WithOutputMode(executor.OutputModeStream))
	}
	return executorOptions
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
	return InstallPythonAt(ctx, version, layout.PythonInstallDir, false, false, verbose)
}

func FindPythonBinary(ctx context.Context, version string) (string, error) {
	return FindPythonAt(ctx, version, layout.PythonInstallDir)
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
		executor.WithArgs("python", "list", "--only-installed", "--install-dir", layout.PythonInstallDir),
	)
	if err != nil {
		return nil, fmt.Errorf("error listing installed Pythons: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.Combined)), "\n")
	return lines, nil
}

func UninstallPython(ctx context.Context, version string, verbose bool) error {
	return executor.RunVerbose(ctx, UVBinaryPath,
		[]string{"python", "uninstall", "--install-dir", layout.PythonInstallDir, version}, verbose)
}
