package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zones"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
)

// CustomValidator function type for custom validation
type CustomValidator func(value any, config map[string]any) error

// AsyncAPIValidator function type for async API validation
type AsyncAPIValidator func(value any, config map[string]any) error

// APIValidationResult holds the result of an async API validation
type APIValidationResult struct {
	Name  string
	Error error
}

// AsyncValidationContext manages async API validations
type AsyncValidationContext struct {
	eg      *errgroup.Group
	results chan APIValidationResult
	errors  []error
	mu      sync.Mutex
}

// NewAsyncValidationContext creates a new async validation context
func NewAsyncValidationContext() *AsyncValidationContext {
	eg := &errgroup.Group{}
	return &AsyncValidationContext{
		eg:      eg,
		results: make(chan APIValidationResult, 10), // Buffer for multiple API validations
	}
}

// AddAPIValidation adds an async API validation to be executed
func (ctx *AsyncValidationContext) AddAPIValidation(name string, validator AsyncAPIValidator, value any, config map[string]any) {
	ctx.eg.Go(func() error {
		err := validator(value, config)
		ctx.results <- APIValidationResult{Name: name, Error: err}
		return nil // We collect errors via channel, not errgroup's error return
	})
}

// Wait waits for all async validations to complete and returns any errors
func (ctx *AsyncValidationContext) Wait() []error {
	// Close the results channel when all goroutines are done
	go func() {
		// We don't use errgroup's error return because errors are collected via the results channel
		// Each goroutine returns nil to errgroup (see AddAPIValidation)
		_ = ctx.eg.Wait() // Errors are collected via channel, not errgroup
		close(ctx.results)
	}()

	// Collect results
	for result := range ctx.results {
		if result.Error != nil {
			ctx.mu.Lock()
			ctx.errors = append(ctx.errors, fmt.Errorf("%s: %w", result.Name, result.Error))
			ctx.mu.Unlock()
		}
	}

	return ctx.errors
}

// customValidators registry of all available custom validators
var customValidators = map[string]CustomValidator{
	"validate_ssh_key_or_url":    validateSSHKeyOrURL,
	"validate_password_strength": validatePasswordStrength,
	"validate_cloudflare_config": validateCloudflareConfigSync,
	"validate_dockerhub_config":  validateDockerhubConfigSync,
	"validate_rclone_remote":     validateRcloneRemote,
	"validate_ansible_bool":      validateAnsibleBool,
	"validate_timezone":          validateTimezone,
	"validate_cron_time":         validateCronTime,
	"validate_directory_path":    validateDirectoryPath,
	"validate_rclone_template":   validateRcloneTemplate,
	"validate_whole_number":      validateWholeNumber,
	"validate_url":               validateURL,
	"validate_positive_number":   validatePositiveNumber,
	"validate_subdomain":         validateSubdomain,
	"validate_hostname":          validateHostnameStrict,
}

// asyncAPIValidators registry of all available async API validators
var asyncAPIValidators = map[string]AsyncAPIValidator{
	"validate_cloudflare_config": validateCloudflareConfigAsync,
	"validate_dockerhub_config":  validateDockerhubConfigAsync,
}

// validateSSHKeyOrURL validates SSH public keys or URLs
func validateSSHKeyOrURL(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil // Optional field
	}

	logging.DebugBool(verboseMode, "validateSSHKeyOrURL called with value: '%s'", str)

	// Check if it's a valid URL
	if isValidURL(str) {
		logging.DebugBool(verboseMode, "validateSSHKeyOrURL - value is a valid URL")
		return nil
	}

	// Check if it's an SSH key
	if isValidSSHKey(str) {
		logging.DebugBool(verboseMode, "validateSSHKeyOrURL - value is a valid SSH key")
		return nil
	}

	return fmt.Errorf("must be a valid SSH public key or URL")
}

// validatePasswordStrength validates password strength and warns about weak passwords
func validatePasswordStrength(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("password must be a string")
	}

	logging.DebugBool(verboseMode, "validatePasswordStrength called with password length: %d", len(str))

	if len(str) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	// Non-fatal warning for short passwords
	if len(str) < 12 {
		fmt.Printf("WARNING: Password is shorter than 12 characters (%d). It's recommended to use a stronger password as some automated application setup flows may require it (Portainer skips user setup as an example).", len(str))
	}

	return nil
}

// validateCloudflareConfigSync validates Cloudflare configuration structure only (no API calls)
func validateCloudflareConfigSync(value any, config map[string]any) error {
	cfConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("cloudflare config must be an object")
	}

	logging.DebugBool(verboseMode, "validateCloudflareConfigSync called with config: %+v", cfConfig)

	_, hasAPI := cfConfig["api"].(string)
	_, hasEmail := cfConfig["email"].(string)

	if !hasAPI && !hasEmail {
		logging.DebugBool(verboseMode, "validateCloudflareConfigSync - both API and email missing, skipping validation")
		return nil // Both missing is OK
	}

	if !hasAPI || !hasEmail {
		return fmt.Errorf("both 'api' and 'email' must be provided together")
	}

	// Validate that user config exists for async validation
	userConfig, ok := config["user"].(map[string]any)
	if !ok {
		return fmt.Errorf("user config is required for Cloudflare validation")
	}

	_, ok = userConfig["domain"].(string)
	if !ok {
		return fmt.Errorf("user domain is required for Cloudflare validation")
	}

	// Structure validation passed - API validation will be done async
	logging.DebugBool(verboseMode, "validateCloudflareConfigSync - structure validation passed")
	return nil
}

// validateCloudflareConfigAsync performs actual Cloudflare API validation
func validateCloudflareConfigAsync(value any, config map[string]any) error {
	startTime := time.Now()
	logging.DebugBool(verboseMode, "validateCloudflareConfigAsync starting at %v", startTime)

	cfConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("cloudflare config must be an object")
	}

	api, hasAPI := cfConfig["api"].(string)
	email, hasEmail := cfConfig["email"].(string)

	if !hasAPI && !hasEmail {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (skipped - no credentials)", time.Since(startTime))
		return nil // Both missing is OK
	}

	if !hasAPI || !hasEmail {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (error - incomplete credentials)", time.Since(startTime))
		return fmt.Errorf("both 'api' and 'email' must be provided together")
	}

	// Get domain from user config for validation
	userConfig, ok := config["user"].(map[string]any)
	if !ok {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (error - no user config)", time.Since(startTime))
		return fmt.Errorf("user config is required for Cloudflare validation")
	}

	domain, ok := userConfig["domain"].(string)
	if !ok {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (error - no domain)", time.Since(startTime))
		return fmt.Errorf("user domain is required for Cloudflare validation")
	}

	// Perform actual Cloudflare API validation
	logging.DebugBool(verboseMode, "validateCloudflareConfigAsync starting API calls for domain: %s", domain)
	err := validateCloudflareCredentials(api, email, domain)
	duration := time.Since(startTime)

	if err != nil {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (API validation failed: %v)", duration, err)
	} else {
		logging.DebugBool(verboseMode, "validateCloudflareConfigAsync completed in %v (API validation successful)", duration)
	}

	return err
}

// validateDockerhubConfigSync validates Docker Hub configuration structure only (no API calls)
func validateDockerhubConfigSync(value any, _ map[string]any) error {
	dhConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("dockerhub config must be an object")
	}

	logging.DebugBool(verboseMode, "validateDockerhubConfigSync called with config: %+v", dhConfig)

	_, hasUser := dhConfig["user"].(string)
	_, hasToken := dhConfig["token"].(string)

	if !hasUser && !hasToken {
		logging.DebugBool(verboseMode, "validateDockerhubConfigSync - both user and token missing, skipping validation")
		return nil // Both missing is OK
	}

	if !hasUser || !hasToken {
		return fmt.Errorf("both 'user' and 'token' must be provided together")
	}

	// Structure validation passed - API validation will be done async
	logging.DebugBool(verboseMode, "validateDockerhubConfigSync - structure validation passed")
	return nil
}

// validateDockerhubConfigAsync performs actual Docker Hub authentication test
func validateDockerhubConfigAsync(value any, _ map[string]any) error {
	startTime := time.Now()
	logging.DebugBool(verboseMode, "validateDockerhubConfigAsync starting at %v", startTime)

	dhConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("dockerhub config must be an object")
	}

	username, hasUser := dhConfig["user"].(string)
	token, hasToken := dhConfig["token"].(string)

	if !hasUser && !hasToken {
		logging.DebugBool(verboseMode, "validateDockerhubConfigAsync completed in %v (skipped - no credentials)", time.Since(startTime))
		return nil // Both missing is OK
	}

	if !hasUser || !hasToken {
		logging.DebugBool(verboseMode, "validateDockerhubConfigAsync completed in %v (error - incomplete credentials)", time.Since(startTime))
		return fmt.Errorf("both 'user' and 'token' must be provided together")
	}

	// Perform actual Docker Hub authentication test
	logging.DebugBool(verboseMode, "validateDockerhubConfigAsync starting API call for user: %s", username)
	err := validateDockerhubCredentials(username, token)
	duration := time.Since(startTime)

	if err != nil {
		logging.DebugBool(verboseMode, "validateDockerhubConfigAsync completed in %v (API validation failed: %v)", duration, err)
	} else {
		logging.DebugBool(verboseMode, "validateDockerhubConfigAsync completed in %v (API validation successful)", duration)
	}

	return err
}

// validateAnsibleBool validates Ansible boolean values
func validateAnsibleBool(value any, _ map[string]any) error {
	logging.DebugBool(verboseMode, "validateAnsibleBool called with value: %v (type: %T)", value, value)

	return validateAnsibleBoolValue(value)
}

// validateAnsibleBoolValue validates a single Ansible boolean value (extracted for reuse)
func validateAnsibleBoolValue(value any) error {
	var str string
	switch v := value.(type) {
	case string:
		str = v
	case bool:
		// Convert boolean to string representation
		if v {
			str = "true"
		} else {
			str = "false"
		}
	default:
		return fmt.Errorf("ansible boolean must be a string or boolean, got: %T", value)
	}

	normalizedValue := strings.ToLower(str)
	switch normalizedValue {
	case "yes", "true", "on", "1", "no", "false", "off", "0":
		return nil
	default:
		return fmt.Errorf("must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s", str)
	}
}

// validateTimezone validates timezone strings or "auto"
func validateTimezone(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("timezone must be a string")
	}

	logging.DebugBool(verboseMode, "validateTimezone called with value: '%s'", str)

	if strings.ToLower(str) == "auto" {
		return nil
	}

	_, err := time.LoadLocation(str)
	if err != nil {
		return fmt.Errorf("invalid timezone: %s", str)
	}

	return nil
}

// validateCronTime validates Ansible cron special time values
func validateCronTime(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cron time must be a string")
	}

	logging.DebugBool(verboseMode, "validateCronTime called with value: '%s'", str)

	normalizedValue := strings.ToLower(str)
	switch normalizedValue {
	case "annually", "daily", "hourly", "monthly", "reboot", "weekly", "yearly":
		return nil
	default:
		return fmt.Errorf("must be a valid Ansible cron special time (annually, daily, hourly, monthly, reboot, weekly, yearly), got: %s", str)
	}
}

// validateDirectoryPath validates directory paths
func validateDirectoryPath(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("directory path must be a string")
	}

	logging.DebugBool(verboseMode, "validateDirectoryPath called with value: '%s'", str)

	// Make path absolute if relative
	dirPath := str
	if !filepath.IsAbs(dirPath) {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine working directory for relative path validation")
		}
		dirPath = filepath.Join(wd, dirPath)
	}

	// Validate path format (simplified check)
	if matched, _ := regexp.MatchString(`^[/\\].*`, dirPath); !matched && !filepath.IsAbs(dirPath) {
		return fmt.Errorf("invalid directory path format: %s", str)
	}

	return nil
}

// validateRcloneTemplate validates rclone template types
func validateRcloneTemplate(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone template must be a string")
	}

	logging.DebugBool(verboseMode, "validateRcloneTemplate called with value: '%s'", str)

	// Check for predefined values
	switch strings.ToLower(str) {
	case "dropbox", "google", "sftp", "nfs":
		return nil
	}

	// Check for absolute path and file existence
	if strings.HasPrefix(str, "/") {
		if _, err := os.Stat(str); err != nil {
			return fmt.Errorf("rclone template file not found: %s", str)
		}
		return nil
	}

	return fmt.Errorf("must be one of 'dropbox', 'google', 'sftp', 'nfs', or a valid absolute file path, got: %s", str)
}

// validateRcloneRemote validates that an rclone remote exists
func validateRcloneRemote(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone remote must be a string")
	}

	logging.DebugBool(verboseMode, "validateRcloneRemote called with value: '%s'", str)

	// Extract remote name from "remote:path" format
	parts := strings.SplitN(str, ":", 2)
	remoteName := str
	if len(parts) == 2 {
		remoteName = parts[0]
	}

	logging.DebugBool(verboseMode, "validateRcloneRemote - checking remote name: '%s'", remoteName)

	// Check if rclone is installed
	if _, err := exec.LookPath("rclone"); err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: rclone is not installed")
		return nil
	}

	// Get the Saltbox user
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: could not retrieve saltbox user: %v", err)
		return nil
	}

	// Check if the user exists on the system
	if _, err := user.Lookup(rcloneUser); err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: user '%s' does not exist", rcloneUser)
		return nil
	}

	// Check if the rclone config file exists
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	if _, err := os.Stat(rcloneConfigPath); os.IsNotExist(err) {
		fmt.Printf("Warning: rclone remote validation skipped: config file not found at %s", rcloneConfigPath)
		return nil
	}

	// Check if the remote exists in rclone config
	// Use context with timeout for external command execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "sudo",
		executor.WithArgs("-u", rcloneUser, "rclone", "config", "show"),
		executor.WithInheritEnv(fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath)),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		return fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, result.Combined)
	}

	output := result.Combined

	// Search for the remote in the output
	remoteRegex := fmt.Sprintf(`(?m)^\[%s\]$`, regexp.QuoteMeta(remoteName))
	if matched, _ := regexp.MatchString(remoteRegex, string(output)); !matched {
		return fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
	}

	return nil
}

// Helper functions for validation

// isValidSSHKey validates SSH public key format
func isValidSSHKey(key string) bool {
	validKeyTypes := []string{"ssh-rsa", "ssh-dss", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-ed25519"}
	keyParts := strings.Fields(key)

	if len(keyParts) < 2 {
		return false
	}

	return slices.Contains(validKeyTypes, keyParts[0])
}

// validateCloudflareCredentials performs actual Cloudflare API validation
func validateCloudflareCredentials(apiKey, email, domain string) error {
	logging.DebugBool(verboseMode, "validateCloudflareCredentials called for domain: %s", domain)

	// Create Cloudflare API client with timeout
	api := cloudflare.NewClient(
		option.WithAPIKey(apiKey),
		option.WithAPIEmail(email),
		option.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Second, // 10 second timeout per request
		}),
	)

	// Verify API key
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - verifying API key")
	_, err := api.User.Get(context.Background())
	if err != nil {
		return fmt.Errorf("cloudflare API key verification failed: %w", err)
	}
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - API key verified")

	// Get root domain for zone lookup
	rootDomain, err := getRootDomain(domain)
	if err != nil {
		return err
	}

	// Verify domain ownership
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - checking domain ownership for %s", rootDomain)
	domainStart := time.Now()
	zonesList, err := api.Zones.List(context.Background(), zones.ZoneListParams{
		Name: cloudflare.F(rootDomain),
	})

	if err != nil {
		return fmt.Errorf("domain verification failed (zone not found): %w", err)
	}

	if len(zonesList.Result) == 0 {
		return fmt.Errorf("domain verification failed: %s not found in Cloudflare account", rootDomain)
	}

	zone := zonesList.Result[0]
	zoneID := zone.ID
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - domain ownership verified in %v", time.Since(domainStart))
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - zone info: ID=%s, Name=%s, Status=%s", zone.ID, zone.Name, zone.Status)

	// Check SSL settings directly (most efficient approach)
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - checking SSL settings")
	sslStart := time.Now()
	ctx := context.Background()
	sslSettings, err := api.Zones.Settings.Get(ctx, "ssl", zones.SettingGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to get zone SSL settings: %w", err)
	}

	// Check for incompatible SSL modes
	if sslSettings != nil && sslSettings.Value != nil {
		if sslValue, ok := sslSettings.Value.(zones.SettingGetResponseZonesSchemasSSLValue); ok {
			if sslValue == zones.SettingGetResponseZonesSchemasSSLValueFlexible ||
				sslValue == zones.SettingGetResponseZonesSchemasSSLValueOff {
				return fmt.Errorf("incompatible SSL/TLS mode detected: '%s'\n"+
					"  This SSL/TLS mode is not compatible with Saltbox."+
					"  Please update your Cloudflare settings:"+
					"  1. Log in to your Cloudflare dashboard"+
					"  2. Go to the SSL/TLS section for domain '%s'"+
					"  3. Change the encryption mode to 'Full' or 'Full (strict)'"+
					"  4. Save your changes",
					string(sslValue), rootDomain)
			}
		}
	}
	logging.DebugBool(verboseMode, "validateCloudflareCredentials - SSL settings verified in %v", time.Since(sslStart))

	return nil
}

// validateDockerhubCredentials performs actual Docker Hub authentication
func validateDockerhubCredentials(username, token string) error {
	logging.DebugBool(verboseMode, "validateDockerhubCredentials called for username: %s", username)

	dockerhubLoginUrl := "https://hub.docker.com/v2/users/login/"
	payload := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, username, token))

	req, err := http.NewRequest("POST", dockerhubLoginUrl, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	// Use client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second, // 10 second timeout
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		var respBody map[string]any
		if json.NewDecoder(res.Body).Decode(&respBody) == nil {
			if message, ok := respBody["message"].(string); ok {
				return fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, message)
			}
			if details, ok := respBody["details"].(string); ok {
				return fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, details)
			}
		}
		return fmt.Errorf("docker Hub authentication failed (HTTP %d)", res.StatusCode)
	}

	return nil
}

// getRootDomain extracts the root domain from a FQDN
func getRootDomain(fqdn string) (string, error) {
	if fqdn == "" {
		return "", fmt.Errorf("empty domain name")
	}

	domain, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
	if err != nil {
		return "", fmt.Errorf("invalid domain format: %s: %w", fqdn, err)
	}

	return domain, nil
}

// validateURLCharacters checks for invalid characters in URLs
func validateURLCharacters(url string) error {
	// RFC 3986 URL allowed characters
	// Unreserved: A-Z a-z 0-9 - . _ ~
	// Reserved: ! * ' ( ) ; : @ & = + $ , / ? # [ ]
	// Percent-encoded: %XX
	validURLPattern := `^[A-Za-z0-9\-._~!*'();:@&=+$,/?#\[\]%]+$`

	matched, err := regexp.MatchString(validURLPattern, url)
	if err != nil {
		return fmt.Errorf("error validating URL characters: %w", err)
	}

	if !matched {
		// Find the first invalid character for better error reporting
		for i, char := range url {
			if !isValidURLCharacter(char) {
				return fmt.Errorf("contains invalid character '%c' at position %d. URLs can only contain letters, numbers, and these special characters: -._~!*'();:@&=+$,/?#[]%%", char, i+1)
			}
		}
		return fmt.Errorf("contains invalid characters. URLs can only contain letters, numbers, and these special characters: -._~!*'();:@&=+$,/?#[]%%")
	}

	return nil
}

// validateSubdomainCharacters checks for invalid characters in subdomains
func validateSubdomainCharacters(subdomain string) error {
	// RFC 1123 subdomain rules:
	// - Can contain letters (a-z, A-Z), digits (0-9), and hyphens (-)
	// - Must start and end with alphanumeric character
	// - Cannot have consecutive hyphens
	// - Length between 1-63 characters

	if len(subdomain) == 0 {
		return fmt.Errorf("subdomain cannot be empty")
	}

	if len(subdomain) > 63 {
		return fmt.Errorf("subdomain cannot be longer than 63 characters, got %d", len(subdomain))
	}

	// Check each character first - this catches invalid characters immediately
	prevWasHyphen := false
	for i, char := range subdomain {
		if !isValidSubdomainCharacter(char) {
			return fmt.Errorf("subdomain contains invalid character '%c' at position %d. Only letters, numbers, and hyphens are allowed", char, i+1)
		}

		if char == '-' {
			if prevWasHyphen {
				return fmt.Errorf("subdomain cannot contain consecutive hyphens at position %d", i+1)
			}
			prevWasHyphen = true
		} else {
			prevWasHyphen = false
		}
	}

	// Check if starts/ends with alphanumeric (only after confirming all chars are valid)
	if !isAlphanumeric(rune(subdomain[0])) {
		return fmt.Errorf("subdomain must start with a letter or number, not '%c'", subdomain[0])
	}

	if !isAlphanumeric(rune(subdomain[len(subdomain)-1])) {
		return fmt.Errorf("subdomain must end with a letter or number, not '%c'", subdomain[len(subdomain)-1])
	}

	return nil
}

// isValidURLCharacter checks if a character is valid in URLs according to RFC 3986
func isValidURLCharacter(char rune) bool {
	return (char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '.' || char == '_' || char == '~' ||
		char == '!' || char == '*' || char == '\'' || char == '(' ||
		char == ')' || char == ';' || char == ':' || char == '@' ||
		char == '&' || char == '=' || char == '+' || char == '$' ||
		char == ',' || char == '/' || char == '?' || char == '#' ||
		char == '[' || char == ']' || char == '%'
}

// isValidSubdomainCharacter checks if a character is valid in subdomains
func isValidSubdomainCharacter(char rune) bool {
	return (char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9') ||
		char == '-'
}

// isAlphanumeric checks if a character is alphanumeric
func isAlphanumeric(char rune) bool {
	return (char >= 'A' && char <= 'Z') ||
		(char >= 'a' && char <= 'z') ||
		(char >= '0' && char <= '9')
}

// validateSubdomain validates subdomain format and characters
func validateSubdomain(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("subdomain must be a string")
	}

	logging.DebugBool(verboseMode, "validateSubdomain called with value: '%s'", str)

	if err := validateSubdomainCharacters(str); err != nil {
		return err
	}

	return nil
}

// validateHostnameStrict validates hostname format and characters with strict RFC compliance
func validateHostnameStrict(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("hostname must be a string")
	}

	logging.DebugBool(verboseMode, "validateHostnameStrict called with value: '%s'", str)

	// Basic format check first
	if !isValidHostname(str) {
		return fmt.Errorf("invalid hostname format")
	}

	// Check each label (part separated by dots) for character compliance
	labels := strings.Split(str, ".")
	for i, label := range labels {
		if err := validateSubdomainCharacters(label); err != nil {
			return fmt.Errorf("invalid characters in hostname label %d ('%s'): %v", i+1, label, err)
		}
	}

	return nil
}

// validateWholeNumber validates that a value is a whole number (integer)
func validateWholeNumber(value any, _ map[string]any) error {
	logging.DebugBool(verboseMode, "validateWholeNumber called with value: %v (type: %T)", value, value)

	switch v := value.(type) {
	case string:
		// String representation of a number
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("must be a whole number (integer), got: %s", v)
		}
		return nil
	case int, int8, int16, int32, int64:
		// Already an integer type
		return nil
	case uint, uint8, uint16, uint32, uint64:
		// Already an unsigned integer type
		return nil
	case float32, float64:
		// Check if it's a whole number (no decimal part)
		floatVal := reflect.ValueOf(v).Float()
		if floatVal != float64(int64(floatVal)) {
			return fmt.Errorf("must be a whole number (integer), got: %v (has decimal part)", v)
		}
		return nil
	default:
		return fmt.Errorf("must be a whole number (integer), got: %v (type: %T)", v, v)
	}
}

// validateURL validates URL format and characters
func validateURL(value any, _ map[string]any) error {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil // Optional field
	}

	logging.DebugBool(verboseMode, "validateURL called with value: '%s'", str)

	// Check basic URL format
	if !isValidURL(str) {
		return fmt.Errorf("must be a valid URL format (e.g., https://example.com)")
	}

	// Check for invalid characters in URL
	if err := validateURLCharacters(str); err != nil {
		return err
	}

	return nil
}

// validatePositiveNumber validates that a number is positive
func validatePositiveNumber(value any, _ map[string]any) error {
	logging.DebugBool(verboseMode, "validatePositiveNumber called with value: %v (type: %T)", value, value)

	switch v := value.(type) {
	case int:
		if v <= 0 {
			return fmt.Errorf("must be greater than 0, got: %d", v)
		}
	case float64:
		if v <= 0 {
			return fmt.Errorf("must be greater than 0, got: %f", v)
		}
	case string:
		if num, err := strconv.Atoi(v); err == nil {
			if num <= 0 {
				return fmt.Errorf("must be greater than 0, got: %d", num)
			}
		} else {
			return fmt.Errorf("must be a valid number, got: %s", v)
		}
	default:
		return fmt.Errorf("must be a number, got: %T", value)
	}

	return nil
}
