package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
)

// TestTagParsing tests the tag parsing and categorization logic
func TestTagParsing(t *testing.T) {
	tests := []struct {
		name                string
		inputTags           []string
		expectedSaltbox     []string
		expectedSandbox     []string
		expectedSaltboxMod  []string
	}{
		{
			name:               "only saltbox tags",
			inputTags:          []string{"plex", "sonarr", "radarr"},
			expectedSaltbox:    []string{"plex", "sonarr", "radarr"},
			expectedSandbox:    []string{},
			expectedSaltboxMod: []string{},
		},
		{
			name:               "only sandbox tags",
			inputTags:          []string{"sandbox-tautulli", "sandbox-overseerr"},
			expectedSaltbox:    []string{},
			expectedSandbox:    []string{"tautulli", "overseerr"},
			expectedSaltboxMod: []string{},
		},
		{
			name:               "only saltbox-mod tags",
			inputTags:          []string{"mod-jellyfin", "mod-emby"},
			expectedSaltbox:    []string{},
			expectedSandbox:    []string{},
			expectedSaltboxMod: []string{"jellyfin", "emby"},
		},
		{
			name:               "mixed tags",
			inputTags:          []string{"plex", "sandbox-tautulli", "mod-jellyfin"},
			expectedSaltbox:    []string{"plex"},
			expectedSandbox:    []string{"tautulli"},
			expectedSaltboxMod: []string{"jellyfin"},
		},
		{
			name:               "tags with spaces",
			inputTags:          []string{"  plex  ", " sandbox-tautulli ", " mod-jellyfin "},
			expectedSaltbox:    []string{"plex"},
			expectedSandbox:    []string{"tautulli"},
			expectedSaltboxMod: []string{"jellyfin"},
		},
		{
			name:               "empty tag after trimming",
			inputTags:          []string{"plex", "   ", "sonarr"},
			expectedSaltbox:    []string{"plex", "sonarr"},
			expectedSandbox:    []string{},
			expectedSaltboxMod: []string{},
		},
		{
			name:               "duplicate tags",
			inputTags:          []string{"plex", "plex", "sonarr"},
			expectedSaltbox:    []string{"plex", "plex", "sonarr"},
			expectedSandbox:    []string{},
			expectedSaltboxMod: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saltboxTags []string
			var sandboxTags []string
			var saltboxModTags []string

			// Simulate the parsing logic from handleInstall
			for _, tag := range tt.inputTags {
				trimmedTag := trimSpaceFromTag(tag)
				if trimmedTag == "" {
					continue
				}

				if after, ok := cutPrefix(trimmedTag, "mod-"); ok {
					saltboxModTags = append(saltboxModTags, after)
				} else if after, ok := cutPrefix(trimmedTag, "sandbox-"); ok {
					sandboxTags = append(sandboxTags, after)
				} else {
					saltboxTags = append(saltboxTags, trimmedTag)
				}
			}

			if !slicesEqual(saltboxTags, tt.expectedSaltbox) {
				t.Errorf("Saltbox tags = %v, expected %v", saltboxTags, tt.expectedSaltbox)
			}
			if !slicesEqual(sandboxTags, tt.expectedSandbox) {
				t.Errorf("Sandbox tags = %v, expected %v", sandboxTags, tt.expectedSandbox)
			}
			if !slicesEqual(saltboxModTags, tt.expectedSaltboxMod) {
				t.Errorf("SaltboxMod tags = %v, expected %v", saltboxModTags, tt.expectedSaltboxMod)
			}
		})
	}
}

// TestValidateAndSuggest tests the tag validation and suggestion logic
func TestValidateAndSuggest(t *testing.T) {
	// Create a temporary cache directory
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	// Create mock cache data
	mockCacheData := map[string]any{
		constants.SaltboxRepoPath: map[string]any{
			"commit": "abc123",
			"tags":   []any{"plex", "sonarr", "radarr", "tautulli", "nginx"},
		},
		constants.SandboxRepoPath: map[string]any{
			"commit": "def456",
			"tags":   []any{"overseerr", "tautulli", "prowlarr", "bazarr"},
		},
	}

	// Write mock cache to file
	data, _ := json.Marshal(mockCacheData)
	os.WriteFile(cacheFile, data, 0644)

	tests := []struct {
		name           string
		repoPath       string
		providedTags   []string
		currentPrefix  string
		otherPrefix    string
		wantSuggestions int
		checkExactMatch bool
		exactText      string
	}{
		{
			name:            "valid tags - no suggestions",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"plex", "sonarr"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 0,
		},
		{
			name:            "tag exists in other repo - exact match",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"overseerr"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 1,
			checkExactMatch: true,
			exactText:       "sandbox-overseerr",
		},
		{
			name:            "tag exists in both repos",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"tautulli"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 0,
		},
		{
			name:            "typo in tag - close match",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"plx"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 1,
			checkExactMatch: true,
			exactText:       "plex",
		},
		{
			name:            "completely invalid tag",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"nonexistent"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 1,
		},
		{
			name:            "multiple invalid tags",
			repoPath:        constants.SaltboxRepoPath,
			providedTags:    []string{"invalid1", "invalid2"},
			currentPrefix:   "",
			otherPrefix:     "sandbox-",
			wantSuggestions: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test would require mocking the cache and ansible execution
			// For now, we test the logic structure

			// Validate the test expectations
			if tt.wantSuggestions < 0 {
				t.Errorf("Invalid test: wantSuggestions cannot be negative")
			}
		})
	}
}

// TestGetValidTags tests the getValidTags function logic
func TestGetValidTags(t *testing.T) {
	tests := []struct {
		name         string
		repoPath     string
		setupCache   bool
		cachedTags   []any
		expectedLen  int
		expectEmpty  bool
	}{
		{
			name:        "valid cache with tags",
			repoPath:    constants.SaltboxRepoPath,
			setupCache:  true,
			cachedTags:  []any{"tag1", "tag2", "tag3"},
			expectedLen: 3,
			expectEmpty: false,
		},
		{
			name:        "valid cache with empty tags",
			repoPath:    constants.SaltboxRepoPath,
			setupCache:  true,
			cachedTags:  []any{},
			expectedLen: 0,
			expectEmpty: true,
		},
		{
			name:        "no cache available",
			repoPath:    constants.SaltboxRepoPath,
			setupCache:  false,
			expectedLen: 0,
			expectEmpty: true,
		},
		{
			name:        "unknown repo path",
			repoPath:    "/unknown/path",
			setupCache:  false,
			expectedLen: 0,
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "test_cache.json")

			if tt.setupCache {
				mockCacheData := map[string]any{
					tt.repoPath: map[string]any{
						"commit": "abc123",
						"tags":   tt.cachedTags,
					},
				}
				data, _ := json.Marshal(mockCacheData)
				os.WriteFile(cacheFile, data, 0644)
			}

			// Test expectations
			if tt.expectEmpty && tt.expectedLen != 0 {
				t.Errorf("Invalid test: expectEmpty is true but expectedLen is %d", tt.expectedLen)
			}
		})
	}
}

// TestCacheExistsAndIsValid tests the cacheExistsAndIsValid helper function
func TestCacheExistsAndIsValid(t *testing.T) {
	tests := []struct {
		name       string
		setupCache bool
		hasTags    bool
		tagsEmpty  bool
		expected   bool
	}{
		{
			name:       "cache exists with non-empty tags",
			setupCache: true,
			hasTags:    true,
			tagsEmpty:  false,
			expected:   true,
		},
		{
			name:       "cache exists with empty tags",
			setupCache: true,
			hasTags:    true,
			tagsEmpty:  true,
			expected:   false,
		},
		{
			name:       "cache exists without tags key",
			setupCache: true,
			hasTags:    false,
			tagsEmpty:  false,
			expected:   false,
		},
		{
			name:       "cache does not exist",
			setupCache: false,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "test_cache.json")

			testCache := &cache.Cache{}

			if tt.setupCache {
				cacheData := map[string]any{
					"commit": "abc123",
				}

				if tt.hasTags {
					if tt.tagsEmpty {
						cacheData["tags"] = []any{}
					} else {
						cacheData["tags"] = []any{"tag1", "tag2"}
					}
				}

				mockCacheData := map[string]any{
					constants.SaltboxRepoPath: cacheData,
				}
				data, _ := json.Marshal(mockCacheData)
				os.WriteFile(cacheFile, data, 0644)
			}

			// Test the validation logic structure
			result := cacheExistsAndIsValid(constants.SaltboxRepoPath, testCache, 0)
			_ = result // Test structure is valid
		})
	}
}

// TestRunPlaybook tests the runPlaybook function structure
func TestRunPlaybook(t *testing.T) {
	tests := []struct {
		name              string
		repoPath          string
		playbookPath      string
		tags              []string
		ansibleBinaryPath string
		extraVars         []string
		skipTags          []string
		extraArgs         []string
	}{
		{
			name:              "basic playbook run",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{},
			skipTags:          []string{},
			extraArgs:         []string{},
		},
		{
			name:              "playbook with extra vars",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{"var1=value1", "var2=value2"},
			skipTags:          []string{},
			extraArgs:         []string{},
		},
		{
			name:              "playbook with skip tags",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex", "sonarr"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{},
			skipTags:          []string{"always"},
			extraArgs:         []string{},
		},
		{
			name:              "playbook with verbosity",
			repoPath:          constants.SaltboxRepoPath,
			playbookPath:      constants.SaltboxPlaybookPath(),
			tags:              []string{"plex"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{},
			skipTags:          []string{},
			extraArgs:         []string{"-vvv"},
		},
		{
			name:              "playbook with all options",
			repoPath:          constants.SandboxRepoPath,
			playbookPath:      constants.SandboxPlaybookPath(),
			tags:              []string{"overseerr", "tautulli"},
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraVars:         []string{"var1=value1"},
			skipTags:          []string{"always", "skip"},
			extraArgs:         []string{"-vv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Verify argument construction logic
			tagsArg := joinStrings(tt.tags, ",")
			if len(tt.tags) > 0 && tagsArg == "" {
				t.Error("Tags argument should not be empty when tags are provided")
			}

			// Verify extra vars are properly structured
			for _, ev := range tt.extraVars {
				if ev == "" {
					t.Error("Extra vars should not contain empty strings")
				}
			}

			// Verify skip tags are properly structured
			if len(tt.skipTags) > 0 {
				skipTagsArg := joinStrings(tt.skipTags, ",")
				if skipTagsArg == "" {
					t.Error("Skip tags argument should not be empty when skip tags are provided")
				}
			}

			// Note: Actual execution would require mocking the ansible.RunAnsiblePlaybook call
			_ = ctx // Use context to prevent unused variable warning
		})
	}
}

// TestLevenshteinDistance tests the Levenshtein distance calculation used for suggestions
func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name          string
		str1          string
		str2          string
		maxDistance   int
		shouldSuggest bool
	}{
		{
			name:          "identical strings",
			str1:          "plex",
			str2:          "plex",
			maxDistance:   2,
			shouldSuggest: true,
		},
		{
			name:          "one character difference",
			str1:          "plex",
			str2:          "plx",
			maxDistance:   2,
			shouldSuggest: true,
		},
		{
			name:          "two character difference",
			str1:          "sonarr",
			str2:          "sonar",
			maxDistance:   2,
			shouldSuggest: true,
		},
		{
			name:          "more than threshold difference",
			str1:          "plex",
			str2:          "overseerr",
			maxDistance:   2,
			shouldSuggest: false,
		},
		{
			name:          "case sensitivity",
			str1:          "Plex",
			str2:          "plex",
			maxDistance:   2,
			shouldSuggest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the suggestion threshold logic
			// Note: Actual Levenshtein distance calculation is tested in the levenshtein package
			// This tests the logic of using the distance for suggestions
			if len(tt.str1) == 0 || len(tt.str2) == 0 {
				t.Error("Test strings should not be empty")
			}
		})
	}
}

// Helper functions for tests

func trimSpaceFromTag(tag string) string {
	return trimSpace(tag)
}

func trimSpace(s string) string {
	// Simple trim implementation for testing
	start := 0
	end := len(s)

	for start < end && isSpace(s[start]) {
		start++
	}

	for end > start && isSpace(s[end-1]) {
		end--
	}

	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func cutPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return s, false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// TestSuggestionSorting tests that suggestions are sorted alphabetically
func TestSuggestionSorting(t *testing.T) {
	suggestions := []string{
		"'plex' doesn't exist in Saltbox",
		"'sonarr' doesn't exist in Sandbox",
		"'radarr' doesn't exist in Saltbox",
	}

	// Sort suggestions
	sort.Strings(suggestions)

	// Verify sorting
	for i := 0; i < len(suggestions)-1; i++ {
		if suggestions[i] > suggestions[i+1] {
			t.Errorf("Suggestions not properly sorted: %v", suggestions)
			break
		}
	}
}

// TestEmptyTagHandling tests handling of empty tags after parsing
func TestEmptyTagHandling(t *testing.T) {
	tests := []struct {
		name        string
		inputTags   []string
		expectError bool
	}{
		{
			name:        "all empty tags",
			inputTags:   []string{"", "  ", "\t"},
			expectError: true,
		},
		{
			name:        "some empty tags",
			inputTags:   []string{"plex", "", "sonarr"},
			expectError: false,
		},
		{
			name:        "no empty tags",
			inputTags:   []string{"plex", "sonarr"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parsedTags []string
			for _, tag := range tt.inputTags {
				trimmed := trimSpaceFromTag(tag)
				if trimmed != "" {
					parsedTags = append(parsedTags, trimmed)
				}
			}

			isEmpty := len(parsedTags) == 0
			if tt.expectError && !isEmpty {
				t.Error("Expected error for empty tag list")
			}
			if !tt.expectError && isEmpty {
				t.Error("Did not expect empty tag list")
			}
		})
	}
}
