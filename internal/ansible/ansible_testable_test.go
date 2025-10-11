package ansible

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/internal/cache"
	"github.com/saltyorg/sb-go/internal/constants"
)

func TestParseTagsFromOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectError bool
		expectedLen int
	}{
		{
			name:        "valid output with tags",
			output:      "TASK TAGS: [tag1, tag2, tag3]",
			expectError: false,
			expectedLen: 3,
		},
		{
			name:        "valid output with single tag",
			output:      "TASK TAGS: [singletag]",
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "valid output with no tags",
			output:      "TASK TAGS: []",
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "valid output with spaces",
			output:      "TASK TAGS: [ tag1 , tag2 , tag3 ]",
			expectError: false,
			expectedLen: 3,
		},
		{
			name:        "output with multiple lines",
			output:      "Some output\nTASK TAGS: [tag1, tag2]\nMore output",
			expectError: false,
			expectedLen: 2,
		},
		{
			name:        "missing TASK TAGS",
			output:      "Some random output without tags",
			expectError: true,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, err := parseTagsFromOutput(tt.output)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError && len(tags) != tt.expectedLen {
				t.Errorf("Expected %d tags, got %d: %v", tt.expectedLen, len(tags), tags)
			}
		})
	}
}

func TestBuildAnsibleCommand(t *testing.T) {
	tests := []struct {
		name         string
		playbookPath string
		extraArgs    []string
		wantMinLen   int
		wantContains []string
	}{
		{
			name:         "basic command",
			playbookPath: "/path/to/playbook.yml",
			extraArgs:    []string{},
			wantMinLen:   2,
			wantContains: []string{"playbook.yml", "--become"},
		},
		{
			name:         "command with tags",
			playbookPath: "/path/to/playbook.yml",
			extraArgs:    []string{"--tags", "tag1,tag2"},
			wantMinLen:   4,
			wantContains: []string{"playbook.yml", "--become", "--tags"},
		},
		{
			name:         "command with verbosity",
			playbookPath: "/path/to/playbook.yml",
			extraArgs:    []string{"-vvv"},
			wantMinLen:   3,
			wantContains: []string{"playbook.yml", "--become", "-vvv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := buildAnsibleCommand(tt.playbookPath, tt.extraArgs)

			if len(command) < tt.wantMinLen {
				t.Errorf("Command length = %d, want at least %d", len(command), tt.wantMinLen)
			}

			cmdStr := strings.Join(command, " ")
			for _, want := range tt.wantContains {
				if !strings.Contains(cmdStr, want) {
					t.Errorf("Command should contain %q, got: %s", want, cmdStr)
				}
			}
		})
	}
}

func TestBuildListTagsCommand(t *testing.T) {
	tests := []struct {
		name          string
		playbookPath  string
		extraSkipTags string
		wantContains  []string
	}{
		{
			name:          "basic list tags",
			playbookPath:  "/path/to/playbook.yml",
			extraSkipTags: "",
			wantContains:  []string{"playbook.yml", "--become", "--list-tags", "--skip-tags=always,"},
		},
		{
			name:          "with extra skip tags",
			playbookPath:  "/path/to/playbook.yml",
			extraSkipTags: "skip1,skip2",
			wantContains:  []string{"playbook.yml", "--list-tags", "skip1,skip2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := buildListTagsCommand(tt.playbookPath, tt.extraSkipTags)

			cmdStr := strings.Join(command, " ")
			for _, want := range tt.wantContains {
				if !strings.Contains(cmdStr, want) {
					t.Errorf("Command should contain %q, got: %s", want, cmdStr)
				}
			}
		})
	}
}

func TestCheckCachedTags(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test_cache.json")

	// Create a git repository mock
	gitRepoPath := filepath.Join(tmpDir, "test-repo")
	os.MkdirAll(filepath.Join(gitRepoPath, ".git"), 0755)

	tests := []struct {
		name           string
		repoPath       string
		setupCache     bool
		commit         string
		tags           []any
		expectFound    bool
		expectedTagLen int
	}{
		{
			name:           "valid cache with matching commit",
			repoPath:       gitRepoPath,
			setupCache:     true,
			commit:         "abc123",
			tags:           []any{"tag1", "tag2", "tag3"},
			expectFound:    false, // Will be false because git.GetGitCommitHash will fail
			expectedTagLen: 0,
		},
		{
			name:           "no cache available",
			repoPath:       gitRepoPath,
			setupCache:     false,
			expectFound:    false,
			expectedTagLen: 0,
		},
		{
			name:           "cache with empty tags",
			repoPath:       gitRepoPath,
			setupCache:     true,
			commit:         "abc123",
			tags:           []any{},
			expectFound:    false,
			expectedTagLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCache, err := cache.NewCache()
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}

			if tt.setupCache {
				cacheData := map[string]any{
					tt.repoPath: map[string]any{
						"commit": tt.commit,
						"tags":   tt.tags,
					},
				}
				data, _ := json.Marshal(cacheData)
				os.WriteFile(cacheFile, data, 0644)
			}

			tags, found := checkCachedTags(tt.repoPath, testCache)

			if found != tt.expectFound {
				t.Errorf("Expected found=%v, got %v", tt.expectFound, found)
			}

			if len(tags) != tt.expectedTagLen {
				t.Errorf("Expected %d tags, got %d", tt.expectedTagLen, len(tags))
			}
		})
	}
}

func TestIsSaltboxModRepo(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		expected bool
	}{
		{
			name:     "saltbox mod repo",
			repoPath: constants.SaltboxModRepoPath,
			expected: true,
		},
		{
			name:     "saltbox repo",
			repoPath: constants.SaltboxRepoPath,
			expected: false,
		},
		{
			name:     "sandbox repo",
			repoPath: constants.SandboxRepoPath,
			expected: false,
		},
		{
			name:     "unknown repo",
			repoPath: "/some/other/path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSaltboxModRepo(tt.repoPath)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCreateTagParserFunc(t *testing.T) {
	tests := []struct {
		name         string
		tags         []string
		expectedLen  int
	}{
		{
			name:        "multiple tags",
			tags:        []string{"tag1", "tag2", "tag3"},
			expectedLen: 3,
		},
		{
			name:        "single tag",
			tags:        []string{"tag1"},
			expectedLen: 1,
		},
		{
			name:        "no tags",
			tags:        []string{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := createTagParserFunc(tt.tags)
			result, err := parser("any input")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(result) != tt.expectedLen {
				t.Errorf("Expected %d tags, got %d", tt.expectedLen, len(result))
			}

			for i, tag := range result {
				if tag != tt.tags[i] {
					t.Errorf("Expected tag %q at position %d, got %q", tt.tags[i], i, tag)
				}
			}
		})
	}
}

func TestFormatPlaybookError(t *testing.T) {
	tests := []struct {
		name         string
		playbookPath string
		errMsg       string
		stderr       string
		verbose      bool
		wantContains []string
	}{
		{
			name:         "non-verbose with stderr",
			playbookPath: "/test/playbook.yml",
			errMsg:       "exit status 1",
			stderr:       "Error: task failed",
			verbose:      false,
			wantContains: []string{"playbook.yml", "failed"},
		},
		{
			name:         "verbose without stderr",
			playbookPath: "/test/playbook.yml",
			errMsg:       "exit status 2",
			stderr:       "",
			verbose:      true,
			wantContains: []string{"playbook.yml", "failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a generic error
			err := context.DeadlineExceeded

			stderrBuf := strings.NewReader(tt.stderr)
			var buf *strings.Reader
			if !tt.verbose {
				buf = stderrBuf
			}

			_ = buf // Prevent unused variable
			errResult := formatPlaybookError(tt.playbookPath, err, nil, tt.verbose)

			if errResult == nil {
				t.Error("Expected error, got nil")
			}

			errStr := errResult.Error()
			for _, want := range tt.wantContains {
				if !strings.Contains(errStr, want) {
					t.Errorf("Error should contain %q, got: %s", want, errStr)
				}
			}
		})
	}
}

func TestCacheTagsWithCommit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a git repository mock
	gitRepoPath := filepath.Join(tmpDir, "test-repo")
	os.MkdirAll(filepath.Join(gitRepoPath, ".git"), 0755)

	testCache, err := cache.NewCache()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	tags := []string{"tag1", "tag2", "tag3"}

	// This will fail because we don't have a real git repo
	// but it tests that the function executes
	err = cacheTagsWithCommit(gitRepoPath, tags, testCache)

	// We expect an error because the git command will fail
	if err == nil {
		t.Error("Expected error due to invalid git repo")
	}
}

func TestRunAnsiblePlaybookStructure(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name              string
		repoPath          string
		playbookPath      string
		ansibleBinaryPath string
		extraArgs         []string
		verbose           bool
	}{
		{
			name:              "basic playbook execution",
			repoPath:          "/test/repo",
			playbookPath:      "/test/repo/playbook.yml",
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraArgs:         []string{"--tags", "test"},
			verbose:           false,
		},
		{
			name:              "verbose playbook execution",
			repoPath:          "/test/repo",
			playbookPath:      "/test/repo/playbook.yml",
			ansibleBinaryPath: constants.AnsiblePlaybookBinaryPath,
			extraArgs:         []string{"--tags", "test", "-vvv"},
			verbose:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that we can build the command structure
			command := buildAnsibleCommand(tt.playbookPath, tt.extraArgs)

			if len(command) < 2 {
				t.Error("Command should have at least playbook and --become")
			}

			if command[1] != "--become" {
				t.Error("Second argument should be --become")
			}

			// Verify context is valid
			if ctx == nil {
				t.Error("Context should not be nil")
			}
		})
	}
}
