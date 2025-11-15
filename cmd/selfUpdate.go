package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/runtime"
	"github.com/saltyorg/sb-go/internal/spinners"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

// Debug flag to enable verbose output
var debug bool

// Auto-accept flag to skip confirmation
var autoAccept bool

// Force update flag to bypass DisableSelfUpdate build flag
var forceUpdate bool

// selfUpdateCmd represents the selfUpdate command
var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update Saltbox CLI",
	Long:  `Update Saltbox CLI`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if self-update is disabled at build time (unless the force flag is used)
		if runtime.DisableSelfUpdate == "true" && !forceUpdate {
			_ = spinners.RunWarningSpinner("Self-update is disabled in this build")
			if runtime.DisableSelfUpdate == "true" {
				_ = spinners.RunInfoSpinner("Use --force-update to override this restriction")
			}
			return nil
		}
		_, err := doSelfUpdate(autoAccept, debug, "", forceUpdate)
		return err
	},
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
	selfUpdateCmd.Flags().BoolVarP(&debug, "verbose", "v", false, "Enable verbose debug output")
	selfUpdateCmd.Flags().BoolVarP(&autoAccept, "yes", "y", false, "Automatically accept update without confirmation")

	// Only add a force-update flag if self-update is disabled at build time
	if runtime.DisableSelfUpdate == "true" {
		selfUpdateCmd.Flags().BoolVar(&forceUpdate, "force-update", false, "Force update even when self-update is disabled")
	}
}

// promptForConfirmation asks the user for confirmation (y/n)
func promptForConfirmation(prompt string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/n]: ", prompt)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("error reading input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}

func doSelfUpdate(autoUpdate bool, verbose bool, optionalMessage string, force bool) (bool, error) {
	// Check if self-update is disabled at build time (unless force is true)
	if runtime.DisableSelfUpdate == "true" && !force {
		if verbose {
			fmt.Println("Debug: Self-update is disabled (build flag)")
		} else {
			_ = spinners.RunWarningSpinner("Self-update is disabled in this build")
		}
		return false, nil
	}

	// Log if force update is being used
	if force && runtime.DisableSelfUpdate == "true" {
		if verbose {
			fmt.Println("Debug: Force update flag is active, bypassing DisableSelfUpdate build flag")
		} else {
			_ = spinners.RunInfoSpinner("Forcing self-update despite build configuration")
		}
	}

	if verbose {
		fmt.Println("Debug: Starting self-update process")
		fmt.Printf("Debug: Current version: %s\n", runtime.Version)
		fmt.Printf("Debug: Current git commit: %s\n", runtime.GitCommit)
		fmt.Printf("Debug: Looking for updates in repository: saltyorg/sb-go\n")
		fmt.Printf("Debug: Auto-update mode: %t\n", autoUpdate)
		//selfupdate.EnableLog()
	}

	v := semver.MustParse(runtime.Version)

	if verbose {
		fmt.Printf("Debug: Parsed semver version: %s\n", v.String())
		fmt.Println("Debug: Checking for latest release from GitHub via Saltbox proxy")
	}

	// Create the Saltbox proxy source
	proxySource := NewSaltboxProxySource("https://svm.saltbox.dev/version")

	// First, check if an update is available without applying it
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: proxySource,
	})
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error creating updater: %v\n", err)
		}
		return false, fmt.Errorf("error creating updater: %w", err)
	}

	latest, found, err := updater.DetectLatest(context.Background(), selfupdate.ParseSlug("saltyorg/sb-go"))
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error checking for updates: %v\n", err)
		}
		return false, fmt.Errorf("error checking for updates: %w", err)
	}

	if !found || latest.Version() == v.String() {
		if verbose {
			fmt.Println("Debug: No update available - current version is the latest")
		}
		_ = spinners.RunInfoSpinner(fmt.Sprintf("Current binary is the latest version: %s", runtime.Version))
		return false, nil
	}

	// An update is available
	_ = spinners.RunInfoSpinner(fmt.Sprintf("New sb CLI version available: %s (current: %s)", latest.Version(), v))

	// If autoUpdate is false, ask for confirmation
	if !autoUpdate {
		confirmed, err := promptForConfirmation("Do you want to update")
		if err != nil {
			return false, err
		}
		if !confirmed {
			_ = spinners.RunWarningSpinner("Update of sb CLI cancelled")
			fmt.Println()
			return false, nil
		}
	} else if verbose {
		fmt.Println("Debug: Auto-update enabled, proceeding without confirmation")
	}

	// User confirmed or auto-update enabled, proceed with update
	exe, err := os.Executable()
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Error getting executable path: %v\n", err)
		}
		return false, fmt.Errorf("error getting executable path: %w", err)
	}

	err = updater.UpdateTo(context.Background(), latest, exe)
	if err != nil {
		if verbose {
			fmt.Printf("Debug: Update failed with error: %v\n", err)
		}
		return false, fmt.Errorf("binary update failed: %w", err)
	}

	if verbose {
		fmt.Printf("Debug: Update successful - previous version: %s, new version: %s\n", v, latest.Version())
	}
	_ = spinners.RunInfoSpinner(fmt.Sprintf("Successfully updated sb CLI to version: %s", latest.Version()))

	// Print an optional message if provided
	if optionalMessage != "" {
		_ = spinners.RunWarningSpinner(optionalMessage)
	}
	fmt.Println("")
	return true, nil
}

// SaltboxProxySource implements the go-selfupdate Source interface
// to route GitHub API calls through the Saltbox version proxy
type SaltboxProxySource struct {
	proxyBaseURL string
	httpClient   *http.Client
}

// NewSaltboxProxySource creates a new Saltbox proxy source
func NewSaltboxProxySource(proxyBaseURL string) *SaltboxProxySource {
	return &SaltboxProxySource{
		proxyBaseURL: proxyBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// githubRelease represents the GitHub API release response
type githubRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int    `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ListReleases fetches releases through the Saltbox proxy
func (s *SaltboxProxySource) ListReleases(ctx context.Context, repository selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	// Get repository owner and name
	owner, name, err := repository.GetSlug()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository slug: %w", err)
	}

	// Construct the GitHub API URL for releases
	githubAPIURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, name)

	// Construct the proxied URL
	proxyURL := fmt.Sprintf("%s?url=%s", s.proxyBaseURL, githubAPIURL)

	// Make the request
	req, err := http.NewRequestWithContext(ctx, "GET", proxyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("proxy returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var githubReleases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&githubReleases); err != nil {
		return nil, fmt.Errorf("failed to decode releases: %w", err)
	}

	// Convert to SourceRelease format
	releases := make([]selfupdate.SourceRelease, 0, len(githubReleases))
	for _, ghRelease := range githubReleases {
		releases = append(releases, newSaltboxRelease(ghRelease))
	}

	return releases, nil
}

// DownloadReleaseAsset downloads the actual release asset
// Note: This downloads directly, not through the proxy
func (s *SaltboxProxySource) DownloadReleaseAsset(ctx context.Context, rel *selfupdate.Release, assetID int64) (io.ReadCloser, error) {
	// Get the download URL from the release's validated asset
	downloadURL := rel.AssetURL

	if downloadURL == "" {
		return nil, fmt.Errorf("no asset URL found in release")
	}

	// Download the asset directly (not through proxy)
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// saltboxRelease wraps githubRelease to implement SourceRelease interface
type saltboxRelease struct {
	release githubRelease
}

func newSaltboxRelease(ghRelease githubRelease) *saltboxRelease {
	return &saltboxRelease{release: ghRelease}
}

func (r *saltboxRelease) GetID() int64 {
	return r.release.ID
}

func (r *saltboxRelease) GetTagName() string {
	return r.release.TagName
}

func (r *saltboxRelease) GetDraft() bool {
	return r.release.Draft
}

func (r *saltboxRelease) GetPrerelease() bool {
	return r.release.Prerelease
}

func (r *saltboxRelease) GetPublishedAt() time.Time {
	t, _ := time.Parse(time.RFC3339, r.release.PublishedAt)
	return t
}

func (r *saltboxRelease) GetReleaseNotes() string {
	return r.release.Body
}

func (r *saltboxRelease) GetName() string {
	return r.release.Name
}

func (r *saltboxRelease) GetURL() string {
	return r.release.HTMLURL
}

func (r *saltboxRelease) GetAssets() []selfupdate.SourceAsset {
	assets := make([]selfupdate.SourceAsset, 0, len(r.release.Assets))
	for _, asset := range r.release.Assets {
		assets = append(assets, newSaltboxAsset(asset))
	}
	return assets
}

// saltboxAsset wraps githubAsset to implement SourceAsset interface
type saltboxAsset struct {
	asset githubAsset
}

func newSaltboxAsset(ghAsset githubAsset) *saltboxAsset {
	return &saltboxAsset{asset: ghAsset}
}

func (a *saltboxAsset) GetID() int64 {
	return a.asset.ID
}

func (a *saltboxAsset) GetName() string {
	return a.asset.Name
}

func (a *saltboxAsset) GetSize() int {
	return a.asset.Size
}

func (a *saltboxAsset) GetBrowserDownloadURL() string {
	return a.asset.BrowserDownloadURL
}
