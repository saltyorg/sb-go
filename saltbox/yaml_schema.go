package saltbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/saltyorg/sb-go/terminal"

	"go.yaml.in/yaml/v3"
)

// SchemaRule represents a validation rule for a field
type SchemaRule struct {
	Type             string                 `yaml:"type"`
	Required         bool                   `yaml:"required"`
	Format           string                 `yaml:"format"`
	MinLength        int                    `yaml:"min_length"`
	MaxLength        int                    `yaml:"max_length"`
	NotEquals        any                    `yaml:"not_equals"`
	RequiredWith     []string               `yaml:"required_with"`
	RequiredWhenTrue []string               `yaml:"required_when_true"`
	ValidateWhenTrue []string               `yaml:"validate_when_true"`
	CustomValidator  string                 `yaml:"custom_validator"`
	Properties       map[string]*SchemaRule `yaml:"properties"`
	Items            *SchemaRule            `yaml:"items"` // For array validation
	// Fields for example config generation
	Description string `yaml:"description"` // Comment to display above the section
	Example     any    `yaml:"example"`     // Example value for this field
}

// Schema holds validation rules
type Schema struct {
	Rules   map[string]*SchemaRule
	verbose bool
}

// LoadSchema loads a YAML schema file
func LoadSchema(schemaPath string, verbose ...bool) (*Schema, error) {
	verboseEnabled := validationVerbose(verbose)
	terminal.DebugBool(verboseEnabled, "LoadSchema called with path: %s", schemaPath)

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	var rules map[string]*SchemaRule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse schema file %s: %w", schemaPath, err)
	}

	terminal.DebugBool(verboseEnabled, "LoadSchema loaded %d top-level rules", len(rules))
	return &Schema{Rules: rules, verbose: verboseEnabled}, nil
}

// ValidateWithTypeFlexibility performs full validation including custom validators but ignores type mismatches
func (s *Schema) ValidateWithTypeFlexibility(config map[string]any) error {
	terminal.DebugBool(s.verbose, "Schema.ValidateWithTypeFlexibility called with config keys: %v", getKeys(config))
	return s.validateObjectWithTypeFlexibility(config, s.Rules, "", nil)
}

// ValidateWithTypeFlexibilityAsync performs validation with async API checks
func (s *Schema) ValidateWithTypeFlexibilityAsync(
	ctx context.Context,
	task *terminal.Task,
	config map[string]any,
) (*AsyncValidationContext, error) {
	terminal.DebugBool(s.verbose, "Schema.ValidateWithTypeFlexibilityAsync called with config keys: %v", getKeys(config))
	asyncCtx := NewAsyncValidationContext(ctx, task, s.verbose)
	err := s.validateObjectWithTypeFlexibility(config, s.Rules, "", asyncCtx)
	return asyncCtx, err
}

// validateObjectWithTypeFlexibility validates an object but skips type checking while running custom validators
func (s *Schema) validateObjectWithTypeFlexibility(obj map[string]any, rules map[string]*SchemaRule, path string, asyncCtx *AsyncValidationContext) error {
	terminal.DebugBool(s.verbose, "validateObjectWithTypeFlexibility called with path: '%s', rules: %v", path, getKeys(rules))

	// Check required fields
	for fieldName, rule := range rules {
		fieldPath := appendPath(path, fieldName)
		value, exists := obj[fieldName]
		isRequired := s.isFieldRequired(rule, obj)

		terminal.DebugBool(s.verbose, "Checking field '%s', exists: %t, required: %t", fieldPath, exists, isRequired)

		if isRequired && !exists {
			return fmt.Errorf("field '%s' is required", fieldPath)
		}

		if !exists {
			continue // Optional field not present
		}

		if !s.shouldValidateField(rule, obj) {
			continue
		}

		if err := s.validateFieldWithTypeFlexibility(value, rule, fieldPath, obj, asyncCtx); err != nil {
			return err
		}
	}

	// Check for unknown fields
	for fieldName := range obj {
		if _, known := rules[fieldName]; !known {
			return fmt.Errorf("unknown field '%s'", appendPath(path, fieldName))
		}
	}

	return nil
}

// validateFieldWithTypeFlexibility validates a field but skips type checking
func (s *Schema) validateFieldWithTypeFlexibility(value any, rule *SchemaRule, path string, parentConfig map[string]any, asyncCtx *AsyncValidationContext) error {
	terminal.DebugBool(s.verbose, "validateFieldWithTypeFlexibility called for '%s' with value type: %T", path, value)

	// Not equals validation
	if err := s.validateNotEquals(value, rule, path); err != nil {
		return err
	}

	// Required with validation
	if err := s.validateRequiredWith(value, rule, path, parentConfig); err != nil {
		return err
	}

	// Skip validation for optional empty values
	if !rule.Required && isEmptyValue(value) {
		return nil
	}

	// Format validation
	if err := s.validateFormat(value, rule, path); err != nil {
		return err
	}

	// Length validation
	if err := s.validateLength(value, rule, path); err != nil {
		return err
	}

	// Built-in type validators (run automatically based on type)
	builtInValidators := map[string]string{
		"ansible_bool":    "validate_ansible_bool",
		"subdomain":       "validate_subdomain",
		"hostname":        "validate_hostname",
		"directory_path":  "validate_directory_path",
		"url":             "validate_url",
		"timezone":        "validate_timezone",
		"cron_time":       "validate_cron_time",
		"rclone_template": "validate_rclone_template",
		"ssh_key_or_url":  "validate_ssh_key_or_url",
		"password":        "validate_password_strength",
	}

	switch rule.Type {
	case "number":
		terminal.DebugBool(s.verbose, "Running built-in number validator for field '%s'", path)
		if err := validateNumberValue(value); err != nil {
			return fmt.Errorf("field '%s': %w", path, err)
		}
	case "float":
		terminal.DebugBool(s.verbose, "Running built-in float validator for field '%s'", path)
		if err := validateFloatValue(value); err != nil {
			return fmt.Errorf("field '%s': %w", path, err)
		}
	}

	if validatorName, isBuiltIn := builtInValidators[rule.Type]; isBuiltIn {
		terminal.DebugBool(s.verbose, "Running built-in %s validator for field '%s'", rule.Type, path)
		if validator, exists := customValidators[validatorName]; exists {
			if err := runCustomValidator(validator, value, parentConfig, asyncCtx, s.verbose); err != nil {
				return fmt.Errorf("field '%s': %w", path, err)
			}
			if validatorName == "validate_password_strength" {
				emitPasswordStrengthWarning(value, asyncCtx)
			}
		}
	}

	// Custom validator - check if it's an async API validator first
	if rule.CustomValidator != "" {
		terminal.DebugBool(s.verbose, "Running custom validator '%s' for field '%s'", rule.CustomValidator, path)

		// Check if this is an async API validator
		if asyncValidator, isAsync := asyncAPIValidators[rule.CustomValidator]; isAsync && asyncCtx != nil {
			terminal.DebugBool(s.verbose, "Adding async API validator '%s' for field '%s'", rule.CustomValidator, path)
			asyncCtx.AddAPIValidation(path, asyncValidator, value, parentConfig)
		} else if validator, exists := customValidators[rule.CustomValidator]; exists {
			// Run synchronous validator
			if err := runCustomValidator(validator, value, parentConfig, asyncCtx, s.verbose); err != nil {
				return fmt.Errorf("field '%s': %w", path, err)
			}
		} else {
			return fmt.Errorf("unknown custom validator '%s' for field '%s'", rule.CustomValidator, path)
		}
	}

	// Nested object validation
	if rule.Type == "object" && rule.Properties != nil {
		if objMap, ok := value.(map[string]any); ok {
			return s.validateObjectWithTypeFlexibility(objMap, rule.Properties, path, asyncCtx)
		}
	}

	// Array validation
	if rule.Type == "array" && rule.Items != nil {
		if arr, ok := value.([]any); ok {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				if err := s.validateFieldWithTypeFlexibility(item, rule.Items, itemPath, parentConfig, asyncCtx); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func runCustomValidator(
	validator CustomValidator,
	value any,
	config map[string]any,
	asyncCtx *AsyncValidationContext,
	verbose bool,
) error {
	err := validator(value, config, verbose)
	var warning *nonFatalValidationWarning
	if !errors.As(err, &warning) {
		return err
	}
	emitValidationWarning(warning.Error(), asyncCtx)
	return nil
}

func emitPasswordStrengthWarning(value any, asyncCtx *AsyncValidationContext) {
	warning := passwordStrengthWarning(value)
	if warning == "" {
		return
	}
	emitValidationWarning(warning, asyncCtx)
}

func emitValidationWarning(warning string, asyncCtx *AsyncValidationContext) {
	if asyncCtx != nil && asyncCtx.task != nil {
		asyncCtx.task.Warning(warning)
		return
	}
	fmt.Fprintln(os.Stderr, warning)
}

// validateFormat validates the format of a string value
func (s *Schema) validateFormat(value any, rule *SchemaRule, path string) error {
	if rule.Format == "" {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("field '%s' must be a string", path)
	}

	// Validation diagnostics are commonly retained in terminal transcripts and
	// support logs. Never include field values here because schemas can gain
	// credential fields without this validator changing.
	terminal.DebugBool(s.verbose, "validateFormat for '%s': format=%s", path, rule.Format)

	switch rule.Format {
	case "email":
		if !isValidEmail(str) {
			return fmt.Errorf("field '%s' must be a valid email address", path)
		}
	case "hostname":
		if !isValidHostname(str) {
			return fmt.Errorf("field '%s' must be a valid hostname", path)
		}
	case "url":
		if !isValidURL(str) {
			return fmt.Errorf("field '%s' must be a valid URL", path)
		}
	default:
		return fmt.Errorf("unknown format '%s' for field '%s'", rule.Format, path)
	}

	return nil
}

// validateLength validates string length constraints
func (s *Schema) validateLength(value any, rule *SchemaRule, path string) error {
	if rule.MinLength == 0 && rule.MaxLength == 0 {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("field '%s' must be a string", path)
	}

	length := len(str)
	terminal.DebugBool(s.verbose, "validateLength for '%s': length=%d, min=%d, max=%d", path, length, rule.MinLength, rule.MaxLength)

	if rule.MinLength > 0 && length < rule.MinLength {
		return fmt.Errorf("field '%s' must be at least %d characters long, got %d", path, rule.MinLength, length)
	}

	if rule.MaxLength > 0 && length > rule.MaxLength {
		return fmt.Errorf("field '%s' must be at most %d characters long, got %d", path, rule.MaxLength, length)
	}

	return nil
}

// validateNotEquals validates that value doesn't equal a forbidden value
func (s *Schema) validateNotEquals(value any, rule *SchemaRule, path string) error {
	if rule.NotEquals == nil {
		return nil
	}

	terminal.DebugBool(s.verbose, "validateNotEquals for '%s': matches forbidden value=%t", path, reflect.DeepEqual(value, rule.NotEquals))

	if reflect.DeepEqual(value, rule.NotEquals) {
		return fmt.Errorf("field '%s' must not equal its configured default value", path)
	}

	return nil
}

// validateRequiredWith validates conditional requirements
func (s *Schema) validateRequiredWith(value any, rule *SchemaRule, path string, parentConfig map[string]any) error {
	if len(rule.RequiredWith) == 0 {
		return nil
	}

	terminal.DebugBool(s.verbose, "validateRequiredWith for '%s': required_with=%v", path, rule.RequiredWith)

	// Check if any of the required_with fields are present with meaningful values (not null/empty)
	hasRequiredField := false
	for _, requiredField := range rule.RequiredWith {
		if fieldValue, exists := parentConfig[requiredField]; exists && !isEmptyValue(fieldValue) {
			hasRequiredField = true
			break
		}
	}

	if hasRequiredField {
		// If any required_with field is present, this field must also be present
		if value == nil || (reflect.ValueOf(value).Kind() == reflect.String && value.(string) == "") {
			return fmt.Errorf("field '%s' is required when any of %v are present", path, rule.RequiredWith)
		}
	}

	return nil
}

func (s *Schema) isFieldRequired(rule *SchemaRule, parentConfig map[string]any) bool {
	if rule.Required {
		return true
	}

	if len(rule.RequiredWhenTrue) == 0 {
		return false
	}

	for _, fieldName := range rule.RequiredWhenTrue {
		rawValue, exists := parentConfig[fieldName]
		if !exists {
			continue
		}

		boolValue, validBool := parseAnsibleBool(rawValue)
		if validBool && boolValue {
			return true
		}
	}

	return false
}

func (s *Schema) shouldValidateField(rule *SchemaRule, parentConfig map[string]any) bool {
	if len(rule.ValidateWhenTrue) == 0 {
		return true
	}

	for _, fieldName := range rule.ValidateWhenTrue {
		rawValue, exists := parentConfig[fieldName]
		if !exists {
			continue
		}

		boolValue, validBool := parseAnsibleBool(rawValue)
		if validBool && boolValue {
			return true
		}
	}

	return false
}

func parseAnsibleBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "yes", "true", "on", "1":
			return true, true
		case "no", "false", "off", "0":
			return false, true
		}
	}

	return false, false
}

// Helper functions

func appendPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	return basePath + "." + fieldName
}

func getKeys(m any) []string {
	switch v := m.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		return keys
	case map[string]*SchemaRule:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		return keys
	default:
		return []string{}
	}
}

// Format validation helper functions

func isValidEmail(email string) bool {
	// Simple email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func isValidHostname(hostname string) bool {
	// Simple hostname validation
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	return hostnameRegex.MatchString(hostname)
}

func isValidURL(url string) bool {
	// Simple URL validation
	urlRegex := regexp.MustCompile(`^https?://[^\s]+$`)
	return urlRegex.MatchString(url)
}

// isEmptyValue checks if a value is considered empty (nil, empty string, etc.)
func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}

	if str, ok := value.(string); ok {
		return str == ""
	}

	return false
}

func validateNumberValue(value any) error {
	switch v := value.(type) {
	case string:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("must be a whole number (integer), got: %s", v)
		}
		return nil
	case int, int8, int16, int32, int64:
		return nil
	case uint, uint8, uint16, uint32, uint64:
		return nil
	case float32, float64:
		return fmt.Errorf("must be a whole number (integer), got: %v", v)
	default:
		return fmt.Errorf("must be a whole number (integer), got: %T", value)
	}
}

func validateFloatValue(value any) error {
	switch v := value.(type) {
	case string:
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return fmt.Errorf("must be a float, got: %s", v)
		}
		return nil
	case float32, float64:
		return nil
	case int, int8, int16, int32, int64:
		return fmt.Errorf("must be a float, got: %v", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Errorf("must be a float, got: %v", v)
	default:
		return fmt.Errorf("must be a float, got: %T", value)
	}
}
