package saltbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/release"
	"github.com/saltyorg/sb-go/terminal"
)

func TestGetCurrentFactVersion(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "passes exact version argument and reads plain version",
			script: `#!/bin/sh
if [ "$#" -ne 1 ] || [ "$1" != "--version" ]; then
	exit 64
fi
printf '1.2.3\n'
`,
			want: "1.2.3",
		},
		{
			name: "reads legacy full JSON output",
			script: `#!/bin/sh
printf '%s\n' '{"groups":{},"ip":{"error_ipv4":null,"error_ipv6":null,"failed_ipv4":false,"failed_ipv6":false,"ipv6_check_error":null,"public_ip":"203.0.113.10","public_ipv6":"2001:db8::10"},"saltbox_facts_version":"1.0.9","timezone":{"timezone":"UTC"},"users":{}}'
`,
			want: "1.0.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binaryPath := filepath.Join(t.TempDir(), "saltbox-facts")
			if err := os.WriteFile(binaryPath, []byte(tt.script), 0o755); err != nil {
				t.Fatalf("write test executable: %v", err)
			}

			got, err := getCurrentFactVersion(t.Context(), binaryPath)
			if err != nil {
				t.Fatalf("getCurrentFactVersion() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("getCurrentFactVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFactVersionOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{name: "plain version", output: " 1.2.3\n", want: "1.2.3"},
		{name: "v-prefixed version", output: "v2.3.4\n", want: "v2.3.4"},
		{name: "legacy JSON", output: `{"saltbox_facts_version":"1.0.9","groups":{},"users":{}}`, want: "1.0.9"},
		{name: "empty output", output: " \n\t", wantErr: true},
		{name: "malformed JSON", output: `{"saltbox_facts_version":`, wantErr: true},
		{name: "missing JSON version", output: `{"groups":{},"users":{}}`, wantErr: true},
		{name: "empty JSON version", output: `{"saltbox_facts_version":""}`, wantErr: true},
		{name: "non-string JSON version", output: `{"saltbox_facts_version":109}`, wantErr: true},
		{name: "invalid plain output", output: "not-a-version", wantErr: true},
		{name: "multi-line plain output", output: "1.2.3\nunexpected", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFactVersionOutput([]byte(tt.output))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseFactVersionOutput() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFactVersionOutput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseFactVersionOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
