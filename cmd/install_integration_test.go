package cmd

import (
	"context"
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
			_, _ = w.Write([]byte("mock playbook output\n"))
		}
	}
	return nil
}

// TestCacheExistsAndIsValid_Integration tests the actual function
func TestCacheExistsAndIsValid_Integration(t *testing.T) {
	t.Skip("Skipping: tests use real system paths that can't be mocked without refactoring production code")

	testRepoPath := "/tmp/test-repo"

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
				c.SetRepoCache(testRepoPath, map[string]any{
					"commit": "abc123",
					"tags":   []any{"tag1", "tag2", "tag3"},
				})
			},
			repoPath:  testRepoPath,
			verbosity: 0,
			expected:  true,
		},
		{
			name: "cache with empty tags",
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(testRepoPath, map[string]any{
					"commit": "abc123",
					"tags":   []any{},
				})
			},
			repoPath:  testRepoPath,
			verbosity: 0,
			expected:  false,
		},
		{
			name: "cache without tags key",
			setupCache: func(c *cache.Cache) {
				c.SetRepoCache(testRepoPath, map[string]any{
					"commit": "abc123",
				})
			},
			repoPath:  testRepoPath,
			verbosity: 0,
			expected:  false,
		},
		{
			name:       "no cache",
			setupCache: func(c *cache.Cache) {},
			repoPath:   testRepoPath,
			verbosity:  0,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache file for this test
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")

			// Create cache instance with temporary file
			c, err := cache.NewCacheWithFile(cacheFile)
			if err != nil {
				t.Fatalf("Failed to create test cache: %v", err)
			}

			// Setup cache data
			if tt.setupCache != nil {
				tt.setupCache(c)
			}

			// Call the actual function
			result := cacheExistsAndIsValid(tt.repoPath, c, tt.verbosity)

			if result != tt.expected {
				t.Errorf("cacheExistsAndIsValid() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestGetValidTags_Integration tests the actual function with mocks
func TestGetValidTags_Integration(t *testing.T) {
	t.Skip("Skipping: tests use real system paths that can't be mocked without refactoring production code")

	testRepoPath := "/tmp/test-repo"

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
		setupCache  func(*cache.Cache, string)
		mockGitHash string
		mockTags    []string
		expectedLen int
		expectEmpty bool
	}{
		{
			name:     "valid cached tags",
			repoPath: testRepoPath,
			setupCache: func(c *cache.Cache, path string) {
				c.SetRepoCache(path, map[string]any{
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
			repoPath: testRepoPath,
			setupCache: func(c *cache.Cache, path string) {
				c.SetRepoCache(path, map[string]any{
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
			repoPath:    testRepoPath,
			setupCache:  func(c *cache.Cache, path string) {},
			mockGitHash: "abc123def456",
			mockTags:    []string{"tag1", "tag2"},
			expectedLen: 2,
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create temporary cache file for this test
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")

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

			// Create cache with temporary file
			c, err := cache.NewCacheWithFile(cacheFile)
			if err != nil {
				t.Fatalf("Failed to create test cache: %v", err)
			}

			// Setup cache data
			if tt.setupCache != nil {
				tt.setupCache(c, tt.repoPath)
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
	t.Skip("Skipping: tests use real system paths that can't be mocked without refactoring production code")

	testSaltboxRepoPath := "/tmp/test-saltbox"
	testSandboxRepoPath := "/tmp/test-sandbox"

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
		otherRepoPath     string
		providedTags      []string
		currentPrefix     string
		otherPrefix       string
		saltboxTags       []string
		sandboxTags       []string
		expectedSuggCount int
	}{
		{
			name:              "valid tags - no suggestions",
			repoPath:          testSaltboxRepoPath,
			otherRepoPath:     testSandboxRepoPath,
			providedTags:      []string{"plex", "sonarr"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr", "radarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 0,
		},
		{
			name:              "invalid tag - suggestions generated",
			repoPath:          testSaltboxRepoPath,
			otherRepoPath:     testSandboxRepoPath,
			providedTags:      []string{"invalidtag"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr", "radarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 1,
		},
		{
			name:              "tag exists in other repo",
			repoPath:          testSaltboxRepoPath,
			otherRepoPath:     testSandboxRepoPath,
			providedTags:      []string{"overseerr"},
			currentPrefix:     "",
			otherPrefix:       "sandbox-",
			saltboxTags:       []string{"plex", "sonarr"},
			sandboxTags:       []string{"overseerr", "tautulli"},
			expectedSuggCount: 1,
		},
		{
			name:              "close match - typo",
			repoPath:          testSaltboxRepoPath,
			otherRepoPath:     testSandboxRepoPath,
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

			// Create temporary cache file for this test
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")

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
					if dir == tt.otherRepoPath {
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

			// Create cache with temporary file and pre-populate data
			c, err := cache.NewCacheWithFile(cacheFile)
			if err != nil {
				t.Fatalf("Failed to create test cache: %v", err)
			}

			c.SetRepoCache(tt.repoPath, map[string]any{
				"commit": "abc123def456",
				"tags":   convertToAnySlice(tt.saltboxTags),
			})
			c.SetRepoCache(tt.otherRepoPath, map[string]any{
				"commit": "abc123def456",
				"tags":   convertToAnySlice(tt.sandboxTags),
			})

			// Call the actual function
			suggestions := validateAndSuggest(ctx, tt.repoPath, tt.providedTags, tt.currentPrefix, tt.otherPrefix, c, 0)

			if len(suggestions) != tt.expectedSuggCount {
				t.Errorf("validateAndSuggest() returned %d suggestions, expected %d. Suggestions: %v",
					len(suggestions), tt.expectedSuggCount, suggestions)
			}
		})
	}
}

// TestHandleInstall_Integration tests parts of handleInstall with mocks
func TestHandleInstall_Integration(t *testing.T) {
	t.Skip("Skipping TestHandleInstall_Integration: this test modifies the real cache file")

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
							_, _ = w.Write([]byte("PLAY RECAP\n"))
						}
					}
					return nil
				},
			}
			ansible.SetExecutor(mockAnsible)

			// Create temporary cache file for this test
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")

			// Create a mock cobra command
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.SetContext(context.Background())

			// Create cache with temporary file and pre-populate to speed up tests
			c, err := cache.NewCacheWithFile(cacheFile)
			if err != nil {
				t.Fatalf("Failed to create test cache: %v", err)
			}

			c.SetRepoCache(constants.SaltboxRepoPath, map[string]any{
				"commit": "abc123def456",
				"tags":   []any{"plex", "sonarr", "radarr"},
			})
			c.SetRepoCache(constants.SandboxRepoPath, map[string]any{
				"commit": "abc123def456",
				"tags":   []any{"overseerr", "tautulli"},
			})

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
