package saltbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/release"
	"github.com/saltyorg/sb-go/terminal"
)

func TestFetchLatestReleaseInfoFromURL(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0", 64)
	t.Run("returns version and size for valid release", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"id":1,"name":"saltbox-facts","size":12345,"digest":"` + digest + `","browser_download_url":"https://example.com/saltbox-facts"}]}`))
		}))
		defer server.Close()

		version, asset, err := fetchLatestReleaseInfoFromURL(context.Background(), server.Client(), server.URL)
		if err != nil {
			t.Fatalf("fetchLatestReleaseInfoFromURL() returned error: %v", err)
		}
		if version != "v1.2.3" {
			t.Fatalf("expected version v1.2.3, got %q", version)
		}
		if asset.Size != 12345 {
			t.Fatalf("expected size 12345, got %d", asset.Size)
		}
	})

	t.Run("rejects missing tag_name", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"","assets":[{"name":"saltbox-facts","size":12345}]}`))
		}))
		defer server.Close()

		_, _, err := fetchLatestReleaseInfoFromURL(context.Background(), server.Client(), server.URL)
		if err == nil {
			t.Fatal("expected error for missing tag_name")
		}
	})

	t.Run("rejects missing expected asset", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"other","size":12345}]}`))
		}))
		defer server.Close()

		_, _, err := fetchLatestReleaseInfoFromURL(context.Background(), server.Client(), server.URL)
		if err == nil {
			t.Fatal("expected error for missing saltbox-facts asset")
		}
	})
}

func TestFetchLatestReleaseInfoFallback(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0", 64)
	releaseJSON := func(version string, size int) string {
		return `{"tag_name":"` + version + `","assets":[{"id":1,"name":"saltbox-facts","size":` + fmt.Sprint(size) + `,"digest":"` + digest + `","browser_download_url":"https://example.com/saltbox-facts"}]}`
	}
	runFetch := func(proxyURL, githubURL string) (string, int64, error) {
		runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: io.Discard})
		var version string
		var size int64
		err := runner.Run(context.Background(), terminal.TaskSpec{Running: "test"}, func(ctx context.Context, task *terminal.Task) error {
			var err error
			var asset release.Asset
			version, asset, err = fetchLatestReleaseInfo(ctx, task, proxyURL, githubURL, true)
			size = asset.Size
			return err
		})
		return version, size, err
	}

	t.Run("uses proxy response when usable", func(t *testing.T) {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(releaseJSON("v2.0.0", 222)))
		}))
		defer proxy.Close()

		githubCalled := false
		github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			githubCalled = true
			_, _ = w.Write([]byte(releaseJSON("v9.9.9", 999)))
		}))
		defer github.Close()

		version, size, err := runFetch(proxy.URL, github.URL)
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
			_, _ = w.Write([]byte(releaseJSON("v3.1.4", 314)))
		}))
		defer github.Close()

		version, size, err := runFetch(proxy.URL, github.URL)
		if err != nil {
			t.Fatalf("fetchLatestReleaseInfo() returned error: %v", err)
		}
		if version != "v3.1.4" || size != 314 {
			t.Fatalf("expected fallback result v3.1.4/314, got %q/%d", version, size)
		}
	})
}
