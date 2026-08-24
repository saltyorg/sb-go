package saltbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cloudflare/cloudflare-go/v7/option"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = old }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestCredentialValidatorsDoNotLogSecrets(t *testing.T) {
	const (
		cloudflareSecret = "cloudflare-secret-sentinel"
		cloudflareLogin  = "cloudflare-login-sentinel@example.com"
		cloudflareToken  = "cloudflare-scoped-token-secret-sentinel"
		dockerLogin      = "docker-login-sentinel"
		dockerSecret     = "docker-secret-sentinel"
	)
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	output := captureStdout(t, func() {
		if err := validateCloudflareConfigSync(
			map[string]any{"api": cloudflareSecret, "email": cloudflareLogin},
			map[string]any{"user": map[string]any{"domain": "example.com"}},
			true,
		); err != nil {
			t.Fatal(err)
		}
		if err := validateCloudflareConfigSync(
			map[string]any{"scoped_token": cloudflareToken},
			map[string]any{"user": map[string]any{"domain": "example.com"}},
			true,
		); err != nil {
			t.Fatal(err)
		}
		if err := validateDockerhubConfigSync(
			map[string]any{"user": dockerLogin, "token": dockerSecret},
			nil,
			true,
		); err != nil {
			t.Fatal(err)
		}
		_ = validateCloudflareConfigAsync(
			canceledContext,
			map[string]any{"api": cloudflareSecret, "email": cloudflareLogin},
			map[string]any{"user": map[string]any{"domain": "example.com"}},
			true,
		)
		_ = validateCloudflareConfigAsync(
			canceledContext,
			map[string]any{"scoped_token": cloudflareToken},
			map[string]any{"user": map[string]any{"domain": "example.com"}},
			true,
		)
		_ = validateDockerhubConfigAsync(
			canceledContext,
			map[string]any{"user": dockerLogin, "token": dockerSecret},
			nil,
			true,
		)
	})
	for _, secret := range []string{cloudflareSecret, cloudflareLogin, cloudflareToken, dockerLogin, dockerSecret} {
		if strings.Contains(output, secret) {
			t.Fatalf("verbose validation output disclosed credential %q: %s", secret, output)
		}
	}
}

func TestParseCloudflareCredentials(t *testing.T) {
	tests := []struct {
		name       string
		value      any
		configured bool
		tokenMode  bool
		wantError  string
	}{
		{name: "disabled", value: map[string]any{}},
		{name: "global api key", value: map[string]any{"api": "key", "email": "user@example.com"}, configured: true},
		{name: "scoped token", value: map[string]any{"scoped_token": "token"}, configured: true, tokenMode: true},
		{name: "API key only", value: map[string]any{"api": "key"}, wantError: "both 'api' and 'email'"},
		{name: "email only", value: map[string]any{"email": "user@example.com"}, wantError: "both 'api' and 'email'"},
		{name: "mixed token and key", value: map[string]any{"scoped_token": "token", "api": "key", "email": "user@example.com"}, wantError: "cannot be combined"},
		{name: "mixed token and email", value: map[string]any{"scoped_token": "token", "email": "user@example.com"}, wantError: "cannot be combined"},
		{name: "wrong type", value: "token", wantError: "must be an object"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials, configured, err := parseCloudflareCredentials(test.value)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("parseCloudflareCredentials() error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if configured != test.configured {
				t.Fatalf("configured = %t, want %t", configured, test.configured)
			}
			if tokenMode := credentials.ScopedToken != ""; tokenMode != test.tokenMode {
				t.Fatalf("token mode = %t, want %t", tokenMode, test.tokenMode)
			}
		})
	}
}

func TestValidateCloudflareCredentialsAuthHeaders(t *testing.T) {
	tests := []struct {
		name          string
		credentials   cloudflareCredentials
		authorization string
		apiKey        string
		email         string
		wantUserCall  bool
	}{
		{
			name:          "scoped token",
			credentials:   cloudflareCredentials{ScopedToken: "scoped-token"},
			authorization: "Bearer scoped-token",
		},
		{
			name:         "Global API key",
			credentials:  cloudflareCredentials{APIKey: "global-key", Email: "user@example.com"},
			apiKey:       "global-key",
			email:        "user@example.com",
			wantUserCall: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if got := request.Header.Get("Authorization"); got != test.authorization {
					t.Errorf("Authorization = %q, want %q", got, test.authorization)
				}
				if got := request.Header.Get("X-Auth-Key"); got != test.apiKey {
					t.Errorf("X-Auth-Key = %q, want %q", got, test.apiKey)
				}
				if got := request.Header.Get("X-Auth-Email"); got != test.email {
					t.Errorf("X-Auth-Email = %q, want %q", got, test.email)
				}

				writer.Header().Set("Content-Type", "application/json")
				switch request.URL.Path {
				case "/user":
					userCalled = true
					_, _ = writer.Write([]byte(`{"success":true,"result":{"id":"user-id","email":"user@example.com"}}`))
				case "/zones":
					_, _ = writer.Write([]byte(`{"success":true,"result":[{"id":"zone-id","name":"example.com","status":"active"}],"result_info":{"page":1,"per_page":20,"count":1,"total_count":1,"total_pages":1}}`))
				case "/zones/zone-id/settings/ssl":
					_, _ = writer.Write([]byte(`{"success":true,"result":{"id":"ssl","value":"full","editable":true}}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			err := validateCloudflareCredentialsWithOptions(
				context.Background(),
				test.credentials,
				"app.example.com",
				false,
				option.WithBaseURL(server.URL),
				option.WithHTTPClient(server.Client()),
			)
			if err != nil {
				t.Fatal(err)
			}
			if userCalled != test.wantUserCall {
				t.Fatalf("user endpoint called = %t, want %t", userCalled, test.wantUserCall)
			}
		})
	}
}

func TestValidateCloudflareCredentialsPermissionGuidance(t *testing.T) {
	tests := []struct {
		name           string
		failedPath     string
		wantPermission string
	}{
		{
			name:           "zone read",
			failedPath:     "/zones",
			wantPermission: "DNS and Zones → Zone → Read",
		},
		{
			name:           "zone settings read",
			failedPath:     "/zones/zone-id/settings/ssl",
			wantPermission: "DNS and Zones → Zone Settings → Read",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Path == test.failedPath {
					writer.WriteHeader(http.StatusForbidden)
					_, _ = writer.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"messages":[],"result":null}`))
					return
				}
				switch request.URL.Path {
				case "/zones":
					_, _ = writer.Write([]byte(`{"success":true,"result":[{"id":"zone-id","name":"example.com","status":"active"}],"result_info":{"page":1,"per_page":20,"count":1,"total_count":1,"total_pages":1}}`))
				case "/zones/zone-id/settings/ssl":
					_, _ = writer.Write([]byte(`{"success":true,"result":{"id":"ssl","value":"full","editable":true}}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			err := validateCloudflareCredentialsWithOptions(
				t.Context(),
				cloudflareCredentials{ScopedToken: "scoped-token"},
				"app.example.com",
				false,
				option.WithBaseURL(server.URL),
				option.WithHTTPClient(server.Client()),
			)
			if err == nil {
				t.Fatal("validateCloudflareCredentialsWithOptions() returned nil, want permission error")
			}
			message := err.Error()
			for _, want := range []string{
				test.wantPermission,
				"Manage Account → Account API Tokens",
				"My Profile → API Tokens",
				"Include → Specific zone → example.com",
				"Authentication error",
			} {
				if !strings.Contains(message, want) {
					t.Errorf("permission error missing %q:\n%s", want, message)
				}
			}
		})
	}
}

func TestCloudflareGlobalAPIKeyErrorStaysOnPoint(t *testing.T) {
	err := cloudflarePermissionError(
		cloudflareCredentials{APIKey: "global-key", Email: "user@example.com"},
		"Cloudflare zone lookup failed",
		"DNS and Zones",
		"Zone",
		"Read",
		"example.com",
		errors.New("access denied"),
	)
	message := err.Error()
	for _, want := range []string{
		"access denied",
		"User Profile → API Tokens",
		"API Keys",
		"Global API Key",
		"cloudflare.api",
		"cloudflare.email",
		"example.com zone",
		"#preferred-global-api-key",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("Global API Key error missing %q:\n%s", want, message)
		}
	}
	for _, unwanted := range []string{"scoped", "Account API Tokens", "My Profile"} {
		if strings.Contains(message, unwanted) {
			t.Errorf("Global API Key error contains unrelated %q guidance:\n%s", unwanted, message)
		}
	}
}

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
		{name: "Invalid - non-string", url: 123, wantError: true, errorMsg: "must be a string"},
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
		value   any
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
		{
			name:    "Invalid non-string",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSHKeyOrURL(tt.value, nil)
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for value '%v', but got none", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error for value '%v', got: %v", tt.value, err)
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
