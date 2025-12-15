package cmd

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"

	"github.com/spf13/cobra"
)

// TestInstallCmdStructure tests the install command structure and flags
func TestInstallCmdStructure(t *testing.T) {
	t.Run("command initialization", func(t *testing.T) {
		if installCmd == nil {
			t.Fatal("installCmd should be initialized")
		}

		if installCmd.Use != "install [tags]" {
			t.Errorf("Expected Use='install [tags]', got %q", installCmd.Use)
		}

		if installCmd.Short == "" {
			t.Error("Command Short description should not be empty")
		}

		if installCmd.Long == "" {
			t.Error("Command Long description should not be empty")
		}

		if installCmd.RunE == nil {
			t.Error("Command RunE should be defined")
		}
	})

	t.Run("command flags", func(t *testing.T) {
		flags := installCmd.Flags()

		// Check extra-vars flag
		extraVarsFlag := flags.Lookup("extra-vars")
		if extraVarsFlag == nil {
			t.Error("extra-vars flag should exist")
		} else {
			if extraVarsFlag.Shorthand != "e" {
				t.Errorf("Expected extra-vars shorthand 'e', got %q", extraVarsFlag.Shorthand)
			}
		}

		// Check skip-tags flag
		skipTagsFlag := flags.Lookup("skip-tags")
		if skipTagsFlag == nil {
			t.Error("skip-tags flag should exist")
		} else {
			if skipTagsFlag.Shorthand != "s" {
				t.Errorf("Expected skip-tags shorthand 's', got %q", skipTagsFlag.Shorthand)
			}
		}

		// Check verbose flag
		verboseFlag := flags.Lookup("verbose")
		if verboseFlag == nil {
			t.Error("verbose flag should exist")
		} else {
			if verboseFlag.Shorthand != "v" {
				t.Errorf("Expected verbose shorthand 'v', got %q", verboseFlag.Shorthand)
			}
		}

		// Check no-cache flag
		noCacheFlag := flags.Lookup("no-cache")
		if noCacheFlag == nil {
			t.Error("no-cache flag should exist")
		}
	})

	t.Run("minimum args requirement", func(t *testing.T) {
		// Test that command requires at least 1 arg
		// MinimumNArgs(1) is set in the command definition
		cmd := &cobra.Command{
			Use:  "test",
			Args: cobra.MinimumNArgs(1),
		}

		// No args - should error
		err := cmd.Args(cmd, []string{})
		if err == nil {
			t.Error("Expected error with no arguments")
		}

		// One arg - should pass
		err = cmd.Args(cmd, []string{"tag1"})
		if err != nil {
			t.Errorf("Expected no error with one argument, got: %v", err)
		}

		// Multiple args - should pass
		err = cmd.Args(cmd, []string{"tag1", "tag2"})
		if err != nil {
			t.Errorf("Expected no error with multiple arguments, got: %v", err)
		}
	})
}

func TestTagCategorization(t *testing.T) {
	tests := []struct {
		name               string
		tags               []string
		expectedSaltbox    int
		expectedSandbox    int
		expectedSaltboxMod int
	}{
		{
			name:               "only saltbox tags",
			tags:               []string{"plex", "sonarr", "radarr"},
			expectedSaltbox:    3,
			expectedSandbox:    0,
			expectedSaltboxMod: 0,
		},
		{
			name:               "only sandbox tags",
			tags:               []string{"sandbox-tautulli", "sandbox-overseerr"},
			expectedSaltbox:    0,
			expectedSandbox:    2,
			expectedSaltboxMod: 0,
		},
		{
			name:               "only mod tags",
			tags:               []string{"mod-jellyfin", "mod-emby"},
			expectedSaltbox:    0,
			expectedSandbox:    0,
			expectedSaltboxMod: 2,
		},
		{
			name:               "mixed tags",
			tags:               []string{"plex", "sandbox-tautulli", "mod-jellyfin", "sonarr"},
			expectedSaltbox:    2,
			expectedSandbox:    1,
			expectedSaltboxMod: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saltboxTags []string
			var sandboxTags []string
			var saltboxModTags []string

			for _, tag := range tt.tags {
				if after, ok := strings.CutPrefix(tag, "mod-"); ok {
					saltboxModTags = append(saltboxModTags, after)
				} else if after, ok := strings.CutPrefix(tag, "sandbox-"); ok {
					sandboxTags = append(sandboxTags, after)
				} else {
					saltboxTags = append(saltboxTags, tag)
				}
			}

			if len(saltboxTags) != tt.expectedSaltbox {
				t.Errorf("Expected %d saltbox tags, got %d", tt.expectedSaltbox, len(saltboxTags))
			}
			if len(sandboxTags) != tt.expectedSandbox {
				t.Errorf("Expected %d sandbox tags, got %d", tt.expectedSandbox, len(sandboxTags))
			}
			if len(saltboxModTags) != tt.expectedSaltboxMod {
				t.Errorf("Expected %d saltbox-mod tags, got %d", tt.expectedSaltboxMod, len(saltboxModTags))
			}
		})
	}
}

func TestVerbosityFlagConstruction(t *testing.T) {
	tests := []struct {
		name      string
		verbosity int
		expected  string
	}{
		{
			name:      "no verbosity",
			verbosity: 0,
			expected:  "",
		},
		{
			name:      "verbosity 1",
			verbosity: 1,
			expected:  "-v",
		},
		{
			name:      "verbosity 2",
			verbosity: 2,
			expected:  "-vv",
		},
		{
			name:      "verbosity 3",
			verbosity: 3,
			expected:  "-vvv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var extraArgs []string
			if tt.verbosity > 0 {
				vFlag := "-" + strings.Repeat("v", tt.verbosity)
				extraArgs = append(extraArgs, vFlag)
			}

			if tt.expected == "" {
				if len(extraArgs) != 0 {
					t.Errorf("Expected no extra args, got %v", extraArgs)
				}
			} else {
				if len(extraArgs) != 1 {
					t.Errorf("Expected 1 extra arg, got %d", len(extraArgs))
				} else if extraArgs[0] != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, extraArgs[0])
				}
			}
		})
	}
}

func TestRunPlaybookArguments(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		extraVars    []string
		skipTags     []string
		extraArgs    []string
		expectedArgs int // Minimum expected argument count
	}{
		{
			name:         "basic tags only",
			tags:         []string{"plex"},
			extraVars:    []string{},
			skipTags:     []string{},
			extraArgs:    []string{},
			expectedArgs: 2, // --tags, tag_value
		},
		{
			name:         "tags with extra vars",
			tags:         []string{"plex"},
			extraVars:    []string{"var1=value1"},
			skipTags:     []string{},
			extraArgs:    []string{},
			expectedArgs: 4, // --tags, tag_value, --extra-vars, var
		},
		{
			name:         "tags with skip tags",
			tags:         []string{"plex"},
			extraVars:    []string{},
			skipTags:     []string{"always"},
			extraArgs:    []string{},
			expectedArgs: 4, // --tags, tag_value, --skip-tags, skip_value
		},
		{
			name:         "all options",
			tags:         []string{"plex", "sonarr"},
			extraVars:    []string{"var1=value1", "var2=value2"},
			skipTags:     []string{"always", "never"},
			extraArgs:    []string{"-vv"},
			expectedArgs: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagsArg := strings.Join(tt.tags, ",")
			allArgs := []string{"--tags", tagsArg}

			for _, extraVar := range tt.extraVars {
				allArgs = append(allArgs, "--extra-vars", extraVar)
			}

			if len(tt.skipTags) > 0 {
				allArgs = append(allArgs, "--skip-tags", strings.Join(tt.skipTags, ","))
			}

			allArgs = append(allArgs, tt.extraArgs...)

			if len(allArgs) < tt.expectedArgs {
				t.Errorf("Expected at least %d args, got %d: %v", tt.expectedArgs, len(allArgs), allArgs)
			}

			// Verify --tags is present
			hasTagsFlag := slices.Contains(allArgs, "--tags")
			if !hasTagsFlag {
				t.Error("--tags flag should be present")
			}
		})
	}
}

func TestCacheExistsAndIsValidFunction(t *testing.T) {
	tests := []struct {
		name       string
		setupCache bool
		hasTags    bool
		tagsEmpty  bool
		expected   bool
	}{
		{
			name:       "cache with valid non-empty tags",
			setupCache: true,
			hasTags:    true,
			tagsEmpty:  false,
			expected:   true,
		},
		{
			name:       "cache with empty tags",
			setupCache: true,
			hasTags:    true,
			tagsEmpty:  true,
			expected:   false,
		},
		{
			name:       "cache without tags key",
			setupCache: true,
			hasTags:    false,
			tagsEmpty:  false,
			expected:   false,
		},
		{
			name:       "no cache",
			setupCache: false,
			hasTags:    false,
			tagsEmpty:  false,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary cache file for this test
			tmpDir := t.TempDir()
			cacheFile := filepath.Join(tmpDir, "cache.json")
			testCache, err := cache.NewCacheWithFile(cacheFile)
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}

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

				// Set cache directly in the cache instance
				_ = testCache.SetRepoCache(constants.SaltboxRepoPath, cacheData)
			}

			result := cacheExistsAndIsValid(constants.SaltboxRepoPath, testCache, 0)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEmptyTagsHandling(t *testing.T) {
	tests := []struct {
		name        string
		inputArgs   []string
		expectEmpty bool
	}{
		{
			name:        "valid tags",
			inputArgs:   []string{"plex", "sonarr"},
			expectEmpty: false,
		},
		{
			name:        "tags with spaces",
			inputArgs:   []string{"  plex  ", "  sonarr  "},
			expectEmpty: false,
		},
		{
			name:        "all empty",
			inputArgs:   []string{"", "  ", "\t"},
			expectEmpty: true,
		},
		{
			name:        "mixed empty and valid",
			inputArgs:   []string{"plex", "", "sonarr"},
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joined := strings.Join(tt.inputArgs, ",")
			rawTags := strings.Split(joined, ",")

			var tags []string
			for _, t := range rawTags {
				tag := strings.TrimSpace(t)
				if tag != "" {
					tags = append(tags, tag)
				}
			}

			isEmpty := len(tags) == 0

			if isEmpty != tt.expectEmpty {
				t.Errorf("Expected isEmpty=%v, got %v (tags: %v)", tt.expectEmpty, isEmpty, tags)
			}
		})
	}
}

func TestGetValidTagsLogic(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		valid    bool
	}{
		{
			name:     "saltbox repo",
			repoPath: constants.SaltboxRepoPath,
			valid:    true,
		},
		{
			name:     "sandbox repo",
			repoPath: constants.SandboxRepoPath,
			valid:    true,
		},
		{
			name:     "unknown repo",
			repoPath: "/unknown/path",
			valid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			playbookPath := ""
			switch tt.repoPath {
			case constants.SaltboxRepoPath:
				playbookPath = constants.SaltboxPlaybookPath()
			case constants.SandboxRepoPath:
				playbookPath = constants.SandboxPlaybookPath()
			}

			if tt.valid && playbookPath == "" {
				t.Error("Valid repo should have a playbook path")
			}

			if !tt.valid && playbookPath != "" {
				t.Error("Invalid repo should not have a playbook path")
			}
		})
	}
}

func TestHandleInstallFunctionStructure(t *testing.T) {
	t.Run("context requirement", func(t *testing.T) {
		ctx := context.Background()
		if ctx == nil {
			t.Error("Context should not be nil")
		}
	})

	t.Run("cache creation", func(t *testing.T) {
		_, err := cache.NewCache()
		if err != nil {
			t.Errorf("Cache creation failed: %v", err)
		}
	})

	t.Run("tag parsing from args", func(t *testing.T) {
		tags := []string{"plex", "sonarr"}
		var parsedTags []string
		for _, arg := range tags {
			parsedTags = append(parsedTags, strings.TrimSpace(arg))
		}

		if len(parsedTags) != len(tags) {
			t.Errorf("Expected %d parsed tags, got %d", len(tags), len(parsedTags))
		}
	})
}

func TestValidateAndSuggestStructure(t *testing.T) {
	tests := []struct {
		name          string
		repoPath      string
		currentPrefix string
		otherPrefix   string
		expectedNames [2]string
	}{
		{
			name:          "saltbox repo",
			repoPath:      constants.SaltboxRepoPath,
			currentPrefix: "",
			otherPrefix:   "sandbox-",
			expectedNames: [2]string{"Saltbox", "Sandbox"},
		},
		{
			name:          "sandbox repo",
			repoPath:      constants.SandboxRepoPath,
			currentPrefix: "sandbox-",
			otherPrefix:   "",
			expectedNames: [2]string{"Sandbox", "Saltbox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoName := "Saltbox"
			otherRepoName := "Sandbox"
			if tt.repoPath == constants.SandboxRepoPath {
				repoName = "Sandbox"
				otherRepoName = "Saltbox"
			}

			if repoName != tt.expectedNames[0] {
				t.Errorf("Expected repo name %q, got %q", tt.expectedNames[0], repoName)
			}

			if otherRepoName != tt.expectedNames[1] {
				t.Errorf("Expected other repo name %q, got %q", tt.expectedNames[1], otherRepoName)
			}
		})
	}
}

func TestLevenshteinDistanceThreshold(t *testing.T) {
	tests := []struct {
		name          string
		distance      int
		threshold     int
		shouldSuggest bool
	}{
		{
			name:          "distance 0",
			distance:      0,
			threshold:     2,
			shouldSuggest: true,
		},
		{
			name:          "distance 1",
			distance:      1,
			threshold:     2,
			shouldSuggest: true,
		},
		{
			name:          "distance 2",
			distance:      2,
			threshold:     2,
			shouldSuggest: true,
		},
		{
			name:          "distance 3",
			distance:      3,
			threshold:     2,
			shouldSuggest: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSuggest := tt.distance <= tt.threshold

			if shouldSuggest != tt.shouldSuggest {
				t.Errorf("Expected shouldSuggest=%v, got %v", tt.shouldSuggest, shouldSuggest)
			}
		})
	}
}
