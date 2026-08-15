package release

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadVerified(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	path, err := DownloadVerified(context.Background(), server.Client(), asset, 1024, t.TempDir(), ".asset-*")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestDownloadVerifiedRejectsMismatchAndPreservesTarget(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("malicious"))
	}))
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "binary")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	asset, err := NewAsset(1, "binary", server.URL, int64(len("malicious")), "sha256:"+strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadVerified(context.Background(), server.Client(), asset, 1024, dir, ".asset-*"); err == nil {
		t.Fatal("expected digest mismatch")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("target = %q, want old", got)
	}
}

func TestNewAssetRejectsMissingDigestAndHTTP(t *testing.T) {
	if _, err := NewAsset(1, "binary", "https://example.com/binary", 1, ""); err == nil {
		t.Fatal("expected missing digest error")
	}
	if _, err := NewAsset(1, "binary", "http://example.com/binary", 1, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected HTTP URL error")
	}
}

func TestHTTPClientRejectsPlaintextRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://example.com/binary", http.StatusFound)
	}))
	defer server.Close()
	client := HTTPClient(time.Second)
	client.Transport = server.Client().Transport
	if _, err := client.Get(server.URL); err == nil {
		t.Fatal("expected plaintext redirect rejection")
	}
}
