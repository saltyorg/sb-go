package fact

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saltyorg/sb-go/internal/spinners"
)

func TestFetchLatestReleaseInfoFromURL(t *testing.T) {
	t.Run("returns version and size for valid release", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"saltbox-facts","size":12345}]}`))
		}))
		defer server.Close()

		version, size, err := fetchLatestReleaseInfoFromURL(server.Client(), server.URL)
		if err != nil {
			t.Fatalf("fetchLatestReleaseInfoFromURL() returned error: %v", err)
		}
		if version != "v1.2.3" {
			t.Fatalf("expected version v1.2.3, got %q", version)
		}
		if size != 12345 {
			t.Fatalf("expected size 12345, got %d", size)
		}
	})

	t.Run("rejects missing tag_name", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"","assets":[{"name":"saltbox-facts","size":12345}]}`))
		}))
		defer server.Close()

		_, _, err := fetchLatestReleaseInfoFromURL(server.Client(), server.URL)
		if err == nil {
			t.Fatal("expected error for missing tag_name")
		}
	})

	t.Run("rejects missing expected asset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"other","size":12345}]}`))
		}))
		defer server.Close()

		_, _, err := fetchLatestReleaseInfoFromURL(server.Client(), server.URL)
		if err == nil {
			t.Fatal("expected error for missing saltbox-facts asset")
		}
	})
}

func TestFetchLatestReleaseInfoFallback(t *testing.T) {
	originalVerboseMode := spinners.VerboseMode
	spinners.VerboseMode = true
	t.Cleanup(func() {
		spinners.VerboseMode = originalVerboseMode
	})

	t.Run("uses proxy response when usable", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","assets":[{"name":"saltbox-facts","size":222}]}`))
		}))
		defer proxy.Close()

		githubCalled := false
		github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			githubCalled = true
			_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","assets":[{"name":"saltbox-facts","size":999}]}`))
		}))
		defer github.Close()

		version, size, err := fetchLatestReleaseInfo(proxy.URL, github.URL, true)
		if err != nil {
			t.Fatalf("fetchLatestReleaseInfo() returned error: %v", err)
		}
		if version != "v2.0.0" || size != 222 {
			t.Fatalf("expected proxy result v2.0.0/222, got %q/%d", version, size)
		}
		if githubCalled {
			t.Fatal("expected fallback GitHub URL not to be called when proxy is usable")
		}
	})

	t.Run("falls back to direct GitHub API when proxy response is unusable", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"","assets":[]}`))
		}))
		defer proxy.Close()

		github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v3.1.4","assets":[{"name":"saltbox-facts","size":314}]}`))
		}))
		defer github.Close()

		version, size, err := fetchLatestReleaseInfo(proxy.URL, github.URL, true)
		if err != nil {
			t.Fatalf("fetchLatestReleaseInfo() returned error: %v", err)
		}
		if version != "v3.1.4" || size != 314 {
			t.Fatalf("expected fallback result v3.1.4/314, got %q/%d", version, size)
		}
	})
}
