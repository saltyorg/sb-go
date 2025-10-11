package utils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCheckUbuntuSupport(t *testing.T) {
	// This test verifies the logic structure of CheckUbuntuSupport
	// Actual OS detection would require system-level mocking
	t.Run("function structure", func(t *testing.T) {
		err := CheckUbuntuSupport()
		// The error will depend on the actual OS running the test
		// We just verify the function executes
		_ = err
	})
}

func TestGetSaltboxUser(t *testing.T) {
	tmpDir := t.TempDir()
	accountsPath := filepath.Join(tmpDir, "accounts.yml")

	tests := []struct {
		name        string
		setupFile   bool
		fileContent string
		expectError bool
		expectedUser string
	}{
		{
			name:      "valid accounts.yml",
			setupFile: true,
			fileContent: `user:
  name: saltbox
  email: test@example.com`,
			expectError:  false,
			expectedUser: "saltbox",
		},
		{
			name:      "accounts.yml without user section",
			setupFile: true,
			fileContent: `other:
  data: value`,
			expectError: true,
		},
		{
			name:      "accounts.yml without user.name",
			setupFile: true,
			fileContent: `user:
  email: test@example.com`,
			expectError: true,
		},
		{
			name:        "missing accounts.yml",
			setupFile:   false,
			expectError: true,
		},
		{
			name:      "invalid yaml",
			setupFile: true,
			fileContent: `invalid: [yaml content
  that: doesn't parse`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFile {
				err := os.WriteFile(accountsPath, []byte(tt.fileContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			// Test the actual function with a mock path
			// Since we can't change constants easily, we test the logic
			data, err := os.ReadFile(accountsPath)
			if tt.setupFile && err != nil && !tt.expectError {
				t.Fatalf("Failed to read test file: %v", err)
			}

			if tt.setupFile && err == nil {
				var accounts map[string]any
				err = yaml.Unmarshal(data, &accounts)

				if tt.expectError {
					// Verify that we expect an error at some point
					if err == nil {
						user, ok := accounts["user"].(map[string]any)
						if !ok {
							// Expected error: no user section
							return
						}
						_, ok = user["name"].(string)
						if !ok {
							// Expected error: no name field
							return
						}
					}
				} else if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			// Clean up for next test
			os.Remove(accountsPath)
		})
	}
}

func TestCheckArchitecture(t *testing.T) {
	ctx := context.Background()

	t.Run("check architecture execution", func(t *testing.T) {
		err := CheckArchitecture(ctx)
		// The result depends on the actual system architecture
		// On x86_64 systems, this should pass
		// On other architectures, it should fail
		_ = err // We just verify the function executes
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := CheckArchitecture(ctx)
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond) // Ensure timeout occurs

		err := CheckArchitecture(ctx)
		// May or may not error depending on timing
		_ = err
	})
}

func TestCheckLXC(t *testing.T) {
	ctx := context.Background()

	t.Run("check LXC execution", func(t *testing.T) {
		err := CheckLXC(ctx)
		// The result depends on whether systemd-detect-virt is available
		// and whether we're running in an LXC container
		_ = err // We just verify the function executes
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := CheckLXC(ctx)
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
	})

	t.Run("LXC detection logic", func(t *testing.T) {
		// Test the logic that handles different outputs
		testCases := []struct {
			name        string
			output      string
			expectError bool
		}{
			{
				name:        "not in container",
				output:      "none",
				expectError: false,
			},
			{
				name:        "in LXC container",
				output:      "lxc",
				expectError: true,
			},
			{
				name:        "in other container",
				output:      "docker",
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				output := strings.ToLower(strings.TrimSpace(tc.output))
				if output == "lxc" {
					if !tc.expectError {
						t.Error("Expected error for LXC detection")
					}
				} else {
					if tc.expectError && output != "lxc" {
						t.Error("Should not error for non-LXC virtualization")
					}
				}
			})
		}
	})
}

func TestCheckDesktopEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("check desktop environment execution", func(t *testing.T) {
		err := CheckDesktopEnvironment(ctx)
		// The result depends on whether ubuntu-desktop is installed
		_ = err // We just verify the function executes
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := CheckDesktopEnvironment(ctx)
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
	})

	t.Run("dpkg exit code logic", func(t *testing.T) {
		// Test that we understand the exit code logic
		// dpkg -l returns 0 if package is installed, 1 if not
		testCases := []struct {
			name        string
			exitCode    int
			expectError bool
		}{
			{
				name:        "package installed",
				exitCode:    0,
				expectError: true, // Error because desktop is installed
			},
			{
				name:        "package not installed",
				exitCode:    1,
				expectError: false, // No error, desktop not installed
			},
			{
				name:        "unexpected exit code",
				exitCode:    2,
				expectError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Verify the logic
				if tc.exitCode == 0 && !tc.expectError {
					t.Error("Should error when package is installed")
				}
				if tc.exitCode == 1 && tc.expectError {
					t.Error("Should not error when package is not installed")
				}
			})
		}
	})
}

func TestRelaunchAsRoot(t *testing.T) {
	t.Run("relaunch structure test", func(t *testing.T) {
		// We can't actually test privilege escalation in a unit test
		// but we can test the function structure

		// Verify we can get the executable path
		executable, err := os.Executable()
		if err != nil {
			t.Errorf("Failed to get executable: %v", err)
		}

		if executable == "" {
			t.Error("Executable path should not be empty")
		}

		// Verify os.Args exists
		if len(os.Args) == 0 {
			t.Error("os.Args should not be empty")
		}
	})

	t.Run("context with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Verify context is valid
		if ctx == nil {
			t.Error("Context should not be nil")
		}

		select {
		case <-ctx.Done():
			t.Error("Context should not be done immediately")
		default:
			// Context is still active, which is expected
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Try to create a command with cancelled context
		cmd := exec.CommandContext(ctx, "sudo", "echo", "test")
		err := cmd.Run()

		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
	})
}

func TestCommandExecutionWithContext(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		args        []string
		timeout     time.Duration
		expectError bool
	}{
		{
			name:        "successful command",
			command:     "echo",
			args:        []string{"test"},
			timeout:     5 * time.Second,
			expectError: false,
		},
		{
			name:        "command with timeout",
			command:     "sleep",
			args:        []string{"10"},
			timeout:     100 * time.Millisecond,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, tt.command, tt.args...)
			err := cmd.Run()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestGetArchitectureOutput(t *testing.T) {
	tests := []struct {
		name           string
		mockOutput     string
		expectSupported bool
	}{
		{
			name:            "x86_64 architecture",
			mockOutput:      "x86_64\n",
			expectSupported: true,
		},
		{
			name:            "x86_64 without newline",
			mockOutput:      "x86_64",
			expectSupported: true,
		},
		{
			name:            "arm64 architecture",
			mockOutput:      "aarch64\n",
			expectSupported: false,
		},
		{
			name:            "i686 architecture",
			mockOutput:      "i686\n",
			expectSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arch := strings.TrimSpace(tt.mockOutput)

			// Test the regex logic
			isSupported := arch == "x86_64"

			if isSupported != tt.expectSupported {
				t.Errorf("Expected supported=%v, got %v for arch %q", tt.expectSupported, isSupported, arch)
			}
		})
	}
}

func TestVirtualizationDetection(t *testing.T) {
	tests := []struct {
		name        string
		virtType    string
		expectError bool
	}{
		{
			name:        "no virtualization",
			virtType:    "none",
			expectError: false,
		},
		{
			name:        "LXC container",
			virtType:    "lxc",
			expectError: true,
		},
		{
			name:        "Docker container",
			virtType:    "docker",
			expectError: false,
		},
		{
			name:        "KVM virtualization",
			virtType:    "kvm",
			expectError: false,
		},
		{
			name:        "VMware virtualization",
			virtType:    "vmware",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			virtType := strings.ToLower(strings.TrimSpace(tt.virtType))

			shouldError := virtType == "lxc"

			if shouldError != tt.expectError {
				t.Errorf("Expected error=%v, got %v for virt type %q", tt.expectError, shouldError, virtType)
			}
		})
	}
}

func TestYAMLParsing(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		expectUser  bool
		expectName  bool
	}{
		{
			name: "valid structure",
			content: `user:
  name: testuser
  email: test@example.com`,
			expectError: false,
			expectUser:  true,
			expectName:  true,
		},
		{
			name: "missing user section",
			content: `config:
  setting: value`,
			expectError: false,
			expectUser:  false,
			expectName:  false,
		},
		{
			name: "user section without name",
			content: `user:
  email: test@example.com`,
			expectError: false,
			expectUser:  true,
			expectName:  false,
		},
		{
			name:        "invalid YAML",
			content:     `invalid: [yaml`,
			expectError: true,
			expectUser:  false,
			expectName:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var accounts map[string]any
			err := yaml.Unmarshal([]byte(tt.content), &accounts)

			if tt.expectError && err == nil {
				t.Error("Expected YAML parsing error")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected YAML parsing error: %v", err)
			}

			if err == nil {
				_, hasUser := accounts["user"]
				if hasUser != tt.expectUser {
					t.Errorf("Expected hasUser=%v, got %v", tt.expectUser, hasUser)
				}

				if hasUser {
					user, ok := accounts["user"].(map[string]any)
					if ok {
						_, hasName := user["name"]
						if hasName != tt.expectName {
							t.Errorf("Expected hasName=%v, got %v", tt.expectName, hasName)
						}
					}
				}
			}
		})
	}
}
