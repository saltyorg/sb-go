package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/release"

	"github.com/Masterminds/semver/v3"
)

type Reporter interface {
	Info(string)
	Warning(string)
}

type Release struct {
	ID         int64   `json:"id"`
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Candidate struct {
	Version *semver.Version
	Asset   release.Asset
}

type Source struct {
	ProxyBaseURL string
	GitHubURL    string
	Client       *http.Client
	Verbose      bool
	Reporter     Reporter
	warnOnce     sync.Once
	successOnce  sync.Once
}

func NewSource(proxyBaseURL string, verbose bool, reporter Reporter) (*Source, error) {
	parsed, err := url.Parse(proxyBaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q", proxyBaseURL)
	}
	return &Source{
		ProxyBaseURL: proxyBaseURL,
		GitHubURL:    "https://api.github.com/repos/saltyorg/sb-go/releases",
		Client:       release.HTTPClient(30 * time.Second),
		Verbose:      verbose,
		Reporter:     reporter,
	}, nil
}

func (s *Source) List(ctx context.Context) ([]Release, error) {
	proxyURL := fmt.Sprintf("%s?url=%s", s.ProxyBaseURL, url.QueryEscape(s.GitHubURL))
	s.debugf("requesting release metadata through the SVM proxy")
	releases, proxyErr := s.list(ctx, proxyURL)
	if proxyErr == nil {
		s.debugf("SVM proxy release metadata is usable")
		return releases, nil
	}
	s.warnOnce.Do(func() {
		if s.Reporter != nil && !s.Verbose {
			s.Reporter.Warning(fmt.Sprintf("SVM proxy %s; trying GitHub directly", release.Describe(proxyErr)))
		}
	})
	s.debugf("SVM proxy unavailable or unusable (%v); trying GitHub directly", proxyErr)
	releases, githubErr := s.list(ctx, s.GitHubURL)
	if githubErr != nil {
		return nil, fmt.Errorf("%w; fallback GitHub API request failed: %w", proxyErr, githubErr)
	}
	s.successOnce.Do(func() {
		if s.Reporter != nil && !s.Verbose {
			s.Reporter.Info("GitHub fallback succeeded")
		}
	})
	s.debugf("direct GitHub API fallback succeeded")
	return releases, nil
}

func (s *Source) debugf(format string, args ...any) {
	if s.Verbose && s.Reporter != nil {
		s.Reporter.Info("Debug: " + fmt.Sprintf(format, args...))
	}
}

func (s *Source) list(ctx context.Context, endpoint string) ([]Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return nil, fmt.Errorf("%w: %s", release.HTTPStatus(resp.StatusCode), strings.TrimSpace(string(body)))
	}
	var releases []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return nil, release.InvalidResponse("returned invalid JSON", err)
	}
	if len(releases) == 0 {
		return nil, release.InvalidResponse("returned no releases", nil)
	}
	if err := releaseListUsabilityError(releases); err != nil {
		return nil, release.InvalidResponse("returned unusable release metadata", err)
	}
	return releases, nil
}

func releaseListUsabilityError(releases []Release) error {
	var newest *Release
	var newestVersion *semver.Version
	for i := range releases {
		candidate := &releases[i]
		if candidate.Draft || candidate.Prerelease {
			continue
		}
		version, err := semver.NewVersion(strings.TrimPrefix(strings.TrimSpace(candidate.TagName), "v"))
		if err != nil {
			continue
		}
		if newestVersion == nil || version.GreaterThan(newestVersion) {
			newest = candidate
			newestVersion = version
		}
	}
	if newest == nil {
		return fmt.Errorf("no stable releases with valid semantic versions")
	}
	for _, raw := range newest.Assets {
		if raw.Name != "sb_linux_amd64" {
			continue
		}
		if _, err := release.NewAsset(raw.ID, raw.Name, raw.BrowserDownloadURL, raw.Size, raw.Digest); err != nil {
			return fmt.Errorf("release %s has unusable sb_linux_amd64 metadata: %w", newest.TagName, err)
		}
		return nil
	}
	return fmt.Errorf("release %s is missing sb_linux_amd64", newest.TagName)
}

func Select(releases []Release, current *semver.Version) (Candidate, bool, error) {
	var newest *Release
	var newestVersion *semver.Version
	for i := range releases {
		candidate := &releases[i]
		if candidate.Draft || candidate.Prerelease {
			continue
		}
		version, err := semver.NewVersion(strings.TrimPrefix(strings.TrimSpace(candidate.TagName), "v"))
		if err != nil {
			continue
		}
		if newestVersion == nil || version.GreaterThan(newestVersion) {
			newest = candidate
			newestVersion = version
		}
	}
	if newest == nil || !newestVersion.GreaterThan(current) {
		return Candidate{}, false, nil
	}
	for _, raw := range newest.Assets {
		if raw.Name != "sb_linux_amd64" {
			continue
		}
		asset, err := release.NewAsset(raw.ID, raw.Name, raw.BrowserDownloadURL, raw.Size, raw.Digest)
		if err != nil {
			return Candidate{}, false, fmt.Errorf("release %s has unusable sb_linux_amd64 metadata: %w", newest.TagName, err)
		}
		return Candidate{Version: newestVersion, Asset: asset}, true, nil
	}
	return Candidate{}, false, fmt.Errorf("release %s is missing sb_linux_amd64", newest.TagName)
}

type Options struct {
	BuildInfo       buildinfo.Info
	AutoAccept      bool
	OptionalMessage string
	Force           bool
	Confirm         func(string) (bool, error)
	Executable      func() (string, error)
}

func Run(ctx context.Context, source *Source, reporter Reporter, options Options) (bool, error) {
	if reporter == nil {
		return false, fmt.Errorf("self-update reporter is required")
	}
	if options.BuildInfo.DisableSelfUpdate && !options.Force {
		reporter.Warning("Self-update is disabled in this build")
		return false, nil
	}
	if options.Force && options.BuildInfo.DisableSelfUpdate {
		reporter.Info("Forcing self-update despite build configuration")
	}
	source.debugf("starting self-update process")
	source.debugf("current version: %s", options.BuildInfo.Version)
	source.debugf("current git commit: %s", options.BuildInfo.GitCommit)
	source.debugf("looking for updates in repository: saltyorg/sb-go")
	source.debugf("auto-update mode: %t", options.AutoAccept)
	current, err := semver.NewVersion(options.BuildInfo.Version)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", options.BuildInfo.Version, err)
	}
	releases, err := source.List(ctx)
	if err != nil {
		return false, fmt.Errorf("check for updates: %w", err)
	}
	latest, found, err := Select(releases, current)
	if err != nil {
		return false, fmt.Errorf("check for updates: %w", err)
	}
	if !found {
		source.debugf("no update available; current version is the latest")
		reporter.Info(fmt.Sprintf("Current binary is the latest version: %s", options.BuildInfo.Version))
		return false, nil
	}
	if options.AutoAccept {
		source.debugf("auto-update enabled; proceeding without confirmation")
	}
	reporter.Info(fmt.Sprintf("New sb CLI version available: %s (current: %s)", latest.Version, current))
	if !options.AutoAccept {
		if options.Confirm == nil {
			return false, fmt.Errorf("update confirmation callback is required")
		}
		confirmed, err := options.Confirm("Do you want to update")
		if err != nil {
			return false, err
		}
		if !confirmed {
			reporter.Warning("Update of sb CLI cancelled")
			return false, nil
		}
	}
	executable := options.Executable
	if executable == nil {
		executable = os.Executable
	}
	target, err := executable()
	if err != nil {
		return false, fmt.Errorf("get executable path: %w", err)
	}
	source.debugf("downloading verified update for %s", latest.Version)
	staged, err := release.DownloadVerified(ctx, source.Client, latest.Asset, 128<<20, filepath.Dir(target), ".sb-update-*")
	if err != nil {
		return false, fmt.Errorf("binary update failed: %w", err)
	}
	defer func() { _ = os.Remove(staged) }()
	if err := release.ValidateLinuxAMD64(staged); err != nil {
		return false, fmt.Errorf("binary update failed validation: %w", err)
	}
	if err := release.Activate(staged, target, 0755); err != nil {
		return false, fmt.Errorf("binary update failed: %w", err)
	}
	source.debugf("update successful; previous version: %s, new version: %s", current, latest.Version)
	reporter.Info(fmt.Sprintf("Successfully updated sb CLI to version: %s", latest.Version))
	if options.OptionalMessage != "" {
		reporter.Warning(options.OptionalMessage)
	}
	return true, nil
}
