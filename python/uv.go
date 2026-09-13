package python

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	UVBinaryPath  = "/usr/local/bin/uv"
	UVXBinaryPath = "/usr/local/bin/uvx"
	UVGitHubRepo  = "astral-sh/uv"
)

var exactUVVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// DownloadAndInstallUV ensures that the exact uv and uvx release pinned into sb-go is installed.
func DownloadAndInstallUV(ctx context.Context, verbose bool) error {
	return EnsureVersion(ctx, buildinfo.Current().UVVersion, UVBinaryPath, verbose)
}

// EnsureVersion downloads, verifies, and atomically installs an exact uv and uvx release.
func EnsureVersion(ctx context.Context, version, destPath string, verbose bool) error {
	metadataURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", UVGitHubRepo, version)
	return ensureVersionWithProxy(ctx, version, destPath, destPath+"x", verbose, layout.SVMVersionProxyURL, metadataURL, assetrelease.HTTPClient(30*time.Second))
}

func ensureVersionWithProxy(ctx context.Context, version, uvPath, uvxPath string, verbose bool, proxyBaseURL, metadataURL string, client *http.Client) error {
	return ensureVersionWithProxyAndRuntime(ctx, defaultManagedRuntime(), version, uvPath, uvxPath, verbose, proxyBaseURL, metadataURL, client)
}

func ensureVersionWithProxyAndRuntime(ctx context.Context, runtime *managedRuntime, version, uvPath, uvxPath string, verbose bool, proxyBaseURL, metadataURL string, client *http.Client) error {
	if !exactUVVersionPattern.MatchString(version) {
		return fmt.Errorf("uv version must be an exact release, got %q", version)
	}

	installedUV, uvErr := binaryVersionWithRuntime(ctx, runtime, uvPath, "uv")
	installedUVX, uvxErr := binaryVersionWithRuntime(ctx, runtime, uvxPath, "uvx")
	if uvErr == nil && uvxErr == nil && installedUV == version && installedUVX == version {
		if verbose {
			fmt.Printf("uv and uvx %s are already installed at %s and %s\n", version, uvPath, uvxPath)
		}
		return nil
	}
	for _, check := range []struct {
		name string
		path string
		err  error
	}{
		{name: "uv", path: uvPath, err: uvErr},
		{name: "uvx", path: uvxPath, err: uvxErr},
	} {
		if check.err == nil {
			continue
		}
		if _, statErr := os.Stat(check.path); statErr != nil && !os.IsNotExist(statErr) {
			return fmt.Errorf("error checking installed %s: %w", check.name, statErr)
		}
		if verbose && !os.IsNotExist(check.err) {
			fmt.Printf("Replacing unusable %s at %s: %v\n", check.name, check.path, check.err)
		}
	}
	scratchDir, err := runtime.newScratch()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratchDir) }()

	asset, err := fetchUVAssetWithFallback(ctx, client, proxyBaseURL, metadataURL)
	if err != nil {
		return fmt.Errorf("error fetching uv %s release metadata: %w", version, err)
	}
	if verbose {
		fmt.Println("Downloading uv from", asset.URL)
	}
	tarballPath, err := assetrelease.DownloadVerified(ctx, client, asset, 128<<20, scratchDir, "uv-*.tar.gz")
	if err != nil {
		return fmt.Errorf("error downloading uv: %w", err)
	}
	defer func() { _ = os.Remove(tarballPath) }()

	if filepath.Dir(uvPath) != filepath.Dir(uvxPath) {
		return fmt.Errorf("uv and uvx destinations must share a directory")
	}
	if err := os.MkdirAll(filepath.Dir(uvPath), 0755); err != nil {
		return fmt.Errorf("error creating uv destination directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(uvPath), ".uv-install-*")
	if err != nil {
		return fmt.Errorf("error creating uv staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	type uvExecutable struct {
		name   string
		target string
		staged string
	}
	executables := []uvExecutable{{name: "uv", target: uvPath}, {name: "uvx", target: uvxPath}}
	for index := range executables {
		executables[index].staged = filepath.Join(stagingDir, executables[index].name)
		if err := os.WriteFile(executables[index].staged, nil, 0600); err != nil {
			return fmt.Errorf("error creating staged %s binary: %w", executables[index].name, err)
		}

		if err := extractUVBinary(tarballPath, executables[index].staged, executables[index].name, verbose); err != nil {
			return fmt.Errorf("error extracting %s binary: %w", executables[index].name, err)
		}
		if err := os.Chmod(executables[index].staged, 0755); err != nil {
			return fmt.Errorf("error setting %s permissions: %w", executables[index].name, err)
		}
		stagedVersion, err := binaryVersionWithRuntime(ctx, runtime, executables[index].staged, executables[index].name)
		if err != nil {
			return fmt.Errorf("error verifying staged %s: %w", executables[index].name, err)
		}
		if stagedVersion != version {
			return fmt.Errorf("downloaded %s reports version %s, expected %s", executables[index].name, stagedVersion, version)
		}
	}
	for index := range executables {
		activationFile, err := os.CreateTemp(filepath.Dir(executables[index].target), "."+executables[index].name+"-*")
		if err != nil {
			return fmt.Errorf("error creating staged %s activation file: %w", executables[index].name, err)
		}
		activationPath := activationFile.Name()
		if err := activationFile.Close(); err != nil {
			_ = os.Remove(activationPath)
			return fmt.Errorf("error closing staged %s activation file: %w", executables[index].name, err)
		}
		if err := os.Rename(executables[index].staged, activationPath); err != nil {
			_ = os.Remove(activationPath)
			return fmt.Errorf("error preparing staged %s for activation: %w", executables[index].name, err)
		}
		executables[index].staged = activationPath
		defer func(path string) { _ = os.Remove(path) }(activationPath)
	}
	for _, executable := range executables {
		if err := assetrelease.Activate(executable.staged, executable.target, 0755); err != nil {
			return fmt.Errorf("error activating %s %s: %w", executable.name, version, err)
		}
	}

	if verbose {
		fmt.Printf("Installed uv and uvx %s to %s and %s\n", version, uvPath, uvxPath)
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
	resp, err := assetrelease.GetWithRetry(ctx, client, metadataURL)
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

func fetchUVAssetWithFallback(ctx context.Context, client *http.Client, proxyBaseURL, metadataURL string) (assetrelease.Asset, error) {
	proxyURL, err := url.Parse(proxyBaseURL)
	if err != nil || proxyURL.Scheme != "https" || proxyURL.Host == "" {
		return assetrelease.Asset{}, fmt.Errorf("invalid SVM proxy URL %q", proxyBaseURL)
	}
	query := proxyURL.Query()
	query.Set("url", metadataURL)
	proxyURL.RawQuery = query.Encode()

	asset, proxyErr := fetchUVAsset(ctx, client, proxyURL.String())
	if proxyErr == nil {
		return asset, nil
	}
	asset, githubErr := fetchUVAsset(ctx, client, metadataURL)
	if githubErr != nil {
		return assetrelease.Asset{}, fmt.Errorf("SVM proxy request failed: %w; direct GitHub API request failed: %w", proxyErr, githubErr)
	}
	return asset, nil
}

func binaryVersion(ctx context.Context, path, name string) (string, error) {
	return binaryVersionWithRuntime(ctx, defaultManagedRuntime(), path, name)
}

func binaryVersionWithRuntime(ctx context.Context, runtime *managedRuntime, path, name string) (string, error) {
	result, err := runtime.run(ctx, managedCommand{path: path, args: []string{"--version"}, kind: managedUV})
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(string(result.Combined)))
	if len(fields) < 2 || fields[0] != name {
		return "", fmt.Errorf("unexpected %s version output %q", name, strings.TrimSpace(string(result.Combined)))
	}
	return fields[1], nil
}

func extractUVBinary(tarballPath, destPath, name string, verbose bool) error {
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
		if !strings.HasSuffix(header.Name, "/"+name) && header.Name != name {
			continue
		}
		if !header.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("%s archive entry is not a regular file", name)
		}
		if header.Size <= 0 || header.Size > 128<<20 {
			return fmt.Errorf("%s archive entry has invalid size %d", name, header.Size)
		}
		if verbose {
			fmt.Printf("Extracting %s to %s\n", header.Name, destPath)
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("error opening staged %s binary: %w", name, err)
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
			return fmt.Errorf("error closing %s binary: %w", name, err)
		}
		if written != header.Size {
			return fmt.Errorf("%s archive entry size mismatch: expected %d bytes, got %d", name, header.Size, written)
		}
		return nil
	}
	return fmt.Errorf("%s binary not found in tarball", name)
}

func InstallPythonAt(ctx context.Context, version, installDir string, reinstall, noCache, verbose bool) error {
	return installPythonAtWithRuntime(ctx, defaultManagedRuntime(), version, installDir, reinstall, noCache, verbose)
}

func installPythonAtWithRuntime(ctx context.Context, runtime *managedRuntime, version, installDir string, reinstall, noCache, verbose bool) error {
	args := installPythonArgs(version, installDir, reinstall, noCache)
	if err := runtime.runVerbose(ctx, managedCommand{
		path: runtime.uvPath, args: args, kind: managedUV, pathArgs: []int{5},
	}, verbose); err != nil {
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
	return findPythonAtWithRuntime(ctx, defaultManagedRuntime(), version, installDir)
}

func findPythonAtWithRuntime(ctx context.Context, runtime *managedRuntime, version, installDir string) (string, error) {
	result, err := runtime.run(ctx, managedCommand{
		path:    runtime.uvPath,
		args:    []string{"python", "find", "--managed-python", "--no-project", "--no-python-downloads", version},
		kind:    managedUV,
		pathEnv: map[string]string{"UV_PYTHON_INSTALL_DIR": installDir},
	})
	if err != nil {
		return "", fmt.Errorf("error finding Python %s: %w", version, err)
	}
	pythonPath := strings.TrimSpace(string(result.Combined))
	if !filepath.IsAbs(pythonPath) {
		return "", fmt.Errorf("uv returned non-absolute path %q for Python %s", pythonPath, version)
	}
	return filepath.Clean(pythonPath), nil
}

func CreateVenvWithPython(ctx context.Context, venvPath, pythonPath string, verbose bool) error {
	return createVenvWithPythonRuntime(ctx, defaultManagedRuntime(), venvPath, pythonPath, verbose)
}

func createVenvWithPythonRuntime(ctx context.Context, runtime *managedRuntime, venvPath, pythonPath string, verbose bool) error {
	if venvPath == "" {
		return fmt.Errorf("venv path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(venvPath), 0755); err != nil {
		return fmt.Errorf("error creating venv parent: %w", err)
	}
	args := []string{"venv", "--python", pythonPath, "--no-project", "--no-python-downloads", venvPath}
	if err := runtime.runVerbose(ctx, managedCommand{
		path: runtime.uvPath, args: args, kind: managedUV, pathArgs: []int{2, 5},
	}, verbose); err != nil {
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
	return syncRequirementsWithRuntime(ctx, defaultManagedRuntime(), pythonPath, requirementsPath, settings)
}

func syncRequirementsWithRuntime(ctx context.Context, runtime *managedRuntime, pythonPath, requirementsPath string, settings SyncRequirementsOptions) error {
	args := syncRequirementsArgs(pythonPath, requirementsPath, settings.NoCache)
	command := managedCommand{
		path: runtime.uvPath, args: args, kind: managedUV, pathArgs: []int{4, len(args) - 1},
		stdout: settings.Stdout, stderr: settings.Stderr,
	}
	if settings.Stdout != nil || (settings.Verbose && settings.Stdout == nil) {
		command.outputMode = executor.OutputModeStream
		command.outputModeSet = true
	}
	if _, err := runtime.run(ctx, command); err != nil {
		return fmt.Errorf("error syncing requirements: %w", err)
	}
	return nil
}

func syncRequirementsOptions(pythonPath, requirementsPath string, settings SyncRequirementsOptions) []executor.Option {
	args := syncRequirementsArgs(pythonPath, requirementsPath, settings.NoCache)
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

func syncRequirementsArgs(pythonPath, requirementsPath string, noCache bool) []string {
	args := []string{"pip", "sync", "--no-progress", "--python", pythonPath, "--require-hashes"}
	if noCache {
		args = append(args, "--no-cache")
	}
	return append(args, requirementsPath)
}

func CheckPackages(ctx context.Context, pythonPath string) error {
	return checkPackagesWithRuntime(ctx, defaultManagedRuntime(), pythonPath)
}

func checkPackagesWithRuntime(ctx context.Context, runtime *managedRuntime, pythonPath string) error {
	_, err := runtime.run(ctx, managedCommand{
		path: runtime.uvPath, args: []string{"pip", "check", "--python", pythonPath}, kind: managedUV, pathArgs: []int{3},
	})
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
	return listInstalledPythonsWithRuntime(ctx, defaultManagedRuntime(), layout.PythonInstallDir)
}

func listInstalledPythonsWithRuntime(ctx context.Context, runtime *managedRuntime, installDir string) ([]string, error) {
	result, err := runtime.run(ctx, managedCommand{
		path:    runtime.uvPath,
		args:    []string{"python", "list", "--only-installed"},
		kind:    managedUV,
		pathEnv: map[string]string{"UV_PYTHON_INSTALL_DIR": installDir},
	})
	if err != nil {
		return nil, fmt.Errorf("error listing installed Pythons: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(result.Combined)), "\n")
	return lines, nil
}

func UninstallPython(ctx context.Context, version string, verbose bool) error {
	return uninstallPythonWithRuntime(ctx, defaultManagedRuntime(), version, layout.PythonInstallDir, verbose)
}

func uninstallPythonWithRuntime(ctx context.Context, runtime *managedRuntime, version, installDir string, verbose bool) error {
	return runtime.runVerbose(ctx, managedCommand{
		path: runtime.uvPath,
		args: []string{"python", "uninstall", "--install-dir", installDir, version},
		kind: managedUV, pathArgs: []int{3},
	}, verbose)
}
