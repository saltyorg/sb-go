package saltbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/saltyorg/sb-go/terminal"

	"github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/cloudflare/cloudflare-go/v7/zones"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/sync/errgroup"
)

// CustomValidator function type for custom validation
type CustomValidator func(value any, config map[string]any, verbose ...bool) error

// AsyncAPIValidator function type for async API validation
type AsyncAPIValidator func(context.Context, any, map[string]any, ...bool) error

type nonFatalValidationWarning struct {
	message string
}

func (w *nonFatalValidationWarning) Error() string {
	return w.message
}

// APIValidationResult holds the result of an async API validation
type APIValidationResult struct {
	Name  string
	Error error
}

// AsyncValidationContext manages async API validations
type AsyncValidationContext struct {
	ctx     context.Context
	task    *terminal.Task
	eg      *errgroup.Group
	results chan APIValidationResult
	errors  []error
	mu      sync.Mutex
	verbose bool
}

// NewAsyncValidationContext creates a new async validation context
func NewAsyncValidationContext(ctx context.Context, task *terminal.Task, verbose ...bool) *AsyncValidationContext {
	eg := &errgroup.Group{}
	return &AsyncValidationContext{
		ctx:     ctx,
		task:    task,
		eg:      eg,
		results: make(chan APIValidationResult, 10), // Buffer for multiple API validations
		verbose: validationVerbose(verbose),
	}
}

// AddAPIValidation adds an async API validation to be executed
func (ctx *AsyncValidationContext) AddAPIValidation(name string, validator AsyncAPIValidator, value any, config map[string]any) {
	ctx.eg.Go(func() error {
		label := apiValidationLabel(name)
		err := ctx.task.Run(ctx.ctx, terminal.TaskSpec{
			Running: "Validating " + label,
			Success: label + " validated",
			Failure: label + " validation",
		}, func(context.Context, *terminal.Task) error {
			return validator(ctx.ctx, value, config, ctx.verbose)
		})
		ctx.results <- APIValidationResult{Name: name, Error: err}
		return nil // We collect errors via channel, not errgroup's error return
	})
}

func validationVerbose(verbose []bool) bool {
	return len(verbose) > 0 && verbose[0]
}

func apiValidationLabel(name string) string {
	lowerName := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lowerName, "cloudflare"):
		return "Cloudflare API credentials"
	case strings.HasSuffix(lowerName, "dockerhub"):
		return "Docker Hub credentials"
	default:
		return name + " API credentials"
	}
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

func getNonEmptyString(config map[string]any, key string) (string, bool) {
	value, ok := config[key]
	if !ok {
		return "", false
	}

	str, ok := value.(string)
	if !ok || str == "" {
		return "", false
	}

	return str, true
}

// validateSSHKeyOrURL validates SSH public keys or URLs
func validateSSHKeyOrURL(value any, _ map[string]any, verbose ...bool) error {
	if value == nil {
		return nil // Optional field
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil // Optional field
	}

	terminal.DebugBool(validationVerbose(verbose), "validateSSHKeyOrURL called with value: '%s'", str)

	if IsValidAuthorizedKeyOrURL(str) {
		terminal.DebugBool(validationVerbose(verbose), "validateSSHKeyOrURL - value is a valid SSH key or URL")
		return nil
	}

	return fmt.Errorf("must be a valid SSH public key or URL")
}

// validatePasswordStrength validates password strength and warns about weak passwords
func validatePasswordStrength(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("password must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validatePasswordStrength called with password length: %d", len(str))

	if len(str) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	return nil
}

func passwordStrengthWarning(value any) string {
	password, ok := value.(string)
	if !ok || len(password) == 0 || len(password) >= 12 {
		return ""
	}
	return fmt.Sprintf("WARNING: Password is shorter than 12 characters (%d). It's recommended to use a stronger password as some automated application setup flows may require it (Portainer skips user setup as an example).", len(password))
}

type cloudflareCredentials struct {
	APIKey      string
	Email       string
	ScopedToken string
}

func parseCloudflareCredentials(value any) (cloudflareCredentials, bool, error) {
	cfConfig, ok := value.(map[string]any)
	if !ok {
		return cloudflareCredentials{}, false, fmt.Errorf("cloudflare config must be an object")
	}

	apiKey, hasAPIKey := getNonEmptyString(cfConfig, "api")
	email, hasEmail := getNonEmptyString(cfConfig, "email")
	scopedToken, hasScopedToken := getNonEmptyString(cfConfig, "scoped_token")

	if hasScopedToken && (hasAPIKey || hasEmail) {
		return cloudflareCredentials{}, false, fmt.Errorf("'scoped_token' cannot be combined with 'api' or 'email'")
	}
	if hasScopedToken {
		return cloudflareCredentials{ScopedToken: scopedToken}, true, nil
	}
	if !hasAPIKey && !hasEmail {
		return cloudflareCredentials{}, false, nil
	}
	if !hasAPIKey || !hasEmail {
		return cloudflareCredentials{}, false, fmt.Errorf("both 'api' and 'email' must be provided together")
	}

	return cloudflareCredentials{APIKey: apiKey, Email: email}, true, nil
}

// validateCloudflareConfigSync validates Cloudflare configuration structure only (no API calls)
func validateCloudflareConfigSync(value any, config map[string]any, verbose ...bool) error {
	credentials, configured, err := parseCloudflareCredentials(value)
	if err != nil {
		return err
	}
	terminal.DebugBool(
		validationVerbose(verbose),
		"validateCloudflareConfigSync called (configured: %t, scoped token mode: %t)",
		configured,
		credentials.ScopedToken != "",
	)

	if !configured {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigSync - credentials missing, skipping validation")
		return nil
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
	terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigSync - structure validation passed")
	return nil
}

// validateCloudflareConfigAsync performs actual Cloudflare API validation
func validateCloudflareConfigAsync(ctx context.Context, value any, config map[string]any, verbose ...bool) error {
	startTime := time.Now()
	terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync starting at %v", startTime)

	credentials, configured, err := parseCloudflareCredentials(value)
	if err != nil {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (error - invalid credential structure)", time.Since(startTime))
		return err
	}

	if !configured {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (skipped - no credentials)", time.Since(startTime))
		return nil
	}

	// Get domain from user config for validation
	userConfig, ok := config["user"].(map[string]any)
	if !ok {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (error - no user config)", time.Since(startTime))
		return fmt.Errorf("user config is required for Cloudflare validation")
	}

	domain, ok := userConfig["domain"].(string)
	if !ok {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (error - no domain)", time.Since(startTime))
		return fmt.Errorf("user domain is required for Cloudflare validation")
	}

	// Perform actual Cloudflare API validation
	terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync starting API calls for domain: %s", domain)
	err = validateCloudflareCredentials(ctx, credentials, domain, validationVerbose(verbose))
	duration := time.Since(startTime)

	if err != nil {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (API validation failed: %v)", duration, err)
	} else {
		terminal.DebugBool(validationVerbose(verbose), "validateCloudflareConfigAsync completed in %v (API validation successful)", duration)
	}

	return err
}

// validateDockerhubConfigSync validates Docker Hub configuration structure only (no API calls)
func validateDockerhubConfigSync(value any, _ map[string]any, verbose ...bool) error {
	dhConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("dockerhub config must be an object")
	}

	_, hasUser := getNonEmptyString(dhConfig, "user")
	_, hasToken := getNonEmptyString(dhConfig, "token")
	terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigSync called (user configured: %t, token configured: %t)", hasUser, hasToken)

	if !hasUser && !hasToken {
		terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigSync - both user and token missing, skipping validation")
		return nil // Both missing is OK
	}

	if !hasUser || !hasToken {
		return fmt.Errorf("both 'user' and 'token' must be provided together")
	}

	// Structure validation passed - API validation will be done async
	terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigSync - structure validation passed")
	return nil
}

// validateDockerhubConfigAsync performs actual Docker Hub authentication test
func validateDockerhubConfigAsync(ctx context.Context, value any, _ map[string]any, verbose ...bool) error {
	startTime := time.Now()
	terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync starting at %v", startTime)

	dhConfig, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("dockerhub config must be an object")
	}

	username, hasUser := getNonEmptyString(dhConfig, "user")
	token, hasToken := getNonEmptyString(dhConfig, "token")

	if !hasUser && !hasToken {
		terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync completed in %v (skipped - no credentials)", time.Since(startTime))
		return nil // Both missing is OK
	}

	if !hasUser || !hasToken {
		terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync completed in %v (error - incomplete credentials)", time.Since(startTime))
		return fmt.Errorf("both 'user' and 'token' must be provided together")
	}

	// Perform actual Docker Hub authentication test
	terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync starting API call for configured user")
	err := validateDockerhubCredentials(ctx, username, token, validationVerbose(verbose))
	duration := time.Since(startTime)

	if err != nil {
		terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync completed in %v (API validation failed: %v)", duration, err)
	} else {
		terminal.DebugBool(validationVerbose(verbose), "validateDockerhubConfigAsync completed in %v (API validation successful)", duration)
	}

	return err
}

// validateAnsibleBool validates Ansible boolean values
func validateAnsibleBool(value any, _ map[string]any, verbose ...bool) error {
	terminal.DebugBool(validationVerbose(verbose), "validateAnsibleBool called with value: %v (type: %T)", value, value)

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
func validateTimezone(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("timezone must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateTimezone called with value: '%s'", str)

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
func validateCronTime(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cron time must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateCronTime called with value: '%s'", str)

	normalizedValue := strings.ToLower(str)
	switch normalizedValue {
	case "annually", "daily", "hourly", "monthly", "reboot", "weekly", "yearly":
		return nil
	default:
		return fmt.Errorf("must be a valid Ansible cron special time (annually, daily, hourly, monthly, reboot, weekly, yearly), got: %s", str)
	}
}

// validateDirectoryPath validates directory paths
func validateDirectoryPath(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("directory path must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateDirectoryPath called with value: '%s'", str)

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
func validateRcloneTemplate(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone template must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateRcloneTemplate called with value: '%s'", str)

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
func validateRcloneRemote(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone remote must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateRcloneRemote called with value: '%s'", str)

	// Extract remote name from "remote:path" format
	parts := strings.SplitN(str, ":", 2)
	remoteName := str
	if len(parts) == 2 {
		remoteName = parts[0]
	}

	terminal.DebugBool(validationVerbose(verbose), "validateRcloneRemote - checking remote name: '%s'", remoteName)
	if err := ValidateRcloneRemote(remoteName, validationVerbose(verbose)); err != nil {
		switch {
		case errors.Is(err, ErrRcloneNotInstalled):
			return &nonFatalValidationWarning{message: "Warning: rclone remote validation skipped: rclone is not installed"}
		case errors.Is(err, ErrSystemUserNotFound), errors.Is(err, ErrRcloneConfigNotFound):
			return &nonFatalValidationWarning{message: fmt.Sprintf("Warning: rclone remote validation skipped: %v", err)}
		default:
			return err
		}
	}

	return nil
}

// Helper functions for validation

// isValidSSHKey validates SSH public key format
func isValidSSHKey(key string) bool {
	return IsValidAuthorizedKeyLine(key)
}

// validateCloudflareCredentials performs actual Cloudflare API validation
func validateCloudflareCredentials(
	ctx context.Context,
	credentials cloudflareCredentials,
	domain string,
	verbose ...bool,
) error {
	return validateCloudflareCredentialsWithOptions(ctx, credentials, domain, validationVerbose(verbose))
}

func validateCloudflareCredentialsWithOptions(
	ctx context.Context,
	credentials cloudflareCredentials,
	domain string,
	verbose bool,
	extraOptions ...option.RequestOption,
) error {
	terminal.DebugBool(verbose, "validateCloudflareCredentials called for domain: %s", domain)

	options := []option.RequestOption{
		option.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
	}
	options = append(options, extraOptions...)
	if credentials.ScopedToken != "" {
		options = append(options, option.WithAPIToken(credentials.ScopedToken))
	} else {
		options = append(
			options,
			option.WithAPIKey(credentials.APIKey),
			option.WithAPIEmail(credentials.Email),
		)
	}
	api := cloudflare.NewClient(options...)

	if credentials.ScopedToken == "" {
		terminal.DebugBool(verbose, "validateCloudflareCredentials - verifying Global API key")
		if _, err := api.User.Get(ctx); err != nil {
			return fmt.Errorf("cloudflare Global API key verification failed: %w", err)
		}
		terminal.DebugBool(verbose, "validateCloudflareCredentials - Global API key verified")
	}

	// Get root domain for zone lookup
	rootDomain, err := getRootDomain(domain)
	if err != nil {
		return err
	}

	// Verify domain ownership
	terminal.DebugBool(verbose, "validateCloudflareCredentials - checking domain ownership for %s", rootDomain)
	domainStart := time.Now()
	zonesList, err := api.Zones.List(ctx, zones.ZoneListParams{
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
	terminal.DebugBool(verbose, "validateCloudflareCredentials - domain ownership verified in %v", time.Since(domainStart))
	terminal.DebugBool(verbose, "validateCloudflareCredentials - zone info: ID=%s, Name=%s, Status=%s", zone.ID, zone.Name, zone.Status)

	// Check SSL settings directly (most efficient approach)
	terminal.DebugBool(verbose, "validateCloudflareCredentials - checking SSL settings")
	sslStart := time.Now()
	sslSettings, err := api.Zones.Settings.Get(ctx, "ssl", zones.SettingGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to get zone SSL settings: %w", err)
	}

	// Check for incompatible SSL modes
	if sslSettings != nil {
		if sslSetting, ok := sslSettings.AsUnion().(zones.SettingGetResponseZonesSSL2); ok {
			sslValue := sslSetting.Value
			if sslValue == zones.SettingGetResponseZonesSSL2ValueFlexible ||
				sslValue == zones.SettingGetResponseZonesSSL2ValueOff {
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
	terminal.DebugBool(verbose, "validateCloudflareCredentials - SSL settings verified in %v", time.Since(sslStart))

	return nil
}

// validateDockerhubCredentials performs actual Docker Hub authentication
func validateDockerhubCredentials(ctx context.Context, username, token string, verbose ...bool) error {
	terminal.DebugBool(validationVerbose(verbose), "validateDockerhubCredentials called for configured username")

	dockerhubLoginUrl := "https://hub.docker.com/v2/users/login/"
	payload := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, username, token))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dockerhubLoginUrl, payload)
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
func validateSubdomain(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("subdomain must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateSubdomain called with value: '%s'", str)

	if err := validateSubdomainCharacters(str); err != nil {
		return err
	}

	return nil
}

// validateHostnameStrict validates hostname format and characters with strict RFC compliance
func validateHostnameStrict(value any, _ map[string]any, verbose ...bool) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("hostname must be a string")
	}

	terminal.DebugBool(validationVerbose(verbose), "validateHostnameStrict called with value: '%s'", str)

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
func validateWholeNumber(value any, _ map[string]any, verbose ...bool) error {
	terminal.DebugBool(validationVerbose(verbose), "validateWholeNumber called with value: %v (type: %T)", value, value)

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
func validateURL(value any, _ map[string]any, verbose ...bool) error {
	if value == nil {
		return nil // Optional field
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("must be a string")
	}

	if str == "" {
		return nil // Optional field
	}

	terminal.DebugBool(validationVerbose(verbose), "validateURL called with value: '%s'", str)

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
func validatePositiveNumber(value any, _ map[string]any, verbose ...bool) error {
	terminal.DebugBool(validationVerbose(verbose), "validatePositiveNumber called with value: %v (type: %T)", value, value)

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
