package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/utils"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zones"
	"github.com/go-playground/validator/v10"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v3"
)

// Config represents the overall configuration structure.
type Config struct {
	Apprise    AppriseConfig    `yaml:"apprise" validate:"omitempty"`
	Cloudflare CloudflareConfig `yaml:"cloudflare"`
	Dockerhub  DockerhubConfig  `yaml:"dockerhub"`
	User       UserConfig       `yaml:"user" validate:"required"`
}

// AppriseConfig holds Apprise-related settings.
type AppriseConfig string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *AppriseConfig) UnmarshalYAML(value *yaml.Node) error {
	logging.DebugBool(verboseMode, "AppriseConfig.UnmarshalYAML called with value: %+v", value)

	// Handle nil or empty values explicitly
	if value == nil || value.Kind == yaml.ScalarNode && value.Value == "" {
		logging.DebugBool(verboseMode, "AppriseConfig.UnmarshalYAML - nil or empty value detected, setting to empty string")
		*a = ""
		return nil
	}

	// Handle properly formatted string values
	var s string
	if err := value.Decode(&s); err != nil {
		logging.DebugBool(verboseMode, "AppriseConfig.UnmarshalYAML - error decoding: %v", err)
		return err
	}
	*a = AppriseConfig(s)
	logging.DebugBool(verboseMode, "AppriseConfig.UnmarshalYAML - set value to: %s", *a)
	return nil
}

// CloudflareConfig holds Cloudflare configuration.
type CloudflareConfig struct {
	API   string `yaml:"api" validate:"required_with=Email,omitempty"`
	Email string `yaml:"email" validate:"required_with=API,omitempty,email"`
}

// DockerhubConfig holds Docker Hub configuration.
type DockerhubConfig struct {
	Token string `yaml:"token" validate:"required_with=User,omitempty"`
	User  string `yaml:"user" validate:"required_with=Token,omitempty"`
}

// UserConfig holds user-specific settings.
type UserConfig struct {
	Domain string `yaml:"domain" validate:"required,fqdn,ne=testsaltbox.ml"`
	Email  string `yaml:"email" validate:"required,email,ne=your@email.com"`
	Name   string `yaml:"name" validate:"required,min=1"`
	Pass   string `yaml:"pass" validate:"required,min=1,ne=password1234"`
	SSHKey string `yaml:"ssh_key" validate:"omitempty,ssh_key_or_url"` // Custom validator
}

// customSSHKeyOrURLValidator is a custom validator function for SSH keys or URLs.
func customSSHKeyOrURLValidator(fl validator.FieldLevel) bool {
	sshKeyOrURL := fl.Field().String()
	logging.DebugBool(verboseMode, "customSSHKeyOrURLValidator called with value: '%s'", sshKeyOrURL)

	if sshKeyOrURL == "" {
		logging.DebugBool(verboseMode, "customSSHKeyOrURLValidator - value is empty, returning true (omitempty)")
		return true // Valid if empty (omitempty)
	}

	if utils.IsValidAuthorizedKeyOrURL(sshKeyOrURL) {
		logging.DebugBool(verboseMode, "customSSHKeyOrURLValidator - '%s' is a valid SSH key or URL, returning true", sshKeyOrURL)
		return true
	}

	logging.DebugBool(verboseMode, "customSSHKeyOrURLValidator - '%s' is neither a valid SSH key nor a supported URL, returning false", sshKeyOrURL)
	return false
}

// ValidateConfig validates the Config struct.
func ValidateConfig(config *Config, inputMap map[string]any) error {
	logging.DebugBool(verboseMode, "\nDEBUG: ValidateConfig called with config: %+v, inputMap: %+v", config, inputMap)
	validate := validator.New()

	// Register the custom SSH key/URL validator.
	if err := RegisterCustomValidators(validate); err != nil {
		return err
	}

	err := validate.RegisterValidation("ssh_key_or_url", customSSHKeyOrURLValidator)
	if err != nil {
		return fmt.Errorf("failed to register SSH key/URL validator: %w", err)
	}

	// --- 1. Validate the overall structure and User fields ---
	logging.DebugBool(verboseMode, "ValidateConfig - validating struct: %+v", config)
	if err := validate.Struct(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateConfig - struct validation error: %v", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "Config.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				logging.DebugBool(verboseMode, "ValidateConfig - validation error on field '%s', tag '%s', value '%v', param '%s'",
					fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					return fmt.Errorf("field '%s' is required", fieldPath)
				case "email":
					return fmt.Errorf("field '%s' must be a valid email address, got: %s", fieldPath, e.Value())
				case "fqdn":
					return fmt.Errorf("field '%s' must be a fully qualified domain name, got: %s", fieldPath, e.Value())
				case "min":
					return fmt.Errorf("field '%s' must be at least %s characters long, got: %s", fieldPath, e.Param(), e.Value())
				case "ssh_key_or_url":
					return fmt.Errorf("field '%s' must be a valid SSH public key or URL, got: %s", fieldPath, e.Value())
				case "ne":
					return fmt.Errorf("field '%s' must not be equal to the default value: %s", fieldPath, e.Value())
				case "string":
					return fmt.Errorf("field '%s' must be a string, got: %v", fieldPath, e.Value())
				case "required_without_all":
					return fmt.Errorf("either '%s' or its related fields must be provided", fieldPath)
				default:
					return fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
				}
			}
		}
		return err
	}

	// --- Password strength warning (non-fatal) ---
	if len(config.User.Pass) > 0 && len(config.User.Pass) < 12 {
		fmt.Printf("WARNING: field 'user.pass' is shorter than 12 characters (%d). It's recommended to use a stronger password as some automated application setup flows may require it (Portainer skips user setup as an example).", len(config.User.Pass))
	}

	// --- Check for extra fields at the TOP LEVEL ---
	logging.DebugBool(verboseMode, "ValidateConfig - checking for extra top-level fields in inputMap: %+v", inputMap)
	configType := reflect.TypeFor[Config]()
	for key := range inputMap {
		logging.DebugBool(verboseMode, "ValidateConfig - checking key '%s'", key)
		found := false
		for field := range configType.Fields() {
			yamlTag := field.Tag.Get("yaml")
			logging.DebugBool(verboseMode, "ValidateConfig - comparing key '%s' with field '%s' (YAML tag: '%s')", key, field.Name, yamlTag)
			// Handle inline YAML tags
			if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
				logging.DebugBool(verboseMode, "ValidateConfig - found matching YAML tag for key '%s'", key)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown field '%s' in configuration", key)
		}
	}

	// --- 2. Check for extra fields in the "user" section ---
	if userMap, ok := inputMap["user"].(map[string]any); ok {
		logging.DebugBool(verboseMode, "ValidateConfig - found 'user' section in inputMap: %+v", userMap)
		userType := reflect.TypeFor[UserConfig]()
		for key := range userMap {
			logging.DebugBool(verboseMode, "ValidateConfig - checking key '%s' in 'user' section", key)
			found := false
			for field := range userType.Fields() {
				yamlTag := field.Tag.Get("yaml")
				logging.DebugBool(verboseMode, "ValidateConfig - comparing key '%s' with user field '%s' (YAML tag: '%s')", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					logging.DebugBool(verboseMode, "ValidateConfig - found matching YAML tag for key '%s' in 'user' section", key)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown field '%s' in 'user' configuration", key)
			}
		}
	} else if inputMap["user"] != nil {
		return fmt.Errorf("'user' configuration must be a map")
	}

	// --- 3. Check for extra fields in other sections ---
	if appriseVal, ok := inputMap["apprise"]; ok {
		logging.DebugBool(verboseMode, "ValidateConfig - found 'apprise' section in inputMap: %+v", appriseVal)
		// Allow empty string or nil for apprise
		if appriseVal != nil && appriseVal != "" {
			if _, isString := appriseVal.(string); !isString {
				return fmt.Errorf("field 'apprise' must be a string, got: %T", appriseVal)
			}
		}
		// If we get here, the apprise key exists but could be empty or nil, which is fine
	}

	if cfMap, ok := inputMap["cloudflare"].(map[string]any); ok {
		logging.DebugBool(verboseMode, "ValidateConfig - found 'cloudflare' section in inputMap: %+v", cfMap)
		cfType := reflect.TypeFor[CloudflareConfig]()
		for key := range cfMap {
			logging.DebugBool(verboseMode, "ValidateConfig - checking key '%s' in 'cloudflare' section", key)
			found := false
			for field := range cfType.Fields() {
				yamlTag := field.Tag.Get("yaml")
				logging.DebugBool(verboseMode, "ValidateConfig - comparing key '%s' with cloudflare field '%s' (YAML tag: '%s')", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					logging.DebugBool(verboseMode, "ValidateConfig - found matching YAML tag for key '%s' in 'cloudflare' section", key)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown field '%s' in 'cloudflare' configuration", key)
			}
		}
	}

	if dhMap, ok := inputMap["dockerhub"].(map[string]any); ok {
		logging.DebugBool(verboseMode, "ValidateConfig - found 'dockerhub' section in inputMap: %+v", dhMap)
		dhType := reflect.TypeFor[DockerhubConfig]()
		for key := range dhMap {
			logging.DebugBool(verboseMode, "ValidateConfig - checking key '%s' in 'dockerhub' section", key)
			found := false
			for field := range dhType.Fields() {
				yamlTag := field.Tag.Get("yaml")
				logging.DebugBool(verboseMode, "ValidateConfig - comparing key '%s' with dockerhub field '%s' (YAML tag: '%s')", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					logging.DebugBool(verboseMode, "ValidateConfig - found matching YAML tag for key '%s' in 'dockerhub' section", key)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown field '%s' in 'dockerhub' configuration", key)
			}
		}
	}

	// --- 4. Validate Cloudflare credentials, domain, and SSL/TLS settings ---
	if config.Cloudflare.API != "" && config.Cloudflare.Email != "" {
		logging.DebugBool(verboseMode, "ValidateConfig - validating Cloudflare credentials, domain, and SSL/TLS settings")
		if err := validateCloudflare(config.Cloudflare.API, config.Cloudflare.Email, config.User.Domain); err != nil {
			return fmt.Errorf("cloudflare validation failed: %w", err)
		}
	} else {
		logging.DebugBool(verboseMode, "ValidateConfig - skipping Cloudflare validation (API or Email not provided)")
	}

	// --- 5. Validate Docker Hub credentials ---
	if config.Dockerhub.User != "" && config.Dockerhub.Token != "" {
		logging.DebugBool(verboseMode, "ValidateConfig - validating Docker Hub credentials")
		if err := validateDockerHub(config.Dockerhub.User, config.Dockerhub.Token); err != nil {
			return fmt.Errorf("dockerhub validation failed: %w", err)
		}
	} else {
		logging.DebugBool(verboseMode, "ValidateConfig - skipping Docker Hub validation (User or Token not provided)")
	}

	logging.DebugBool(verboseMode, "ValidateConfig - validation successful")
	return nil
}

// getRootDomain extracts the root domain from a potential FQDN that includes a subdomain.
func getRootDomain(fqdn string) (string, error) {
	logging.DebugBool(verboseMode, "getRootDomain called with fqdn: '%s'", fqdn)

	// Validate the domain format first
	if fqdn == "" {
		err := fmt.Errorf("empty domain name")
		logging.DebugBool(verboseMode, "getRootDomain - %v", err)
		return "", err
	}

	// Get the effective TLD plus one level using the public suffix list
	domain, err := publicsuffix.EffectiveTLDPlusOne(fqdn)
	if err != nil {
		err = fmt.Errorf("invalid domain format: %s: %w", fqdn, err)
		logging.DebugBool(verboseMode, "getRootDomain - invalid domain format: %v", err)
		return "", err
	}

	logging.DebugBool(verboseMode, "getRootDomain - extracted root domain: '%s'", domain)
	return domain, nil
}

// validateCloudflare checks Cloudflare API credentials, domain ownership, and SSL/TLS settings.
func validateCloudflare(apiKey, email, domain string) error {
	logging.DebugBool(verboseMode, "validateCloudflare called with apiKey: '%s', email: '%s', domain: '%s'", apiKey, email, domain)
	// Create a new Cloudflare API client.
	api := cloudflare.NewClient(
		option.WithAPIKey(apiKey),
		option.WithAPIEmail(email),
	)
	logging.DebugBool(verboseMode, "validateCloudflare - Cloudflare API client created successfully")

	// --- Verify API Key ---
	_, err := api.User.Get(context.Background())
	if err != nil {
		err = fmt.Errorf("cloudflare API key verification failed: %w", err)
		logging.DebugBool(verboseMode, "validateCloudflare - API key verification failed: %v", err)
		return err
	}
	logging.DebugBool(verboseMode, "validateCloudflare - Cloudflare API key verified successfully")

	// --- Verify Domain Ownership ---
	rootDomain, err := getRootDomain(domain) // Use utility function.
	if err != nil {
		logging.DebugBool(verboseMode, "validateCloudflare - error getting root domain: %v", err)
		return err // Invalid domain format
	}
	zonesList, err := api.Zones.List(context.Background(), zones.ZoneListParams{
		Name: cloudflare.F(rootDomain),
	})
	if err != nil {
		err = fmt.Errorf("domain verification failed (zone not found): %w", err)
		logging.DebugBool(verboseMode, "validateCloudflare - domain verification failed for '%s': %v", rootDomain, err)
		return err
	}
	// Check if zone exists (indicating ownership).
	if len(zonesList.Result) == 0 {
		err = fmt.Errorf("domain verification failed: %s not found in Cloudflare account", rootDomain)
		logging.DebugBool(verboseMode, "validateCloudflare - %v", err)
		return err
	}
	zoneID := zonesList.Result[0].ID
	logging.DebugBool(verboseMode, "validateCloudflare - domain '%s' verified successfully (zone ID: '%s')", rootDomain, zoneID)

	// --- Verify SSL/TLS Settings ---
	// Get the current SSL/TLS settings for the zone
	ctx := context.Background()
	sslSettings, err := api.Zones.Settings.Get(ctx, "ssl", zones.SettingGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		err = fmt.Errorf("failed to get zone SSL settings: %w", err)
		logging.DebugBool(verboseMode, "validateCloudflare - failed to get zone SSL settings: %v", err)
		return err
	}

	// Validate SSL mode using the typed value - reject "flexible" and "off" modes
	if sslSettings != nil && sslSettings.Value != nil {
		// Type assert to the SSL value type
		if sslValue, ok := sslSettings.Value.(zones.SettingGetResponseZonesSchemasSSLValue); ok {
			logging.DebugBool(verboseMode, "validateCloudflare - SSL mode for domain '%s': '%s'", rootDomain, string(sslValue))

			// Check for incompatible SSL modes using the typed constants
			if sslValue == zones.SettingGetResponseZonesSchemasSSLValueFlexible ||
				sslValue == zones.SettingGetResponseZonesSchemasSSLValueOff {
				err = fmt.Errorf("incompatible SSL/TLS mode detected: '%s'\n"+
					"  This SSL/TLS mode is not compatible with Saltbox."+
					"  With '%s' mode, connections will fail or behave unexpectedly.\n"+
					"  Please update your Cloudflare settings by:"+
					"  1. Log in to your Cloudflare dashboard"+
					"  2. Go to the SSL/TLS section for domain '%s'"+
					"  3. Change the encryption mode to 'Full' or 'Full (strict)'"+
					"  4. Save your changes",
					string(sslValue), string(sslValue), rootDomain)
				logging.DebugBool(verboseMode, "validateCloudflare - %v", err)
				return err
			}

			logging.DebugBool(verboseMode, "validateCloudflare - SSL mode '%s' is secure", string(sslValue))
		} else {
			// Fallback: try to get as string if type assertion fails
			logging.DebugBool(verboseMode, "validateCloudflare - SSL value type assertion failed, trying string conversion")
			sslModeStr := fmt.Sprintf("%v", sslSettings.Value)
			logging.DebugBool(verboseMode, "validateCloudflare - SSL mode for domain '%s': '%s'", rootDomain, sslModeStr)

			if sslModeStr == "flexible" || sslModeStr == "off" {
				err = fmt.Errorf("incompatible SSL/TLS mode detected: '%s'\n"+
					"  This SSL/TLS mode is not compatible with Saltbox."+
					"  With '%s' mode, connections will fail or behave unexpectedly.\n"+
					"  Please update your Cloudflare settings by:"+
					"  1. Log in to your Cloudflare dashboard"+
					"  2. Go to the SSL/TLS section for domain '%s'"+
					"  3. Change the encryption mode to 'Full' or 'Full (strict)'"+
					"  4. Save your changes",
					sslModeStr, sslModeStr, rootDomain)
				logging.DebugBool(verboseMode, "validateCloudflare - %v", err)
				return err
			}

			logging.DebugBool(verboseMode, "validateCloudflare - SSL mode '%s' is secure", sslModeStr)
		}
	} else {
		// If we can't get SSL settings, return an error
		err = fmt.Errorf("failed to determine SSL/TLS mode for domain '%s'\n"+
			"  Please verify your Cloudflare settings:"+
			"  1. Log in to your Cloudflare dashboard"+
			"  2. Navigate to the SSL/TLS section"+
			"  3. Confirm encryption mode is set to 'Full' or 'Full (strict)'"+
			"  4. Flexible or Off modes are incompatible with Saltbox", rootDomain)
		logging.DebugBool(verboseMode, "validateCloudflare - %v", err)
		return err
	}

	return nil
}

// validateDockerHub checks Docker Hub credentials using the /v2/users/login endpoint.
func validateDockerHub(username, token string) error {
	logging.DebugBool(verboseMode, "validateDockerHub called with username: '%s', token: '********'", username)
	dockerhubLoginUrl := "https://hub.docker.com/v2/users/login/"
	payload := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, username, token))
	req, err := http.NewRequest("POST", dockerhubLoginUrl, payload)
	if err != nil {
		err = fmt.Errorf("failed to create request: %w", err)
		logging.DebugBool(verboseMode, "validateDockerHub - %v", err)
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to make request: %w", err)
		logging.DebugBool(verboseMode, "validateDockerHub - %v", err)
		return err
	}
	defer func() { _ = res.Body.Close() }()

	logging.DebugBool(verboseMode, "validateDockerHub - received HTTP status: %d", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		// Attempt to decode the response body to give a better error message.
		var respBody map[string]any
		if json.NewDecoder(res.Body).Decode(&respBody) == nil { //Decode and check for errors
			logging.DebugBool(verboseMode, "validateDockerHub - decoded response body: %+v", respBody)
			if message, ok := respBody["message"].(string); ok { //Check if a message exists in the body
				err = fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, message)
				logging.DebugBool(verboseMode, "validateDockerHub - %v", err)
				return err
			}
			if details, ok := respBody["details"].(string); ok { //Check if details exist in the body
				err = fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, details)
				logging.DebugBool(verboseMode, "validateDockerHub - %v", err)
				return err
			}
		}
		//Default error
		err = fmt.Errorf("docker Hub authentication failed (HTTP %d)", res.StatusCode)
		logging.DebugBool(verboseMode, "validateDockerHub - %v", err)
		return err
	}

	logging.DebugBool(verboseMode, "validateDockerHub - authentication successful")
	return nil
}
