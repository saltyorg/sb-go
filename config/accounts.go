package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-playground/validator/v10"
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
	debugPrintf("DEBUG: AppriseConfig.UnmarshalYAML called with value: %+v\n", value)
	var s string
	if err := value.Decode(&s); err != nil {
		debugPrintf("DEBUG: AppriseConfig.UnmarshalYAML - error decoding: %v\n", err)
		return err
	}
	*a = AppriseConfig(s)
	debugPrintf("DEBUG: AppriseConfig.UnmarshalYAML - set value to: %s\n", *a)
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
	debugPrintf("DEBUG: customSSHKeyOrURLValidator called with value: '%s'\n", sshKeyOrURL)

	if sshKeyOrURL == "" {
		debugPrintf("DEBUG: customSSHKeyOrURLValidator - value is empty, returning true (omitempty)\n")
		return true // Valid if empty (omitempty)
	}

	// Check if it's a valid URL.
	_, err := url.ParseRequestURI(sshKeyOrURL)
	if err == nil {
		debugPrintf("DEBUG: customSSHKeyOrURLValidator - '%s' is a valid URL, returning true\n", sshKeyOrURL)
		return true // It's a valid URL
	}
	debugPrintf("DEBUG: customSSHKeyOrURLValidator - '%s' is not a valid URL: %v\n", sshKeyOrURL, err)

	// If not a URL, check if it looks like an SSH key (simplified check).
	validKeyTypes := []string{"ssh-rsa", "ssh-dss", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-ed25519"}
	keyParts := strings.Fields(sshKeyOrURL)
	if len(keyParts) < 2 {
		debugPrintf("DEBUG: customSSHKeyOrURLValidator - '%s' has less than 2 parts, not a likely SSH key, returning false\n", sshKeyOrURL)
		return false
	}
	for _, keyType := range validKeyTypes {
		if keyParts[0] == keyType {
			debugPrintf("DEBUG: customSSHKeyOrURLValidator - '%s' starts with valid key type '%s', returning true\n", sshKeyOrURL, keyType)
			return true
		}
	}

	debugPrintf("DEBUG: customSSHKeyOrURLValidator - '%s' is neither a URL nor a recognizable SSH key, returning false\n", sshKeyOrURL)
	return false // Neither a URL nor a recognizable SSH key
}

// ValidateConfig validates the Config struct.
func ValidateConfig(config *Config, inputMap map[string]interface{}) error {
	debugPrintf("\nDEBUG: ValidateConfig called with config: %+v, inputMap: %+v\n", config, inputMap)
	validate := validator.New()

	// Register the custom SSH key/URL validator.
	RegisterCustomValidators(validate) // Moved to generic.go
	err := validate.RegisterValidation("ssh_key_or_url", customSSHKeyOrURLValidator)
	if err != nil {
		return fmt.Errorf("failed to register SSH key/URL validator: %w", err)
	}

	// --- 1. Validate the overall structure and User fields ---
	debugPrintf("DEBUG: ValidateConfig - validating struct: %+v\n", config)
	if err := validate.Struct(config); err != nil {
		debugPrintf("DEBUG: ValidateConfig - struct validation error: %v\n", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "Config.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				debugPrintf("DEBUG: ValidateConfig - validation error on field '%s', tag '%s', value '%v', param '%s'\n",
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
		fmt.Printf("WARNING: field 'user.pass' is shorter than 12 characters (%d). It's recommended to use a stronger password as some automated application setup flows may require it (Portainer skips user setup as an example).\n", len(config.User.Pass))
	}

	// --- Check for extra fields at the TOP LEVEL ---
	debugPrintf("DEBUG: ValidateConfig - checking for extra top-level fields in inputMap: %+v\n", inputMap)
	configType := reflect.TypeOf(Config{})
	for key := range inputMap {
		debugPrintf("DEBUG: ValidateConfig - checking key '%s'\n", key)
		found := false
		for i := 0; i < configType.NumField(); i++ {
			field := configType.Field(i)
			yamlTag := field.Tag.Get("yaml")
			debugPrintf("DEBUG: ValidateConfig - comparing key '%s' with field '%s' (YAML tag: '%s')\n", key, field.Name, yamlTag)
			// Handle inline YAML tags
			if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
				debugPrintf("DEBUG: ValidateConfig - found matching YAML tag for key '%s'\n", key)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown field '%s' in configuration", key)
		}
	}

	// --- 2. Check for extra fields in the "user" section ---
	if userMap, ok := inputMap["user"].(map[string]interface{}); ok {
		debugPrintf("DEBUG: ValidateConfig - found 'user' section in inputMap: %+v\n", userMap)
		userType := reflect.TypeOf(UserConfig{})
		for key := range userMap {
			debugPrintf("DEBUG: ValidateConfig - checking key '%s' in 'user' section\n", key)
			found := false
			for i := 0; i < userType.NumField(); i++ {
				field := userType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				debugPrintf("DEBUG: ValidateConfig - comparing key '%s' with user field '%s' (YAML tag: '%s')\n", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					debugPrintf("DEBUG: ValidateConfig - found matching YAML tag for key '%s' in 'user' section\n", key)
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
		debugPrintf("DEBUG: ValidateConfig - found 'apprise' section in inputMap: %+v\n", appriseVal)
		if _, isString := appriseVal.(string); !isString {
			return fmt.Errorf("field 'apprise' must be a string, got: %T", appriseVal)
		}
	}

	if cfMap, ok := inputMap["cloudflare"].(map[string]interface{}); ok {
		debugPrintf("DEBUG: ValidateConfig - found 'cloudflare' section in inputMap: %+v\n", cfMap)
		cfType := reflect.TypeOf(CloudflareConfig{})
		for key := range cfMap {
			debugPrintf("DEBUG: ValidateConfig - checking key '%s' in 'cloudflare' section\n", key)
			found := false
			for i := 0; i < cfType.NumField(); i++ {
				field := cfType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				debugPrintf("DEBUG: ValidateConfig - comparing key '%s' with cloudflare field '%s' (YAML tag: '%s')\n", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					debugPrintf("DEBUG: ValidateConfig - found matching YAML tag for key '%s' in 'cloudflare' section\n", key)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown field '%s' in 'cloudflare' configuration", key)
			}
		}
	}

	if dhMap, ok := inputMap["dockerhub"].(map[string]interface{}); ok {
		debugPrintf("DEBUG: ValidateConfig - found 'dockerhub' section in inputMap: %+v\n", dhMap)
		dhType := reflect.TypeOf(DockerhubConfig{})
		for key := range dhMap {
			debugPrintf("DEBUG: ValidateConfig - checking key '%s' in 'dockerhub' section\n", key)
			found := false
			for i := 0; i < dhType.NumField(); i++ {
				field := dhType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				debugPrintf("DEBUG: ValidateConfig - comparing key '%s' with dockerhub field '%s' (YAML tag: '%s')\n", key, field.Name, yamlTag)
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
					debugPrintf("DEBUG: ValidateConfig - found matching YAML tag for key '%s' in 'dockerhub' section\n", key)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("unknown field '%s' in 'dockerhub' configuration", key)
			}
		}
	}

	// --- 4. Validate Cloudflare credentials and domain ---
	if config.Cloudflare.API != "" && config.Cloudflare.Email != "" {
		debugPrintf("DEBUG: ValidateConfig - validating Cloudflare credentials and domain\n")
		if err := validateCloudflare(config.Cloudflare.API, config.Cloudflare.Email, config.User.Domain); err != nil {
			return fmt.Errorf("cloudflare validation failed: %w", err)
		}
	} else {
		debugPrintf("DEBUG: ValidateConfig - skipping Cloudflare validation (API or Email not provided)\n")
	}

	// --- 5. Validate Docker Hub credentials ---
	if config.Dockerhub.User != "" && config.Dockerhub.Token != "" {
		debugPrintf("DEBUG: ValidateConfig - validating Docker Hub credentials\n")
		if err := validateDockerHub(config.Dockerhub.User, config.Dockerhub.Token); err != nil {
			return fmt.Errorf("dockerhub validation failed: %w", err)
		}
	} else {
		debugPrintf("DEBUG: ValidateConfig - skipping Docker Hub validation (User or Token not provided)\n")
	}

	debugPrintf("DEBUG: ValidateConfig - validation successful\n")
	return nil
}

// getRootDomain extracts the root domain from a potential FQDN that includes a subdomain.
func getRootDomain(fqdn string) (string, error) {
	debugPrintf("DEBUG: getRootDomain called with fqdn: '%s'\n", fqdn)
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		err := fmt.Errorf("invalid domain format: %s", fqdn)
		debugPrintf("DEBUG: getRootDomain - invalid domain format: %v\n", err)
		return "", err
	}
	// Return the last two parts joined by a dot (e.g., "example.com")
	root := strings.Join(parts[len(parts)-2:], ".")
	debugPrintf("DEBUG: getRootDomain - extracted root domain: '%s'\n", root)
	return root, nil
}

// validateCloudflare checks Cloudflare API credentials and domain ownership.
func validateCloudflare(apiKey, email, domain string) error {
	debugPrintf("DEBUG: validateCloudflare called with apiKey: '%s', email: '%s', domain: '%s'\n", apiKey, email, domain)
	// Create a new Cloudflare API client.
	api, err := cloudflare.New(apiKey, email)
	if err != nil {
		err = fmt.Errorf("failed to create Cloudflare API client: %w", err)
		debugPrintf("DEBUG: validateCloudflare - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateCloudflare - Cloudflare API client created successfully\n")

	// --- Verify API Key ---
	_, err = api.UserDetails(context.Background())
	if err != nil {
		err = fmt.Errorf("cloudflare API key verification failed: %w", err)
		debugPrintf("DEBUG: validateCloudflare - API key verification failed: %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateCloudflare - Cloudflare API key verified successfully\n")

	// --- Verify Domain Ownership ---
	rootDomain, err := getRootDomain(domain) // Use utility function.
	if err != nil {
		debugPrintf("DEBUG: validateCloudflare - error getting root domain: %v\n", err)
		return err // Invalid domain format
	}
	zoneID, err := api.ZoneIDByName(rootDomain)
	if err != nil {
		err = fmt.Errorf("domain verification failed (zone not found): %w", err)
		debugPrintf("DEBUG: validateCloudflare - domain verification failed for '%s': %v\n", rootDomain, err)
		return err
	}
	// Check if zone ID exists (indicating ownership).
	if zoneID == "" {
		err = fmt.Errorf("domain verification failed: %s not found in Cloudflare account", rootDomain)
		debugPrintf("DEBUG: validateCloudflare - %v\n", err)
		return err
	}
	debugPrintf("DEBUG: validateCloudflare - domain '%s' verified successfully (zone ID: '%s')\n", rootDomain, zoneID)
	return nil
}

// validateDockerHub checks Docker Hub credentials using the /v2/users/login endpoint.
func validateDockerHub(username, token string) error {
	debugPrintf("DEBUG: validateDockerHub called with username: '%s', token: '********'\n", username)
	dockerhubLoginUrl := "https://hub.docker.com/v2/users/login/"
	payload := strings.NewReader(fmt.Sprintf(`{"username": "%s", "password": "%s"}`, username, token))
	req, err := http.NewRequest("POST", dockerhubLoginUrl, payload)
	if err != nil {
		err = fmt.Errorf("failed to create request: %w", err)
		debugPrintf("DEBUG: validateDockerHub - %v\n", err)
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to make request: %w", err)
		debugPrintf("DEBUG: validateDockerHub - %v\n", err)
		return err
	}
	defer res.Body.Close()

	debugPrintf("DEBUG: validateDockerHub - received HTTP status: %d\n", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		// Attempt to decode the response body to give a better error message.
		var respBody map[string]interface{}
		if json.NewDecoder(res.Body).Decode(&respBody) == nil { //Decode and check for errors
			debugPrintf("DEBUG: validateDockerHub - decoded response body: %+v\n", respBody)
			if message, ok := respBody["message"].(string); ok { //Check if message exists in the body
				err = fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, message)
				debugPrintf("DEBUG: validateDockerHub - %v\n", err)
				return err
			}
			if details, ok := respBody["details"].(string); ok { //Check if details exists in the body
				err = fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, details)
				debugPrintf("DEBUG: validateDockerHub - %v\n", err)
				return err
			}
		}
		//Default error
		err = fmt.Errorf("docker Hub authentication failed (HTTP %d)", res.StatusCode)
		debugPrintf("DEBUG: validateDockerHub - %v\n", err)
		return err
	}

	debugPrintf("DEBUG: validateDockerHub - authentication successful\n")
	return nil
}
