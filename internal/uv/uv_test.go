package uv

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/internal/runtime"
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
	data, err := os.ReadFile(filepath.Join("..", "..", ".uv-version"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != runtime.UVVersion {
		t.Fatalf(".uv-version = %q, runtime.UVVersion = %q", got, runtime.UVVersion)
	}
}
