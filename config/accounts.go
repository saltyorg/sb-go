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
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	*a = AppriseConfig(s)
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
	Pass   string `yaml:"pass" validate:"required,min=12,ne=password1234"`
	SSHKey string `yaml:"ssh_key" validate:"omitempty,ssh_key_or_url"` // Custom validator
}

// customSSHKeyOrURLValidator is a custom validator function for SSH keys or URLs.
func customSSHKeyOrURLValidator(fl validator.FieldLevel) bool {
	sshKeyOrURL := fl.Field().String()

	if sshKeyOrURL == "" {
		return true // Valid if empty (omitempty)
	}

	// Check if it's a valid URL.
	_, err := url.ParseRequestURI(sshKeyOrURL)
	if err == nil {
		return true // It's a valid URL
	}

	// If not a URL, check if it looks like an SSH key (simplified check).
	validKeyTypes := []string{"ssh-rsa", "ssh-dss", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-ed25519"}
	keyParts := strings.Fields(sshKeyOrURL)
	if len(keyParts) < 2 {
		return false
	}
	for _, keyType := range validKeyTypes {
		if keyParts[0] == keyType {
			return true
		}
	}

	return false // Neither a URL nor a recognizable SSH key
}

// ValidateConfig validates the Config struct.
func ValidateConfig(config *Config, inputMap map[string]interface{}) error {
	validate := validator.New()

	// Register the custom SSH key/URL validator.
	RegisterCustomValidators(validate) // Moved to generic.go
	err := validate.RegisterValidation("ssh_key_or_url", customSSHKeyOrURLValidator)
	if err != nil {
		return fmt.Errorf("failed to register SSH key/URL validator: %w", err)
	}

	// --- 1. Validate the overall structure and User fields ---
	if err := validate.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				lowercaseField := strings.ToLower(e.Field())
				switch e.Tag() {
				case "required":
					return fmt.Errorf("field '%s' is required", lowercaseField)
				case "email":
					return fmt.Errorf("field '%s' must be a valid email address, got: %s", lowercaseField, e.Value())
				case "fqdn":
					return fmt.Errorf("field '%s' must be a fully qualified domain name, got: %s", lowercaseField, e.Value())
				case "min":
					return fmt.Errorf("field '%s' must be at least %s characters long, got: %s", lowercaseField, e.Param(), e.Value())
				case "ssh_key_or_url":
					return fmt.Errorf("field '%s' must be a valid SSH public key or URL, got: %s", lowercaseField, e.Value())
				case "ne":
					return fmt.Errorf("field '%s' must not be equal to the default value: %s", lowercaseField, e.Value())
				case "string":
					return fmt.Errorf("field '%s' must be a string, got: %v", lowercaseField, e.Value())
				case "required_without_all":
					return fmt.Errorf("either '%s' or its related fields must be provided", lowercaseField)
				default:
					return fmt.Errorf("field '%s' is invalid: %s", lowercaseField, e.Error())
				}
			}
		}
		return err
	}
	// --- Check for extra fields at the TOP LEVEL ---
	configType := reflect.TypeOf(Config{})
	for key := range inputMap {
		found := false
		for i := 0; i < configType.NumField(); i++ {
			field := configType.Field(i)
			yamlTag := field.Tag.Get("yaml")
			// Handle inline YAML tags
			if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
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
		userType := reflect.TypeOf(UserConfig{})
		for key := range userMap {
			found := false
			for i := 0; i < userType.NumField(); i++ {
				field := userType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
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
		if _, isString := appriseVal.(string); !isString {
			return fmt.Errorf("field 'apprise' must be a string, got: %T", appriseVal)
		}
	}

	if cfMap, ok := inputMap["cloudflare"].(map[string]interface{}); ok {
		cfType := reflect.TypeOf(CloudflareConfig{})
		for key := range cfMap {
			found := false
			for i := 0; i < cfType.NumField(); i++ {
				field := cfType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
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
		dhType := reflect.TypeOf(DockerhubConfig{})
		for key := range dhMap {
			found := false
			for i := 0; i < dhType.NumField(); i++ {
				field := dhType.Field(i)
				yamlTag := field.Tag.Get("yaml")
				if yamlTag == key || (strings.Contains(yamlTag, ",") && strings.Split(yamlTag, ",")[0] == key) {
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
		if err := validateCloudflare(config.Cloudflare.API, config.Cloudflare.Email, config.User.Domain); err != nil {
			return fmt.Errorf("cloudflare validation failed: %w", err)
		}
	}

	// --- 5. Validate Docker Hub credentials ---
	if config.Dockerhub.User != "" && config.Dockerhub.Token != "" {
		if err := validateDockerHub(config.Dockerhub.User, config.Dockerhub.Token); err != nil {
			return fmt.Errorf("dockerhub validation failed: %w", err)
		}
	}

	return nil
}

// getRootDomain extracts the root domain from a potential FQDN that includes a subdomain.
func getRootDomain(fqdn string) (string, error) {
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid domain format: %s", fqdn)
	}
	// Return the last two parts joined by a dot (e.g., "example.com")
	return strings.Join(parts[len(parts)-2:], "."), nil
}

// validateCloudflare checks Cloudflare API credentials and domain ownership.
func validateCloudflare(apiKey, email, domain string) error {
	// Create a new Cloudflare API client.
	api, err := cloudflare.New(apiKey, email)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare API client: %w", err)
	}

	// --- Verify API Key ---
	_, err = api.UserDetails(context.Background())
	if err != nil {
		return fmt.Errorf("cloudflare API key verification failed: %w", err)
	}

	// --- Verify Domain Ownership ---
	rootDomain, err := getRootDomain(domain) // Use utility function.
	if err != nil {
		return err // Invalid domain format
	}
	zoneID, err := api.ZoneIDByName(rootDomain)
	if err != nil {
		return fmt.Errorf("domain verification failed (zone not found): %w", err)
	}
	// Check if zone ID exists (indicating ownership).
	if zoneID == "" {
		return fmt.Errorf("domain verification failed: %s not found in Cloudflare account", rootDomain)
	}
	return nil
}

// validateDockerHub checks Docker Hub credentials using the /v2/users/login endpoint.
func validateDockerHub(username, token string) error {
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
		// Attempt to decode the response body to give a better error message.
		var respBody map[string]interface{}
		if json.NewDecoder(res.Body).Decode(&respBody) == nil { //Decode and check for errors
			if message, ok := respBody["message"].(string); ok { //Check if message exists in the body
				return fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, message)
			}
			if details, ok := respBody["details"].(string); ok { //Check if details exists in the body
				return fmt.Errorf("docker hub authentication failed (HTTP %d): %s", res.StatusCode, details)
			}
		}
		//Default error
		return fmt.Errorf("docker Hub authentication failed (HTTP %d)", res.StatusCode)
	}

	return nil
}
