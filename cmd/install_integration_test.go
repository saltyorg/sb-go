package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/saltyorg/sb-go/internal/ansible"
	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/git"
	"github.com/spf13/cobra"
)

// isAnsiblePlaybookAvailable checks if ansible-playbook is installed
func isAnsiblePlaybookAvailable() bool {
	_, err := exec.LookPath(constants.AnsiblePlaybookBinaryPath)
	return err == nil
}

// MockGitExecutor for git operations
type MockGitExecutor struct {
	ExecuteFunc func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

func (m *MockGitExecutor) ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, dir, name, args...)
	}
	// Default: return a mock commit hash
	if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
		return []byte("abc123def456\n"), nil
	}
	return []byte{}, nil
}

// MockAnsibleExecutor for ansible operations
type MockAnsibleExecutor struct {
	ExecuteContextFunc func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	ExecuteWithIOFunc  func(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error
}

func (m *MockAnsibleExecutor) ExecuteContext(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	if m.ExecuteContextFunc != nil {
		return m.ExecuteContextFunc(ctx, dir, name, args...)
	}
	// Default: return mock ansible output
	return []byte("TASK TAGS: [tag1, tag2, tag3]"), nil
}

func (m *MockAnsibleExecutor) ExecuteWithIO(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
	if m.ExecuteWithIOFunc != nil {
		return m.ExecuteWithIOFunc(ctx, dir, name, args, stdout, stderr, stdin)
	}
	// Default: write mock output
	if stdout != nil {
		if w, ok := stdout.(interface{ Write([]byte) (int, error) }); ok {
			w.Write([]byte("mock playbook output\n"))
		}
	}
	return nil
}

// TestCacheExistsAndIsValid_Integration tests the actual function
func TestCacheExistsAndIsValid_Integration(t *testing.T) {
	tests := []struct {
		name       string
		setupCache func(*cache.Cache)
		repoPath   string
		verbosity  int
		expected   bool
	}{
		{
			name: "valid cache with tags",
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123",
					"tags":   []any{"tag1", "tag2", "tag3"},
				})
			},
			repoPath:  constants.SaltboxRepoPath,
			verbosity: 0,
			expected:  true,
		},
		{
			name: "cache with empty tags",
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123",
					"tags":   []any{},
				})
			},
			repoPath:  constants.SaltboxRepoPath,
			verbosity: 0,
			expected:  false,
		},
		{
			name: "cache without tags key",
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123",
				})
			},
			repoPath:  constants.SaltboxRepoPath,
			verbosity: 0,
			expected:  false,
		},
		{
			name:       "no cache",
			setupCache: func(c *cache.Cache) {},
			repoPath:   constants.SaltboxRepoPath,
			verbosity:  0,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")

			// Create cache instance - need to use NewCache or initialize properly
			c, err := cache.NewCache()
			if err != nil {
				// If we can't load from file, that's okay for tests
				t.Logf("Cache load failed (expected in tests): %v", err)
			}
			if c != nil {
				tt.setupCache(c)
			}

			// Save cache to file
			data, _ := json.Marshal(c)
			os.WriteFile(cacheFile, data, 0644)

			// Load cache from file
			loadedCache, err := cache.NewCache()
			if err != nil {
				// If cache creation fails, create a new one
				loadedCache = &cache.Cache{}
			}

			// Copy the setup data to the loaded cache
			if tt.setupCache != nil {
				tt.setupCache(loadedCache)
			}

			// Call the actual function
			result := cacheExistsAndIsValid(tt.repoPath, loadedCache, tt.verbosity)

			if result != tt.expected {
				t.Errorf("cacheExistsAndIsValid() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestGetValidTags_Integration tests the actual function with mocks
func TestGetValidTags_Integration(t *testing.T) {
	// Save original executors
	originalGitExecutor := git.GetExecutor()
	originalAnsibleExecutor := ansible.GetExecutor()
	defer func() {
		git.SetExecutor(originalGitExecutor)
		ansible.SetExecutor(originalAnsibleExecutor)
	}()

	tests := []struct {
		name        string
		repoPath    string
		setupCache  func(*cache.Cache)
		mockGitHash string
		mockTags    []string
		expectedLen int
		expectEmpty bool
	}{
		{
			name:     "valid cached tags",
			repoPath: constants.SaltboxRepoPath,
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123def456",
					"tags":   []any{"tag1", "tag2", "tag3"},
				})
			},
			mockGitHash: "abc123def456",
			mockTags:    []string{"tag1", "tag2", "tag3"},
			expectedLen: 3,
			expectEmpty: false,
		},
		{
			name:     "cache miss - needs update",
			repoPath: constants.SaltboxRepoPath,
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "old123",
					"tags":   []any{"oldtag1", "oldtag2"},
				})
			},
			mockGitHash: "abc123def456",
			mockTags:    []string{"newtag1", "newtag2", "newtag3"},
			expectedLen: 3,
			expectEmpty: false,
		},
		{
			name:        "no cache - fetch from ansible",
			repoPath:    constants.SaltboxRepoPath,
			setupCache:  func(c *cache.Cache) {},
			mockGitHash: "abc123def456",
			mockTags:    []string{"tag1", "tag2"},
			expectedLen: 2,
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock git executor
			mockGit := &MockGitExecutor{
				ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
						return []byte(tt.mockGitHash + "\n"), nil
					}
					return []byte{}, nil
				},
			}
			git.SetExecutor(mockGit)

			// Setup mock ansible executor
			mockAnsible := &MockAnsibleExecutor{
				ExecuteContextFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					// Return tags in ansible format
					tagsStr := ""
					for i, tag := range tt.mockTags {
						if i > 0 {
							tagsStr += ", "
						}
						tagsStr += tag
					}
					return []byte("TASK TAGS: [" + tagsStr + "]"), nil
				},
			}
			ansible.SetExecutor(mockAnsible)

			// Create cache
			c, err := cache.NewCache()
			if err != nil {
				t.Logf("Cache load failed (expected): %v", err)
			}
			if c != nil {
				tt.setupCache(c)
			}

			// Call the actual function
			tags := getValidTags(ctx, tt.repoPath, c, 0)

			if tt.expectEmpty && len(tags) != 0 {
				t.Errorf("Expected empty tags, got %v", tags)
			}

			if !tt.expectEmpty && len(tags) != tt.expectedLen {
				t.Errorf("getValidTags() returned %d tags, expected %d. Tags: %v", len(tags), tt.expectedLen, tags)
			}
		})
	}
}

// TestValidateAndSuggest_Integration tests the actual function with mocks
func TestValidateAndSuggest_Integration(t *testing.T) {
	// Save original executors
	originalGitExecutor := git.GetExecutor()
	originalAnsibleExecutor := ansible.GetExecutor()
	defer func() {
		git.SetExecutor(originalGitExecutor)
		ansible.SetExecutor(originalAnsibleExecutor)
	}()

	tests := []struct {
		name              string
		repoPath          string
		providedTags      []string
		currentPrefix     string
		otherPrefix       string
		saltboxTags       []string
		sandboxTags       []string
		expectedSuggCount int
	}{
		{
			name:              "valid tags - no suggestions",
			repoPath:          constants.SaltboxRepoPath,
			providedTags:      []string{"plex", "sonarr"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr", "radarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 0,
		},
		{
			name:              "invalid tag - suggestions generated",
			repoPath:          constants.SaltboxRepoPath,
			providedTags:      []string{"invalidtag"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr", "radarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 1,
		},
		{
			name:              "tag exists in other repo",
			repoPath:          constants.SaltboxRepoPath,
			providedTags:      []string{"overseerr"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 1,
		},
		{
			name:              "close match - typo",
			repoPath:          constants.SaltboxRepoPath,
			providedTags:      []string{"plx"}, // typo of "plex"
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr"},
			sandboxTags:       []string{"overseerr"},
			expectedSuggCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock git executor
			mockGit := &MockGitExecutor{
				ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					return []byte("abc123def456\n"), nil
				},
			}
			git.SetExecutor(mockGit)

			// Setup mock ansible executor
			mockAnsible := &MockAnsibleExecutor{
				ExecuteContextFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					// Determine which tags to return based on the working directory
					var tags []string
					// Use the dir parameter to determine which repo's tags to return
					tags = tt.saltboxTags
					if dir == constants.SandboxRepoPath {
						tags = tt.sandboxTags
					}

					tagsStr := ""
					for i, tag := range tags {
						if i > 0 {
							tagsStr += ", "
						}
						tagsStr += tag
					}
					return []byte("TASK TAGS: [" + tagsStr + "]"), nil
				},
			}
			ansible.SetExecutor(mockAnsible)

			// Create cache with pre-populated data
			c, err := cache.NewCache()
			if err != nil {
				t.Logf("Cache load failed (expected): %v", err)
			}
			if c != nil {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123def456",
					"tags":   convertToAnySlice(tt.saltboxTags),
				})
				c.SetRepoCache(constants.SandboxRepoPath, map[string]any{
					"commit": "abc123def456",
					"tags":   convertToAnySlice(tt.sandboxTags),
				})
			}

			// Call the actual function
			suggestions := validateAndSuggest(ctx, tt.repoPath, tt.providedTags, tt.currentPrefix, tt.otherPrefix, c, 0)

			if len(suggestions) != tt.expectedSuggCount {
				t.Errorf("validateAndSuggest() returned %d suggestions, expected %d. Suggestions: %v",
					len(suggestions), tt.expectedSuggCount, suggestions)
			}
		})
	}
}

// TestRunPlaybook_Integration tests the runPlaybook function structure
func TestRunPlaybook_Integration(t *testing.T) {
	if !isAnsiblePlaybookAvailable() {
		t.Skip("Skipping TestRunPlaybook_Integration: ansible-playbook not found in PATH")
	}

	// Save original executor
	originalAnsibleExecutor := ansible.GetExecutor()
	defer ansible.SetExecutor(originalAnsibleExecutor)

	tests := []struct {
		name              string
		repoPath          string
		playbookPath      string
		tags              []string
		ansibleBinaryPath string
		extraVars         []string
		skipTags          []string
		extraArgs         []string
		mockError         error
		expectError       bool
	}{
		{
			name:              "successful playbook run",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{},
			skipTags:          []string{},
			extraArgs:         []string{},
			mockError:         nil,
			expectError:       false,
		},
		{
			name:              "playbook with extra vars",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex", "sonarr"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{"var1=value1", "var2=value2"},
			skipTags:          []string{},
			extraArgs:         []string{},
			mockError:         nil,
			expectError:       false,
		},
		{
			name:              "playbook with skip tags",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{},
			skipTags:          []string{"always"},
			extraArgs:         []string{},
			mockError:         nil,
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock ansible executor
			mockAnsible := &MockAnsibleExecutor{
				ExecuteWithIOFunc: func(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
					// Write success output
					if stdout != nil {
						if w, ok := stdout.(interface{ Write([]byte) (int, error) }); ok {
							w.Write([]byte("PLAY RECAP\n"))
							w.Write([]byte("localhost: ok=5 changed=2\n"))
						}
					}
					return tt.mockError
				},
			}
			ansible.SetExecutor(mockAnsible)

			// Call the actual function with verbose=false to use mocks
			err := runPlaybook(ctx, tt.repoPath, tt.playbookPath, tt.tags, tt.ansibleBinaryPath, tt.extraVars, tt.skipTags, tt.extraArgs, false)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestHandleInstall_Integration tests parts of handleInstall with mocks
func TestHandleInstall_Integration(t *testing.T) {
	if !isAnsiblePlaybookAvailable() {
		t.Skip("Skipping TestHandleInstall_Integration: ansible-playbook not found in PATH")
	}

	// Save original executors
	originalGitExecutor := git.GetExecutor()
	originalAnsibleExecutor := ansible.GetExecutor()
	defer func() {
		git.SetExecutor(originalGitExecutor)
		ansible.SetExecutor(originalAnsibleExecutor)
	}()

	tests := []struct {
		name        string
		tags        []string
		expectError bool
	}{
		{
			name:        "empty tags",
			tags:        []string{},
			expectError: true,
		},
		{
			name:        "saltbox tags",
			tags:        []string{"plex", "sonarr"},
			expectError: false,
		},
		{
			name:        "sandbox tags",
			tags:        []string{"sandbox-overseerr", "sandbox-tautulli"},
			expectError: false,
		},
		{
			name:        "saltbox-mod tags",
			tags:        []string{"mod-jellyfin", "mod-emby"},
			expectError: false,
		},
		{
			name:        "mixed tags",
			tags:        []string{"plex", "sandbox-overseerr", "mod-jellyfin"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock git executor
			mockGit := &MockGitExecutor{
				ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					return []byte("abc123def456\n"), nil
				},
			}
			git.SetExecutor(mockGit)

			// Setup mock ansible executor
			mockAnsible := &MockAnsibleExecutor{
				ExecuteContextFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					return []byte("TASK TAGS: [plex, sonarr, radarr, overseerr, tautulli, jellyfin, emby]"), nil
				},
				ExecuteWithIOFunc: func(ctx context.Context, dir string, name string, args []string, stdout, stderr, stdin any) error {
					if stdout != nil {
						if w, ok := stdout.(interface{ Write([]byte) (int, error) }); ok {
							w.Write([]byte("PLAY RECAP\n"))
						}
					}
					return nil
				},
			}
			ansible.SetExecutor(mockAnsible)

			// Create a mock cobra command
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.SetContext(context.Background())

			// Create cache
			c, err := cache.NewCache()
			if err != nil {
				t.Logf("Cache load failed (expected): %v", err)
			}

			// Pre-populate cache to speed up tests
			if c != nil {
				c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
					"commit": "abc123def456",
					"tags":   []any{"plex", "sonarr", "radarr"},
				})
				c.SetRepoCache(constants.SandboxRepoPath, map[string]any{
					"commit": "abc123def456",
					"tags":   []any{"overseerr", "tautulli"},
				})
			}

			// Call handleInstall - but note this may fail if it tries to actually run ansible
			// We're mainly testing the parsing logic here
			err = handleInstall(cmd, tt.tags, []string{}, []string{}, []string{}, 0, true) // Use noCache=true

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Helper function to convert string slice to []any slice
func convertToAnySlice(strings []string) []any {
	result := make([]any, len(strings))
	for i, s := range strings {
		result[i] = s
	}
	return result
}
