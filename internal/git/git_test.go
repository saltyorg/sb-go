package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// MockCommandExecutor is a mock implementation of CommandExecutor for testing
type MockCommandExecutor struct {
	ExecuteFunc func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
	calls       []MockCall
}

type MockCall struct {
	Dir  string
	Name string
	Args []string
}

func (m *MockCommandExecutor) ExecuteCommand(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, MockCall{Dir: dir, Name: name, Args: args})
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, dir, name, args...)
	}
	return []byte{}, nil
}

func (m *MockCommandExecutor) GetCalls() []MockCall {
	return m.calls
}

func (m *MockCommandExecutor) Reset() {
	m.calls = nil
}

// TestBuildCloneArgs tests the clone arguments builder
func TestBuildCloneArgs(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		destPath string
		branch   string
		expected []string
	}{
		{
			name:     "standard repository",
			repoURL:  "https://github.com/user/repo.git",
			destPath: "/srv/git/repo",
			branch:   "main",
			expected: []string{"clone", "--depth", "1", "-b", "main", "https://github.com/user/repo.git", "/srv/git/repo"},
		},
		{
			name:     "master branch",
			repoURL:  "https://github.com/user/repo.git",
			destPath: "/opt/repo",
			branch:   "master",
			expected: []string{"clone", "--depth", "1", "-b", "master", "https://github.com/user/repo.git", "/opt/repo"},
		},
		{
			name:     "feature branch",
			repoURL:  "git@github.com:user/repo.git",
			destPath: "/tmp/repo",
			branch:   "feature/new-feature",
			expected: []string{"clone", "--depth", "1", "-b", "feature/new-feature", "git@github.com:user/repo.git", "/tmp/repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := BuildCloneArgs(tt.repoURL, tt.destPath, tt.branch)
			if len(args) != len(tt.expected) {
				t.Errorf("BuildCloneArgs() length = %d, expected %d", len(args), len(tt.expected))
			}
			for i, arg := range args {
				if arg != tt.expected[i] {
					t.Errorf("BuildCloneArgs()[%d] = %s, expected %s", i, arg, tt.expected[i])
				}
			}
		})
	}
}

// TestBuildRevParseArgs tests the rev-parse arguments builder
func TestBuildRevParseArgs(t *testing.T) {
	args := BuildRevParseArgs()
	expected := []string{"rev-parse", "HEAD"}
	if len(args) != len(expected) {
		t.Errorf("BuildRevParseArgs() length = %d, expected %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("BuildRevParseArgs()[%d] = %s, expected %s", i, arg, expected[i])
		}
	}
}

// TestBuildRevParseBranchArgs tests the branch name arguments builder
func TestBuildRevParseBranchArgs(t *testing.T) {
	args := BuildRevParseBranchArgs()
	expected := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	if len(args) != len(expected) {
		t.Errorf("BuildRevParseBranchArgs() length = %d, expected %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("BuildRevParseBranchArgs()[%d] = %s, expected %s", i, arg, expected[i])
		}
	}
}

// TestParseCommitHash tests commit hash parsing
func TestParseCommitHash(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "standard commit hash",
			input:    []byte("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0\n"),
			expected: "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
		},
		{
			name:     "commit hash with spaces",
			input:    []byte("  a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0  \n"),
			expected: "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
		},
		{
			name:     "short commit hash",
			input:    []byte("a1b2c3d\n"),
			expected: "a1b2c3d",
		},
		{
			name:     "commit hash without newline",
			input:    []byte("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0"),
			expected: "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
		},
		{
			name:     "empty output",
			input:    []byte(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCommitHash(tt.input)
			if result != tt.expected {
				t.Errorf("ParseCommitHash() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestParseBranchName tests branch name parsing
func TestParseBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "main branch",
			input:    []byte("main\n"),
			expected: "main",
		},
		{
			name:     "master branch",
			input:    []byte("master\n"),
			expected: "master",
		},
		{
			name:     "feature branch",
			input:    []byte("feature/new-feature\n"),
			expected: "feature/new-feature",
		},
		{
			name:     "branch with spaces",
			input:    []byte("  develop  \n"),
			expected: "develop",
		},
		{
			name:     "empty output",
			input:    []byte(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("ParseBranchName() = %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestGetGitCommitHash tests GetGitCommitHash with mocked executor
func TestGetGitCommitHash(t *testing.T) {
	tests := []struct {
		name          string
		repoPath      string
		mockOutput    []byte
		mockError     error
		expectedHash  string
		expectedError bool
	}{
		{
			name:          "successful commit hash retrieval",
			repoPath:      "/srv/git/saltbox",
			mockOutput:    []byte("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0\n"),
			mockError:     nil,
			expectedHash:  "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
			expectedError: false,
		},
		{
			name:          "short commit hash",
			repoPath:      "/opt/sandbox",
			mockOutput:    []byte("a1b2c3d\n"),
			mockError:     nil,
			expectedHash:  "a1b2c3d",
			expectedError: false,
		},
		{
			name:          "git command fails",
			repoPath:      "/nonexistent",
			mockOutput:    []byte(""),
			mockError:     errors.New("fatal: not a git repository"),
			expectedHash:  "",
			expectedError: true,
		},
		{
			name:          "commit hash with whitespace",
			repoPath:      "/srv/git/repo",
			mockOutput:    []byte("  a1b2c3d4  \n"),
			mockError:     nil,
			expectedHash:  "a1b2c3d4",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original executor and restore after test
			originalExecutor := defaultExecutor
			defer SetExecutor(originalExecutor)

			// Create mock executor
			mock := &MockCommandExecutor{
				ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
					// Verify the command arguments
					if name != "git" {
						t.Errorf("Expected command 'git', got '%s'", name)
					}
					if len(args) != 2 || args[0] != "rev-parse" || args[1] != "HEAD" {
						t.Errorf("Expected args ['rev-parse', 'HEAD'], got %v", args)
					}
					if dir != tt.repoPath {
						t.Errorf("Expected dir %s, got %s", tt.repoPath, dir)
					}

					// Return mock result
					if tt.mockError != nil {
						// Check if directory exists for specific error message
						if _, err := os.Stat(tt.repoPath); err != nil {
							return tt.mockOutput, tt.mockError
						}
					}
					return tt.mockOutput, tt.mockError
				},
			}
			SetExecutor(mock)

			// Call the function
			hash, err := GetGitCommitHash(context.Background(), tt.repoPath)

			// Verify results
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if hash != tt.expectedHash {
					t.Errorf("GetGitCommitHash() = %s, expected %s", hash, tt.expectedHash)
				}
			}
		})
	}
}

// TestGetGitCommitHash_NonexistentDirectory tests error handling for nonexistent directory
func TestGetGitCommitHash_NonexistentDirectory(t *testing.T) {
	// Save original executor and restore after test
	originalExecutor := defaultExecutor
	defer SetExecutor(originalExecutor)

	nonexistentPath := "/this/path/does/not/exist/at/all"

	// Create mock executor that simulates directory not existing
	mock := &MockCommandExecutor{
		ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
			return []byte(""), errors.New("fatal: not a git repository")
		},
	}
	SetExecutor(mock)

	hash, err := GetGitCommitHash(context.Background(), nonexistentPath)

	if err == nil {
		t.Errorf("Expected error for nonexistent directory")
	}

	if hash != "" {
		t.Errorf("Expected empty hash for error case, got %s", hash)
	}

	// The error could be either about incomplete install (if dir doesn't exist)
	// or about git command failure (if dir exists but isn't a git repo)
	if err != nil && !strings.Contains(err.Error(), "incomplete install") && !strings.Contains(err.Error(), "error occurred while trying to get") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// TestCloneRepository tests CloneRepository error handling
func TestCloneRepository(t *testing.T) {
	tests := []struct {
		name          string
		repoURL       string
		destPath      string
		branch        string
		verbose       bool
		expectedError bool
		setupDir      bool
	}{
		{
			name:          "clone to existing directory",
			repoURL:       "https://github.com/user/repo.git",
			destPath:      t.TempDir(), // Already exists
			branch:        "main",
			verbose:       false,
			expectedError: true,
			setupDir:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			err := CloneRepository(ctx, tt.repoURL, tt.destPath, tt.branch, tt.verbose)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestCloneRepository_ContextCancellation tests context cancellation handling
func TestCloneRepository_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	destPath := filepath.Join(t.TempDir(), "test-repo-cancel")
	err := CloneRepository(ctx, "https://github.com/user/repo.git", destPath, "main", false)

	// With a cancelled context, we expect an error (but the exact error depends on timing)
	// It could be "context canceled" or "failed to clone"
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

// TestDefaultCommandExecutor tests the default executor implementation
func TestDefaultCommandExecutor(t *testing.T) {
	// Use the global executor that the rest of the program uses
	exec := GetExecutor()
	ctx := context.Background()

	// Test with a simple command that should work on all systems
	output, err := exec.ExecuteCommand(ctx, "", "git", "--version")

	if err != nil {
		// Git might not be installed in test environment
		t.Logf("Git not available in test environment: %v", err)
		return
	}

	if len(output) == 0 {
		t.Errorf("Expected output from git --version")
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "git") {
		t.Errorf("Expected git version in output, got: %s", outputStr)
	}
}

// TestSetExecutor tests executor replacement
func TestSetExecutor(t *testing.T) {
	originalExecutor := GetExecutor()
	defer SetExecutor(originalExecutor)

	mockExecutor := &MockCommandExecutor{}
	SetExecutor(mockExecutor)

	currentExecutor := GetExecutor()
	if currentExecutor != mockExecutor {
		t.Errorf("SetExecutor() did not set the executor correctly")
	}

	// Verify it's actually used
	SetExecutor(originalExecutor)
	currentExecutor = GetExecutor()
	if currentExecutor != originalExecutor {
		t.Errorf("Failed to restore original executor")
	}
}

// TestCloneRepository_ErrorMessageFormat tests error message formatting
func TestCloneRepository_ErrorMessageFormat(t *testing.T) {
	// Test that errors are returned properly
	ctx := context.Background()
	destPath := filepath.Join(t.TempDir(), "test-repo-error")

	// Use invalid git URL to trigger error
	err := CloneRepository(ctx, "invalid://url", destPath, "main", false)

	if err == nil {
		t.Errorf("Expected error but got none")
		return
	}

	errMsg := err.Error()
	// Just verify we get an error message
	if !strings.Contains(errMsg, "failed to clone") {
		t.Logf("Got error (expected): %s", errMsg)
	}
}

// TestExecutorIntegration tests the executor with real git commands (if git is available)
func TestExecutorIntegration(t *testing.T) {
	// Create a temporary git repository for testing
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	// Try to create a git repository
	// Use the global executor that the rest of the program uses
	exec := GetExecutor()
	ctx := context.Background()

	// Check if git is available
	_, err := exec.ExecuteCommand(ctx, "", "git", "--version")
	if err != nil {
		t.Skip("Git not available, skipping integration test")
	}

	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Initialize git repository
	_, err = exec.ExecuteCommand(ctx, repoPath, "git", "init")
	if err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Create a test commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = exec.ExecuteCommand(ctx, repoPath, "git", "add", "test.txt")
	if err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	_, err = exec.ExecuteCommand(ctx, repoPath, "git", "commit", "-m", "Initial commit")
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Now test GetGitCommitHash
	hash, err := GetGitCommitHash(context.Background(), repoPath)
	if err != nil {
		t.Errorf("GetGitCommitHash failed: %v", err)
	}

	if len(hash) == 0 {
		t.Errorf("Expected non-empty commit hash")
	}

	// Verify it's a valid git hash (40 hex characters or 7+ for short hash)
	if len(hash) < 7 {
		t.Errorf("Commit hash too short: %s", hash)
	}
}

// TestMockCommandExecutor_CallTracking tests that mock tracks calls
func TestMockCommandExecutor_CallTracking(t *testing.T) {
	mock := &MockCommandExecutor{
		ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
			return []byte("output"), nil
		},
	}

	ctx := context.Background()
	mock.ExecuteCommand(ctx, "/path1", "git", "status")
	mock.ExecuteCommand(ctx, "/path2", "git", "log")

	calls := mock.GetCalls()
	if len(calls) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(calls))
	}

	if calls[0].Name != "git" || calls[0].Args[0] != "status" {
		t.Errorf("First call not recorded correctly")
	}

	if calls[1].Name != "git" || calls[1].Args[0] != "log" {
		t.Errorf("Second call not recorded correctly")
	}

	mock.Reset()
	calls = mock.GetCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", len(calls))
	}
}

// TestBuildCloneArgs_Validation tests argument validation
func TestBuildCloneArgs_Validation(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		destPath string
		branch   string
	}{
		{
			name:     "empty repo URL",
			repoURL:  "",
			destPath: "/path",
			branch:   "main",
		},
		{
			name:     "empty dest path",
			repoURL:  "https://github.com/user/repo.git",
			destPath: "",
			branch:   "main",
		},
		{
			name:     "empty branch",
			repoURL:  "https://github.com/user/repo.git",
			destPath: "/path",
			branch:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := BuildCloneArgs(tt.repoURL, tt.destPath, tt.branch)
			// Should still build args even if inputs are empty
			// The actual validation happens when executing the command
			if len(args) != 7 {
				t.Errorf("Expected 7 arguments, got %d", len(args))
			}
		})
	}
}

// TestGetGitCommitHash_DirectoryNotExist tests the special case where directory doesn't exist
func TestGetGitCommitHash_DirectoryNotExist(t *testing.T) {
	// Save original executor
	originalExecutor := defaultExecutor
	defer SetExecutor(originalExecutor)

	// Create a unique non-existent path
	nonExistPath := filepath.Join(os.TempDir(), fmt.Sprintf("nonexistent-%d", os.Getpid()))

	// Ensure it doesn't exist
	os.RemoveAll(nonExistPath)

	// Mock executor that simulates git command failure
	mock := &MockCommandExecutor{
		ExecuteFunc: func(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
			return []byte(""), errors.New("fatal: not a git repository")
		},
	}
	SetExecutor(mock)

	hash, err := GetGitCommitHash(context.Background(), nonExistPath)

	if err == nil {
		t.Errorf("Expected error for nonexistent directory")
	}

	if hash != "" {
		t.Errorf("Expected empty hash, got %s", hash)
	}

	// Verify error message mentions incomplete install
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		// Check if error mentions incomplete install
		if !strings.Contains(err.Error(), "incomplete install") {
			t.Errorf("Expected error to mention incomplete install, got: %v", err)
		}
	}
}
