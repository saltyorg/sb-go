package python

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/executor"
)

func TestUVBinaryPath(t *testing.T) {
	const expected = "/usr/local/bin/uv"
	if UVBinaryPath != expected {
		t.Fatalf("UVBinaryPath = %q, want %q", UVBinaryPath, expected)
	}
}

func TestExtractUVBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "uv.tar.gz")
	want := []byte("test uv binary")

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{Name: "uv-x86_64-unknown-linux-gnu/uv", Mode: 0755, Size: int64(len(want))}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "uv")
	if err := os.WriteFile(destination, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractUVBinary(archivePath, destination, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted uv = %q, want %q", got, want)
	}
}

func TestEnsureVersionRejectsFloatingVersion(t *testing.T) {
	err := EnsureVersion(context.Background(), "latest", filepath.Join(t.TempDir(), "uv"), false)
	if err == nil || !strings.Contains(err.Error(), "exact release") {
		t.Fatalf("EnsureVersion() error = %v, want exact-release rejection", err)
	}
}

func TestRuntimeUVVersionMatchesVersionFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".uv-version"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != buildinfo.Current().UVVersion {
		t.Fatalf(".uv-version = %q, buildinfo UVVersion = %q", got, buildinfo.Current().UVVersion)
	}
}

func TestSyncRequirementsUsesNonInteractiveProgressFreeOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	config := executor.Config{OutputMode: executor.OutputModeCombined}
	for _, option := range syncRequirementsOptions("/venv/bin/python", "/requirements.txt", SyncRequirementsOptions{
		Verbose: true,
		Stdout:  &stdout,
		Stderr:  &stderr,
	}) {
		option(&config)
	}

	wantArgs := []string{"pip", "sync", "--no-progress", "--python", "/venv/bin/python", "--require-hashes", "/requirements.txt"}
	if strings.Join(config.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("sync arguments = %q, want %q", config.Args, wantArgs)
	}
	if config.PseudoTerminal {
		t.Fatal("uv sync unexpectedly enables a pseudo-terminal")
	}
	if config.OutputMode != executor.OutputModeStream {
		t.Fatalf("output mode = %v, want stream", config.OutputMode)
	}
	if config.Stdout != &stdout || config.Stderr != &stderr {
		t.Fatal("uv sync did not retain managed output writers")
	}
}

func TestCleanReinstallBypassesUVCache(t *testing.T) {
	t.Run("Python install", func(t *testing.T) {
		got := installPythonArgs("3.12.13", "/srv/python/release", true, true)
		want := []string{
			"python", "install", "--managed-python", "--no-bin", "--install-dir", "/srv/python/release",
			"--reinstall", "--no-cache", "3.12.13",
		}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("install arguments = %q, want %q", got, want)
		}
	})

	t.Run("Python install during forced venv recovery", func(t *testing.T) {
		got := installPythonArgs("3.12.13", "/srv/python/release", false, true)
		if !slices.Contains(got, "--no-cache") || slices.Contains(got, "--reinstall") {
			t.Fatalf("forced venv Python fallback arguments = %q, want no-cache without reinstall", got)
		}
	})

	t.Run("requirements sync", func(t *testing.T) {
		config := executor.Config{}
		for _, option := range syncRequirementsOptions("/venv/bin/python", "/requirements.txt", SyncRequirementsOptions{NoCache: true}) {
			option(&config)
		}
		want := []string{
			"pip", "sync", "--no-progress", "--python", "/venv/bin/python", "--require-hashes", "--no-cache", "/requirements.txt",
		}
		if strings.Join(config.Args, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("sync arguments = %q, want %q", config.Args, want)
		}
	})
}

func TestNormalInstallKeepsUVCache(t *testing.T) {
	if got := installPythonArgs("3.12.13", "/srv/python/release", false, false); slices.Contains(got, "--no-cache") {
		t.Fatalf("normal Python install arguments unexpectedly bypass cache: %q", got)
	}
	config := executor.Config{}
	for _, option := range syncRequirementsOptions("/venv/bin/python", "/requirements.txt", SyncRequirementsOptions{}) {
		option(&config)
	}
	if slices.Contains(config.Args, "--no-cache") {
		t.Fatalf("normal sync arguments unexpectedly bypass cache: %q", config.Args)
	}
}
