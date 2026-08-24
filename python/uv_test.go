package python

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestUVXBinaryPath(t *testing.T) {
	const expected = "/usr/local/bin/uvx"
	if UVXBinaryPath != expected {
		t.Fatalf("UVXBinaryPath = %q, want %q", UVXBinaryPath, expected)
	}
}

func TestExtractUVBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "uv.tar.gz")
	wantUV := []byte("test uv binary")
	wantUVX := []byte("test uvx binary")

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range map[string][]byte{"uv": wantUV, "uvx": wantUVX} {
		header := &tar.Header{Name: "uv-x86_64-unknown-linux-gnu/" + name, Mode: 0755, Size: int64(len(content))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
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

	for name, want := range map[string][]byte{"uv": wantUV, "uvx": wantUVX} {
		destination := filepath.Join(dir, name)
		if err := os.WriteFile(destination, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := extractUVBinary(archivePath, destination, name, false); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("extracted %s = %q, want %q", name, got, want)
		}
	}
}

func TestEnsureVersionRejectsFloatingVersion(t *testing.T) {
	err := EnsureVersion(context.Background(), "latest", filepath.Join(t.TempDir(), "uv"), false)
	if err == nil || !strings.Contains(err.Error(), "exact release") {
		t.Fatalf("EnsureVersion() error = %v, want exact-release rejection", err)
	}
}

func TestEnsureVersionInstallsUVPair(t *testing.T) {
	const version = "1.2.3"
	archive := uvTestArchive(t, map[string][]byte{
		"uv":  []byte("#!/bin/sh\necho 'uv 1.2.3'\n"),
		"uvx": []byte("#!/bin/sh\nuv=\"$(dirname \"$0\")/uv\"\nversion=\"$(\"$uv\" --version)\" || exit $?\nprintf 'uvx %s\\n' \"${version#uv }\"\n"),
	})
	digest := sha256.Sum256(archive)
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		switch request.URL.Path {
		case "/metadata":
			_, _ = fmt.Fprintf(writer, `{"assets":[{"id":1,"name":"uv-x86_64-unknown-linux-gnu.tar.gz","size":%d,"digest":"sha256:%x","browser_download_url":"%s/archive"}]}`, len(archive), digest, serverURL(request))
		case "/archive":
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	uvPath := filepath.Join(dir, "uv")
	uvxPath := filepath.Join(dir, "uvx")

	if err := ensureVersion(context.Background(), version, uvPath, uvxPath, false, server.URL+"/metadata", server.Client()); err != nil {
		t.Fatal(err)
	}
	if got, err := binaryVersion(context.Background(), uvPath, "uv"); err != nil || got != version {
		t.Fatalf("uv version = %q, %v", got, err)
	}
	if got, err := binaryVersion(context.Background(), uvxPath, "uvx"); err != nil || got != version {
		t.Fatalf("uvx version = %q, %v", got, err)
	}

	requestsBeforeNoop := requests
	if err := ensureVersion(context.Background(), version, uvPath, uvxPath, false, server.URL+"/metadata", server.Client()); err != nil {
		t.Fatal(err)
	}
	if requests != requestsBeforeNoop {
		t.Fatalf("healthy uv pair triggered %d additional HTTP requests", requests-requestsBeforeNoop)
	}
}

func uvTestArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		header := &tar.Header{Name: "uv-x86_64-unknown-linux-gnu/" + name, Mode: 0755, Size: int64(len(content))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func serverURL(request *http.Request) string {
	return "https://" + request.Host
}

func TestRuntimeUVVersionIsExact(t *testing.T) {
	if got := buildinfo.Current().UVVersion; !exactUVVersionPattern.MatchString(got) {
		t.Fatalf("runtime uv version = %q, want exact major.minor.patch release", got)
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
