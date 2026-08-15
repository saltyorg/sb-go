package selfupdate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
)

const testAssetDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

func releaseListJSON(version string) string {
	return `[{"id":1,"tag_name":"` + version + `","assets":[{"id":2,"name":"sb_linux_amd64","size":123,"digest":"` + testAssetDigest + `","browser_download_url":"https://example.com/sb"}]}]`
}

func TestNewSourceRequiresHTTPS(t *testing.T) {
	if _, err := NewSource("http://svm.example/version", false, nil); err == nil {
		t.Fatal("NewSource accepted a plaintext HTTP proxy")
	}
}

func TestSourceListUsesProxy(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseListJSON("v1.2.3")))
	}))
	defer proxy.Close()
	directCalled := false
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directCalled = true
		_, _ = w.Write([]byte(releaseListJSON("v9.9.9")))
	}))
	defer direct.Close()

	source, err := NewSource(proxy.URL, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.GitHubURL = direct.URL
	source.Client = proxy.Client()
	source.Client.Timeout = time.Second
	releases, err := source.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "v1.2.3" {
		t.Fatalf("unexpected proxy releases: %+v", releases)
	}
	if directCalled {
		t.Fatal("direct GitHub fallback was called")
	}
}

func TestSourceListFallsBack(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"invalid":true}`))
	}))
	defer proxy.Close()
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseListJSON("v2.0.0")))
	}))
	defer direct.Close()

	source, err := NewSource(proxy.URL, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.GitHubURL = direct.URL
	// Both test servers use separate certificates, so use an explicit transport
	// that trusts their generated roots.
	pool := x509.NewCertPool()
	pool.AddCert(proxy.Certificate())
	pool.AddCert(direct.Certificate())
	source.Client = &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	releases, err := source.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "v2.0.0" {
		t.Fatalf("unexpected fallback releases: %+v", releases)
	}
}

func TestSourceListFallsBackOnUsableJSONWithUnusableRelease(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"tag_name":"","assets":[]}]`))
	}))
	defer proxy.Close()
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseListJSON("v2.0.0")))
	}))
	defer direct.Close()

	source, err := NewSource(proxy.URL, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.GitHubURL = direct.URL
	pool := x509.NewCertPool()
	pool.AddCert(proxy.Certificate())
	pool.AddCert(direct.Certificate())
	source.Client = &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}

	releases, err := source.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].TagName != "v2.0.0" {
		t.Fatalf("unexpected fallback releases: %+v", releases)
	}
}

func TestSourceListFallsBackWhenNewestProxyAssetLacksDigest(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"tag_name":"v2.0.0","assets":[{"id":2,"name":"sb_linux_amd64","size":123,"browser_download_url":"https://example.com/sb"}]}]`))
	}))
	defer proxy.Close()
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseListJSON("v2.0.0")))
	}))
	defer direct.Close()

	source, err := NewSource(proxy.URL, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.GitHubURL = direct.URL
	pool := x509.NewCertPool()
	pool.AddCert(proxy.Certificate())
	pool.AddCert(direct.Certificate())
	source.Client = &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}

	releases, err := source.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].Assets[0].Digest == "" {
		t.Fatalf("proxy response did not fall back to digest-bearing metadata: %+v", releases)
	}
}

func TestSourceListRejectsUnusableDirectFallback(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"tag_name":"","assets":[]}]`))
	}))
	defer proxy.Close()
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":2,"tag_name":"v2.0.0","assets":[]}]`))
	}))
	defer direct.Close()

	source, err := NewSource(proxy.URL, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	source.GitHubURL = direct.URL
	pool := x509.NewCertPool()
	pool.AddCert(proxy.Certificate())
	pool.AddCert(direct.Certificate())
	source.Client = &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}

	if _, err := source.List(context.Background()); err == nil || !strings.Contains(err.Error(), "fallback GitHub API request failed") {
		t.Fatalf("List() error = %v, want unusable fallback failure", err)
	}
}

type recordingReporter struct {
	mu       sync.Mutex
	messages []string
}

func (r *recordingReporter) Info(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, message)
}

func (r *recordingReporter) Warning(message string) { r.Info(message) }

func (r *recordingReporter) contains(want string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, message := range r.messages {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

func TestSourceVerboseReportsSourceSelection(t *testing.T) {
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseListJSON("v1.2.3")))
	}))
	defer proxy.Close()
	reporter := &recordingReporter{}
	source, err := NewSource(proxy.URL, true, reporter)
	if err != nil {
		t.Fatal(err)
	}
	source.Client = proxy.Client()
	if _, err := source.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reporter.contains("Debug: requesting release metadata") || !reporter.contains("metadata is usable") {
		t.Fatalf("verbose messages = %v", reporter.messages)
	}
}

func TestSelectRequiresDigestOnNewestRelease(t *testing.T) {
	current := semver.MustParse("1.0.0")
	releases := []Release{
		{
			TagName: "v2.0.0",
			Assets: []Asset{{
				ID: 1, Name: "sb_linux_amd64", Size: 10,
				BrowserDownloadURL: "https://example.com/sb",
			}},
		},
		{
			TagName: "v1.5.0",
			Assets: []Asset{{
				ID: 2, Name: "sb_linux_amd64", Size: 10, Digest: testAssetDigest,
				BrowserDownloadURL: "https://example.com/sb",
			}},
		},
	}
	if _, _, err := Select(releases, current); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("Select() error = %v, want missing digest rejection", err)
	}
}

func TestSelectIgnoresDraftsAndPrereleases(t *testing.T) {
	current := semver.MustParse("1.0.0")
	releases := []Release{
		{TagName: "v9.0.0", Draft: true},
		{TagName: "v8.0.0", Prerelease: true},
		{
			TagName: "v2.0.0",
			Assets: []Asset{{
				ID: 2, Name: "sb_linux_amd64", Size: 10, Digest: testAssetDigest,
				BrowserDownloadURL: "https://example.com/sb",
			}},
		},
	}
	candidate, found, err := Select(releases, current)
	if err != nil {
		t.Fatal(err)
	}
	if !found || candidate.Version.String() != "2.0.0" {
		t.Fatalf("candidate = %+v, found = %t", candidate, found)
	}
}
