package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/creativeprojects/go-selfupdate"
)

type fakeSelfUpdateSource struct {
	releases []selfupdate.SourceRelease
	err      error
	calls    int
}

func (f *fakeSelfUpdateSource) ListReleases(_ context.Context, _ selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

func (f *fakeSelfUpdateSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func validSourceReleases() []selfupdate.SourceRelease {
	return []selfupdate.SourceRelease{
		newSaltboxRelease(githubRelease{
			ID:      123,
			TagName: "v1.2.3",
			Assets: []githubAsset{
				{
					ID:                 456,
					Name:               "sb_linux_amd64",
					BrowserDownloadURL: "https://example.com/sb_linux_amd64",
				},
			},
		}),
	}
}

func TestSaltboxProxySourceListReleasesUsesProxyWhenUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"tag_name":"v1.0.0","assets":[{"id":2,"name":"sb_linux_amd64","browser_download_url":"https://example.com/sb"}]}]`))
	}))
	defer server.Close()

	fallback := &fakeSelfUpdateSource{releases: validSourceReleases()}
	source := &SaltboxProxySource{
		proxyBaseURL: server.URL,
		httpClient:   &http.Client{Timeout: time.Second},
		githubSource: fallback,
		verbose:      true,
	}

	releases, err := source.ListReleases(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err != nil {
		t.Fatalf("ListReleases() returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if fallback.calls != 0 {
		t.Fatalf("expected fallback source not to be used, got %d calls", fallback.calls)
	}
}

func TestSaltboxProxySourceListReleasesFallsBackOnProxyError(t *testing.T) {
	fallback := &fakeSelfUpdateSource{releases: validSourceReleases()}
	source := &SaltboxProxySource{
		proxyBaseURL: "http://127.0.0.1:1",
		httpClient:   &http.Client{Timeout: 100 * time.Millisecond},
		githubSource: fallback,
		verbose:      true,
	}

	releases, err := source.ListReleases(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err != nil {
		t.Fatalf("ListReleases() returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected fallback releases, got %d", len(releases))
	}
	if fallback.calls != 1 {
		t.Fatalf("expected fallback source to be called once, got %d", fallback.calls)
	}
}

func TestSaltboxProxySourceListReleasesFallsBackOnUnusableProxyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"tag_name":"","assets":[]}]`))
	}))
	defer server.Close()

	fallback := &fakeSelfUpdateSource{releases: validSourceReleases()}
	source := &SaltboxProxySource{
		proxyBaseURL: server.URL,
		httpClient:   &http.Client{Timeout: time.Second},
		githubSource: fallback,
		verbose:      true,
	}

	releases, err := source.ListReleases(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err != nil {
		t.Fatalf("ListReleases() returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected fallback releases, got %d", len(releases))
	}
	if fallback.calls != 1 {
		t.Fatalf("expected fallback source to be called once, got %d", fallback.calls)
	}
}

func TestSaltboxProxySourceListReleasesErrorsWhenBothSourcesFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not":"a release list"}`))
	}))
	defer server.Close()

	fallback := &fakeSelfUpdateSource{err: errors.New("fallback failed")}
	source := &SaltboxProxySource{
		proxyBaseURL: server.URL,
		httpClient:   &http.Client{Timeout: time.Second},
		githubSource: fallback,
		verbose:      true,
	}

	_, err := source.ListReleases(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err == nil {
		t.Fatal("expected an error when both proxy and fallback fail")
	}
	if !strings.Contains(err.Error(), "fallback GitHub API request failed") {
		t.Fatalf("expected fallback failure in error, got %v", err)
	}
}

func TestReleaseListUsabilityError(t *testing.T) {
	if err := releaseListUsabilityError(validSourceReleases()); err != nil {
		t.Fatalf("expected valid releases to pass usability check, got %v", err)
	}

	err := releaseListUsabilityError([]selfupdate.SourceRelease{
		newSaltboxRelease(githubRelease{
			TagName: "",
			Assets: []githubAsset{
				{Name: "sb_linux_amd64", BrowserDownloadURL: "https://example.com/sb"},
			},
		}),
	})
	if err == nil {
		t.Fatal("expected missing tag_name to fail usability check")
	}
}
