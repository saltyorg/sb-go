package validate

import (
	"testing"
)

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		name      string
		subdomain string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid simple subdomain",
			subdomain: "app",
			wantError: false,
		},
		{
			name:      "Valid subdomain with numbers",
			subdomain: "app123",
			wantError: false,
		},
		{
			name:      "Valid subdomain with hyphens",
			subdomain: "my-app",
			wantError: false,
		},
		{
			name:      "Valid subdomain with mixed case",
			subdomain: "MyApp",
			wantError: false,
		},
		{
			name:      "Invalid - starts with hyphen",
			subdomain: "-app",
			wantError: true,
			errorMsg:  "must start with a letter or number",
		},
		{
			name:      "Invalid - ends with hyphen",
			subdomain: "app-",
			wantError: true,
			errorMsg:  "must end with a letter or number",
		},
		{
			name:      "Invalid - consecutive hyphens",
			subdomain: "app--test",
			wantError: true,
			errorMsg:  "cannot contain consecutive hyphens",
		},
		{
			name:      "Invalid - special characters",
			subdomain: "app_test",
			wantError: true,
			errorMsg:  "invalid character",
		},
		{
			name:      "Invalid - spaces",
			subdomain: "app test",
			wantError: true,
			errorMsg:  "invalid character",
		},
		{
			name:      "Invalid - too long",
			subdomain: "a1234567890123456789012345678901234567890123456789012345678901234",
			wantError: true,
			errorMsg:  "cannot be longer than 63 characters",
		},
		{
			name:      "Invalid - empty",
			subdomain: "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubdomain(tt.subdomain, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for subdomain '%s', but got none", tt.subdomain)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for subdomain '%s', got: %v", tt.subdomain, err)
				}
			}
		})
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name      string
		password  any
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid long password",
			password:  "MySecurePassword123!",
			wantError: false,
		},
		{
			name:      "Valid minimum length password",
			password:  "Password123!",
			wantError: false,
		},
		{
			name:      "Short password - warning only",
			password:  "pass123",
			wantError: false, // Warning is printed but no error
		},
		{
			name:      "Empty password",
			password:  "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "Invalid - not a string",
			password:  123,
			wantError: true,
			errorMsg:  "must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePasswordStrength(tt.password, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for password test '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for password test '%s', got: %v", tt.name, err)
				}
			}
		})
	}
}

func TestValidateAnsibleBool(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantError bool
	}{
		// String values
		{name: "String yes", value: "yes", wantError: false},
		{name: "String YES", value: "YES", wantError: false},
		{name: "String true", value: "true", wantError: false},
		{name: "String TRUE", value: "TRUE", wantError: false},
		{name: "String on", value: "on", wantError: false},
		{name: "String ON", value: "ON", wantError: false},
		{name: "String 1", value: "1", wantError: false},
		{name: "String no", value: "no", wantError: false},
		{name: "String NO", value: "NO", wantError: false},
		{name: "String false", value: "false", wantError: false},
		{name: "String FALSE", value: "FALSE", wantError: false},
		{name: "String off", value: "off", wantError: false},
		{name: "String OFF", value: "OFF", wantError: false},
		{name: "String 0", value: "0", wantError: false},

		// Boolean values
		{name: "Bool true", value: true, wantError: false},
		{name: "Bool false", value: false, wantError: false},

		// Invalid values
		{name: "Invalid string", value: "maybe", wantError: true},
		{name: "Invalid number", value: 123, wantError: true},
		{name: "Invalid nil", value: nil, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnsibleBool(tt.value, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for value %v, but got none", tt.value)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for value %v, got: %v", tt.value, err)
				}
			}
		})
	}
}

func TestValidateTimezone(t *testing.T) {
	tests := []struct {
		name      string
		timezone  any
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid - auto",
			timezone:  "auto",
			wantError: false,
		},
		{
			name:      "Valid - AUTO (case insensitive)",
			timezone:  "AUTO",
			wantError: false,
		},
		{
			name:      "Valid - America/New_York",
			timezone:  "America/New_York",
			wantError: false,
		},
		{
			name:      "Valid - Europe/London",
			timezone:  "Europe/London",
			wantError: false,
		},
		{
			name:      "Valid - UTC",
			timezone:  "UTC",
			wantError: false,
		},
		{
			name:      "Invalid timezone",
			timezone:  "Invalid/Timezone",
			wantError: true,
			errorMsg:  "invalid timezone",
		},
		{
			name:      "Invalid - not a string",
			timezone:  123,
			wantError: true,
			errorMsg:  "must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimezone(tt.timezone, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for timezone %v, but got none", tt.timezone)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for timezone %v, got: %v", tt.timezone, err)
				}
			}
		})
	}
}

func TestValidateCronTime(t *testing.T) {
	tests := []struct {
		name      string
		cronTime  any
		wantError bool
		errorMsg  string
	}{
		{name: "Valid - annually", cronTime: "annually", wantError: false},
		{name: "Valid - daily", cronTime: "daily", wantError: false},
		{name: "Valid - hourly", cronTime: "hourly", wantError: false},
		{name: "Valid - monthly", cronTime: "monthly", wantError: false},
		{name: "Valid - reboot", cronTime: "reboot", wantError: false},
		{name: "Valid - weekly", cronTime: "weekly", wantError: false},
		{name: "Valid - yearly", cronTime: "yearly", wantError: false},
		{name: "Valid - DAILY (uppercase)", cronTime: "DAILY", wantError: false},
		{name: "Invalid cron time", cronTime: "invalid", wantError: true, errorMsg: "must be a valid Ansible cron special time"},
		{name: "Invalid - not a string", cronTime: 123, wantError: true, errorMsg: "must be a string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCronTime(tt.cronTime, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for cron time %v, but got none", tt.cronTime)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for cron time %v, got: %v", tt.cronTime, err)
				}
			}
		})
	}
}

func TestValidateWholeNumber(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantError bool
		errorMsg  string
	}{
		// Integer types
		{name: "Valid int", value: 42, wantError: false},
		{name: "Valid int zero", value: 0, wantError: false},
		{name: "Valid int negative", value: -10, wantError: false},
		{name: "Valid uint", value: uint(42), wantError: false},

		// String representations
		{name: "Valid string int", value: "42", wantError: false},
		{name: "Valid string zero", value: "0", wantError: false},
		{name: "Valid string negative", value: "-10", wantError: false},

		// Float whole numbers
		{name: "Valid float whole", value: 42.0, wantError: false},
		{name: "Valid float32 whole", value: float32(42.0), wantError: false},

		// Invalid values
		{name: "Invalid float with decimal", value: 42.5, wantError: true, errorMsg: "has decimal part"},
		{name: "Invalid string non-number", value: "abc", wantError: true, errorMsg: "must be a whole number"},
		{name: "Invalid string float", value: "42.5", wantError: true, errorMsg: "must be a whole number"},
		{name: "Invalid bool", value: true, wantError: true, errorMsg: "must be a whole number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWholeNumber(tt.value, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for value %v, but got none", tt.value)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for value %v, got: %v", tt.value, err)
				}
			}
		})
	}
}

func TestValidatePositiveNumber(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantError bool
		errorMsg  string
	}{
		// Valid positive numbers
		{name: "Valid int", value: 42, wantError: false},
		{name: "Valid float", value: 3.14, wantError: false},
		{name: "Valid string", value: "100", wantError: false},

		// Invalid values
		{name: "Invalid zero", value: 0, wantError: true, errorMsg: "must be greater than 0"},
		{name: "Invalid negative int", value: -5, wantError: true, errorMsg: "must be greater than 0"},
		{name: "Invalid negative float", value: -3.14, wantError: true, errorMsg: "must be greater than 0"},
		{name: "Invalid string zero", value: "0", wantError: true, errorMsg: "must be greater than 0"},
		{name: "Invalid string negative", value: "-10", wantError: true, errorMsg: "must be greater than 0"},
		{name: "Invalid string non-number", value: "abc", wantError: true, errorMsg: "must be a valid number"},
		{name: "Invalid bool", value: true, wantError: true, errorMsg: "must be a number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePositiveNumber(tt.value, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for value %v, but got none", tt.value)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for value %v, got: %v", tt.value, err)
				}
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		url       any
		wantError bool
		errorMsg  string
	}{
		// Valid URLs
		{name: "Valid https URL", url: "https://example.com", wantError: false},
		{name: "Valid http URL", url: "http://example.com", wantError: false},
		{name: "Valid URL with path", url: "https://example.com/path/to/resource", wantError: false},
		{name: "Valid URL with query", url: "https://example.com?key=value", wantError: false},
		{name: "Empty string (optional)", url: "", wantError: false},

		// Invalid URLs
		{name: "Invalid - no scheme", url: "example.com", wantError: true, errorMsg: "must be a valid URL format"},
		{name: "Invalid - invalid characters", url: "https://example.com/<script>", wantError: true, errorMsg: "contains invalid character"}, // The validator rejects < and >
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url, nil)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for URL %v, but got none", tt.url)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for URL %v, got: %v", tt.url, err)
				}
			}
		})
	}
}

func TestIsValidSSHKey(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		valid bool
	}{
		{
			name:  "Valid ssh-rsa key",
			key:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host",
			valid: true,
		},
		{
			name:  "Valid sk-ecdsa key",
			key:   "sk-ecdsa-sha2-nistp256@openssh.com AAAAInNrLWVjZHNh... user@host",
			valid: true,
		},
		{
			name:  "Valid ssh-xmss key",
			key:   "ssh-xmss@openssh.com AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host",
			valid: true,
		},
		{
			name:  "Valid rsa-sha2-512 key",
			key:   "rsa-sha2-512 AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host",
			valid: true,
		},
		{
			name:  "Valid key with options",
			key:   "command=\"echo hello world\" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host",
			valid: true,
		},
		{
			name:  "Valid ssh-ed25519 key",
			key:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host",
			valid: true,
		},
		{
			name:  "Valid ecdsa key",
			key:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNT... user@host",
			valid: true,
		},
		{
			name:  "Invalid - no key data",
			key:   "ssh-rsa",
			valid: false,
		},
		{
			name:  "Invalid - unknown key type",
			key:   "unknown-type AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host",
			valid: false,
		},
		{
			name:  "Invalid - empty",
			key:   "",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSSHKey(tt.key)
			if result != tt.valid {
				t.Errorf("Expected isValidSSHKey(%s) = %v, got %v", tt.key, tt.valid, result)
			}
		})
	}
}

func TestValidateSSHKeyOrURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "Valid https URL",
			value:   "https://github.com/user.keys",
			wantErr: false,
		},
		{
			name:    "Valid file URL",
			value:   "file:///home/user/.ssh/id_ed25519.pub",
			wantErr: false,
		},
		{
			name:    "Valid multiple keys with comments",
			value:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host\n# comment\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC... user@host",
			wantErr: false,
		},
		{
			name:    "Invalid key",
			value:   "ssh-rsa",
			wantErr: true,
		},
		{
			name:    "Invalid value",
			value:   "not-a-key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSHKeyOrURL(tt.value, nil)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for value '%s', but got none", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error for value '%s', got: %v", tt.value, err)
			}
		})
	}
}

func TestValidateSubdomainCharacters(t *testing.T) {
	tests := []struct {
		name      string
		subdomain string
		wantError bool
	}{
		{name: "Valid subdomain", subdomain: "app", wantError: false},
		{name: "Valid with numbers", subdomain: "app123", wantError: false},
		{name: "Valid with hyphens", subdomain: "my-app", wantError: false},
		{name: "Invalid underscore", subdomain: "my_app", wantError: true},
		{name: "Invalid space", subdomain: "my app", wantError: true},
		{name: "Invalid special char", subdomain: "my@app", wantError: true},
		{name: "Invalid starts with hyphen", subdomain: "-app", wantError: true},
		{name: "Invalid ends with hyphen", subdomain: "app-", wantError: true},
		{name: "Invalid consecutive hyphens", subdomain: "app--test", wantError: true},
		{name: "Invalid too long", subdomain: "a1234567890123456789012345678901234567890123456789012345678901234", wantError: true},
		{name: "Invalid empty", subdomain: "", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubdomainCharacters(tt.subdomain)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for subdomain '%s', but got none", tt.subdomain)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for subdomain '%s', got: %v", tt.subdomain, err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
