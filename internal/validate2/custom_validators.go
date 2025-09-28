package validate2

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
	"strconv"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zones"
	"github.com/saltyorg/sb-go/internal/utils"
	"golang.org/x/net/publicsuffix"
)

// CustomValidator function type for custom validation
type CustomValidator func(value interface{}, config map[string]interface{}) error

// customValidators registry of all available custom validators
var customValidators = map[string]CustomValidator{
	"validate_ssh_key_or_url":    validateSSHKeyOrURL,
	"validate_password_strength": validatePasswordStrength,
	"validate_cloudflare_config": validateCloudflareConfig,
	"validate_dockerhub_config":  validateDockerhubConfig,
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

// validateSSHKeyOrURL validates SSH public keys or URLs
func validateSSHKeyOrURL(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil // Optional field
	}

	debugPrintf("DEBUG: validateSSHKeyOrURL called with value: '%s'\n", str)

	// Check if it's a valid URL
	if isValidURL(str) {
		debugPrintf("DEBUG: validateSSHKeyOrURL - value is a valid URL\n")
		return nil
	}

	// Check if it's an SSH key
	if isValidSSHKey(str) {
		debugPrintf("DEBUG: validateSSHKeyOrURL - value is a valid SSH key\n")
		return nil
	}

	return fmt.Errorf("must be a valid SSH public key or URL")
}

// validatePasswordStrength validates password strength and warns about weak passwords
func validatePasswordStrength(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("password must be a string")
	}

	debugPrintf("DEBUG: validatePasswordStrength called with password length: %d\n", len(str))

	if len(str) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	// Non-fatal warning for short passwords
	if len(str) < 12 {
		fmt.Printf("WARNING: Password is shorter than 12 characters (%d). It's recommended to use a stronger password as some automated application setup flows may require it (Portainer skips user setup as an example).\n", len(str))
	}

	return nil
}

// validateCloudflareConfig validates Cloudflare configuration including API credentials and SSL settings
func validateCloudflareConfig(value interface{}, config map[string]interface{}) error {
	cfConfig, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("cloudflare config must be an object")
	}

	debugPrintf("DEBUG: validateCloudflareConfig called with config: %+v\n", cfConfig)

	api, hasAPI := cfConfig["api"].(string)
	email, hasEmail := cfConfig["email"].(string)

	if !hasAPI && !hasEmail {
		debugPrintf("DEBUG: validateCloudflareConfig - both API and email missing, skipping validation\n")
		return nil // Both missing is OK
	}

	if !hasAPI || !hasEmail {
		return fmt.Errorf("both 'api' and 'email' must be provided together")
	}

	// Get domain from user config for validation
	userConfig, ok := config["user"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("user config is required for Cloudflare validation")
	}

	domain, ok := userConfig["domain"].(string)
	if !ok {
		return fmt.Errorf("user domain is required for Cloudflare validation")
	}

	// Perform actual Cloudflare API validation
	return validateCloudflareCredentials(api, email, domain)
}

// validateDockerhubConfig validates Docker Hub configuration and credentials
func validateDockerhubConfig(value interface{}, config map[string]interface{}) error {
	dhConfig, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("dockerhub config must be an object")
	}

	debugPrintf("DEBUG: validateDockerhubConfig called with config: %+v\n", dhConfig)

	username, hasUser := dhConfig["user"].(string)
	token, hasToken := dhConfig["token"].(string)

	if !hasUser && !hasToken {
		debugPrintf("DEBUG: validateDockerhubConfig - both user and token missing, skipping validation\n")
		return nil // Both missing is OK
	}

	if !hasUser || !hasToken {
		return fmt.Errorf("both 'user' and 'token' must be provided together")
	}

	// Perform actual Docker Hub authentication test
	return validateDockerhubCredentials(username, token)
}

// validateAnsibleBool validates Ansible boolean values
func validateAnsibleBool(value interface{}, config map[string]interface{}) error {
	debugPrintf("DEBUG: validateAnsibleBool called with value: %v (type: %T)\n", value, value)

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

	debugPrintf("DEBUG: validateAnsibleBool normalized value: '%s'\n", str)

	normalizedValue := strings.ToLower(str)
	switch normalizedValue {
	case "yes", "true", "on", "1", "no", "false", "off", "0":
		return nil
	default:
		return fmt.Errorf("must be a valid Ansible boolean (yes/no, true/false, on/off, 1/0), got: %s", str)
	}
}

// validateTimezone validates timezone strings or "auto"
func validateTimezone(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("timezone must be a string")
	}

	debugPrintf("DEBUG: validateTimezone called with value: '%s'\n", str)

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
func validateCronTime(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("cron time must be a string")
	}

	debugPrintf("DEBUG: validateCronTime called with value: '%s'\n", str)

	normalizedValue := strings.ToLower(str)
	switch normalizedValue {
	case "annually", "daily", "hourly", "monthly", "reboot", "weekly", "yearly":
		return nil
	default:
		return fmt.Errorf("must be a valid Ansible cron special time (annually, daily, hourly, monthly, reboot, weekly, yearly), got: %s", str)
	}
}

// validateDirectoryPath validates directory paths
func validateDirectoryPath(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("directory path must be a string")
	}

	debugPrintf("DEBUG: validateDirectoryPath called with value: '%s'\n", str)

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
func validateRcloneTemplate(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone template must be a string")
	}

	debugPrintf("DEBUG: validateRcloneTemplate called with value: '%s'\n", str)

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
func validateRcloneRemote(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("rclone remote must be a string")
	}

	debugPrintf("DEBUG: validateRcloneRemote called with value: '%s'\n", str)

	// Extract remote name from "remote:path" format
	parts := strings.SplitN(str, ":", 2)
	remoteName := str
	if len(parts) == 2 {
		remoteName = parts[0]
	}

	debugPrintf("DEBUG: validateRcloneRemote - checking remote name: '%s'\n", remoteName)

	// Check if rclone is installed
	if _, err := exec.LookPath("rclone"); err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: rclone is not installed\n")
		return nil
	}

	// Get the Saltbox user
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: could not retrieve saltbox user: %v\n", err)
		return nil
	}

	// Check if the user exists on the system
	if _, err := user.Lookup(rcloneUser); err != nil {
		fmt.Printf("Warning: rclone remote validation skipped: user '%s' does not exist\n", rcloneUser)
		return nil
	}

	// Check if the rclone config file exists
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	if _, err := os.Stat(rcloneConfigPath); os.IsNotExist(err) {
		fmt.Printf("Warning: rclone remote validation skipped: config file not found at %s\n", rcloneConfigPath)
		return nil
	}

	// Check if the remote exists in rclone config
	cmd := exec.Command("sudo", "-u", rcloneUser, "rclone", "config", "show")
	cmd.Env = append(os.Environ(), fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, output)
	}

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

	for _, keyType := range validKeyTypes {
		if keyParts[0] == keyType {
			return true
		}
	}

	return false
}

// validateCloudflareCredentials performs actual Cloudflare API validation
func validateCloudflareCredentials(apiKey, email, domain string) error {
	debugPrintf("DEBUG: validateCloudflareCredentials called for domain: %s\n", domain)

	// Create Cloudflare API client
	api := cloudflare.NewClient(
		option.WithAPIKey(apiKey),
		option.WithAPIEmail(email),
	)

	// Verify API key
	_, err := api.User.Get(context.Background())
	if err != nil {
		return fmt.Errorf("cloudflare API key verification failed: %w", err)
	}

	// Get root domain for zone lookup
	rootDomain, err := getRootDomain(domain)
	if err != nil {
		return err
	}

	// Verify domain ownership
	zonesList, err := api.Zones.List(context.Background(), zones.ZoneListParams{
		Name: cloudflare.F(rootDomain),
	})
	if err != nil {
		return fmt.Errorf("domain verification failed (zone not found): %w", err)
	}

	if len(zonesList.Result) == 0 {
		return fmt.Errorf("domain verification failed: %s not found in Cloudflare account", rootDomain)
	}

	zoneID := zonesList.Result[0].ID

	// Verify SSL/TLS settings
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
				return fmt.Errorf("incompatible SSL/TLS mode detected: '%s'\n\n"+
					"  This SSL/TLS mode is not compatible with Saltbox.\n"+
					"  Please update your Cloudflare settings:\n"+
					"  1. Log in to your Cloudflare dashboard\n"+
					"  2. Go to the SSL/TLS section for domain '%s'\n"+
					"  3. Change the encryption mode to 'Full' or 'Full (strict)'\n"+
					"  4. Save your changes\n",
					string(sslValue), rootDomain)
			}
		}
	}

	return nil
}

// validateDockerhubCredentials performs actual Docker Hub authentication
func validateDockerhubCredentials(username, token string) error {
	debugPrintf("DEBUG: validateDockerhubCredentials called for username: %s\n", username)

	dockerhubLoginUrl := "https://hub.docker.com/v2/users/login/"
	payload := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, username, token))

	req, err := http.NewRequest("POST", dockerhubLoginUrl, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var respBody map[string]interface{}
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
func validateSubdomain(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("subdomain must be a string")
	}

	debugPrintf("DEBUG: validateSubdomain called with value: '%s'\n", str)

	if err := validateSubdomainCharacters(str); err != nil {
		return err
	}

	return nil
}

// validateHostnameStrict validates hostname format and characters with strict RFC compliance
func validateHostnameStrict(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("hostname must be a string")
	}

	debugPrintf("DEBUG: validateHostnameStrict called with value: '%s'\n", str)

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
func validateWholeNumber(value interface{}, config map[string]interface{}) error {
	debugPrintf("DEBUG: validateWholeNumber called with value: %v (type: %T)\n", value, value)

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
func validateURL(value interface{}, config map[string]interface{}) error {
	str, ok := value.(string)
	if !ok || str == "" {
		return nil // Optional field
	}

	debugPrintf("DEBUG: validateURL called with value: '%s'\n", str)

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
func validatePositiveNumber(value interface{}, config map[string]interface{}) error {
	debugPrintf("DEBUG: validatePositiveNumber called with value: %v (type: %T)\n", value, value)

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