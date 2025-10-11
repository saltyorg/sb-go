package validate

import (
	"fmt"
	"os"
	"reflect"
	"regexp"

	"gopkg.in/yaml.v3"
)

// SchemaRule represents a validation rule for a field
type SchemaRule struct {
	Type            string                 `yaml:"type"`
	Required        bool                   `yaml:"required"`
	Format          string                 `yaml:"format"`
	MinLength       int                    `yaml:"min_length"`
	MaxLength       int                    `yaml:"max_length"`
	NotEquals       any                    `yaml:"not_equals"`
	RequiredWith    []string               `yaml:"required_with"`
	CustomValidator string                 `yaml:"custom_validator"`
	Properties      map[string]*SchemaRule `yaml:"properties"`
	Items           *SchemaRule            `yaml:"items"` // For array validation
}

// Schema holds validation rules
type Schema struct {
	Rules map[string]*SchemaRule
}

var verboseMode bool

// SetVerbose sets verbose mode for debugging
func SetVerbose(v bool) {
	verboseMode = v
}

// debugPrintf prints debug messages in verbose mode
func debugPrintf(format string, a ...any) {
	if verboseMode {
		fmt.Printf(format, a...)
	}
}

// LoadSchema loads a YAML schema file
func LoadSchema(schemaPath string) (*Schema, error) {
	debugPrintf("DEBUG: LoadSchema called with path: %s\n", schemaPath)

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	var rules map[string]*SchemaRule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse schema file %s: %w", schemaPath, err)
	}

	debugPrintf("DEBUG: LoadSchema loaded %d top-level rules\n", len(rules))
	return &Schema{Rules: rules}, nil
}

// Validate validates a configuration against the schema
func (s *Schema) Validate(config map[string]any) error {
	debugPrintf("DEBUG: Schema.Validate called with config keys: %v\n", getKeys(config))
	return s.validateObject(config, s.Rules, "")
}

// ValidateStructure performs lightweight structure validation (checks for unknown fields, required fields, but skips type checking)
func (s *Schema) ValidateStructure(config map[string]any) error {
	debugPrintf("DEBUG: Schema.ValidateStructure called with config keys: %v\n", getKeys(config))
	return s.validateObjectStructure(config, s.Rules, "")
}

// ValidateWithTypeFlexibility performs full validation including custom validators but ignores type mismatches
func (s *Schema) ValidateWithTypeFlexibility(config map[string]any) error {
	debugPrintf("DEBUG: Schema.ValidateWithTypeFlexibility called with config keys: %v\n", getKeys(config))
	return s.validateObjectWithTypeFlexibility(config, s.Rules, "", nil)
}

// ValidateWithTypeFlexibilityAsync performs validation with async API checks
func (s *Schema) ValidateWithTypeFlexibilityAsync(config map[string]any) (error, *AsyncValidationContext) {
	debugPrintf("DEBUG: Schema.ValidateWithTypeFlexibilityAsync called with config keys: %v\n", getKeys(config))
	asyncCtx := NewAsyncValidationContext()
	err := s.validateObjectWithTypeFlexibility(config, s.Rules, "", asyncCtx)
	return err, asyncCtx
}

// validateObject validates an object against schema rules
func (s *Schema) validateObject(obj map[string]any, rules map[string]*SchemaRule, path string) error {
	debugPrintf("DEBUG: validateObject called with path: '%s', rules: %v\n", path, getKeys(rules))

	// Check required fields
	for fieldName, rule := range rules {
		fieldPath := appendPath(path, fieldName)
		value, exists := obj[fieldName]

		debugPrintf("DEBUG: Checking field '%s', exists: %t, required: %t\n", fieldPath, exists, rule.Required)

		if rule.Required && !exists {
			return fmt.Errorf("field '%s' is required", fieldPath)
		}

		if !exists {
			continue // Optional field not present
		}

		if err := s.validateField(value, rule, fieldPath, obj); err != nil {
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

// validateObjectStructure validates object structure without strict type checking
func (s *Schema) validateObjectStructure(obj map[string]any, rules map[string]*SchemaRule, path string) error {
	debugPrintf("DEBUG: validateObjectStructure called with path: '%s', rules: %v\n", path, getKeys(rules))

	// Check for unknown fields
	for fieldName := range obj {
		if _, known := rules[fieldName]; !known {
			return fmt.Errorf("unknown field '%s'", appendPath(path, fieldName))
		}
	}

	// Check required fields
	for fieldName, rule := range rules {
		fieldPath := appendPath(path, fieldName)
		value, exists := obj[fieldName]

		if rule.Required && !exists {
			return fmt.Errorf("field '%s' is required", fieldPath)
		}

		if exists && rule.Type == "object" && rule.Properties != nil {
			if objMap, ok := value.(map[string]any); ok {
				if err := s.validateObjectStructure(objMap, rule.Properties, fieldPath); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateObjectWithTypeFlexibility validates an object but skips type checking while running custom validators
func (s *Schema) validateObjectWithTypeFlexibility(obj map[string]any, rules map[string]*SchemaRule, path string, asyncCtx *AsyncValidationContext) error {
	debugPrintf("DEBUG: validateObjectWithTypeFlexibility called with path: '%s', rules: %v\n", path, getKeys(rules))

	// Check required fields
	for fieldName, rule := range rules {
		fieldPath := appendPath(path, fieldName)
		value, exists := obj[fieldName]

		debugPrintf("DEBUG: Checking field '%s', exists: %t, required: %t\n", fieldPath, exists, rule.Required)

		if rule.Required && !exists {
			return fmt.Errorf("field '%s' is required", fieldPath)
		}

		if !exists {
			continue // Optional field not present
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
	debugPrintf("DEBUG: validateFieldWithTypeFlexibility called for '%s' with value type: %T\n", path, value)

	// Not equals validation
	if err := s.validateNotEquals(value, rule, path); err != nil {
		return err
	}

	// Required with validation
	if err := s.validateRequiredWith(value, rule, path, parentConfig); err != nil {
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

	if validatorName, isBuiltIn := builtInValidators[rule.Type]; isBuiltIn {
		debugPrintf("DEBUG: Running built-in %s validator for field '%s'\n", rule.Type, path)
		if validator, exists := customValidators[validatorName]; exists {
			if err := validator(value, parentConfig); err != nil {
				return fmt.Errorf("field '%s': %w", path, err)
			}
		}
	}

	// Custom validator - check if it's an async API validator first
	if rule.CustomValidator != "" {
		debugPrintf("DEBUG: Running custom validator '%s' for field '%s'\n", rule.CustomValidator, path)

		// Check if this is an async API validator
		if asyncValidator, isAsync := asyncAPIValidators[rule.CustomValidator]; isAsync && asyncCtx != nil {
			debugPrintf("DEBUG: Adding async API validator '%s' for field '%s'\n", rule.CustomValidator, path)
			asyncCtx.AddAPIValidation(path, asyncValidator, value, parentConfig)
		} else if validator, exists := customValidators[rule.CustomValidator]; exists {
			// Run synchronous validator
			if err := validator(value, parentConfig); err != nil {
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

// validateField validates a single field value
func (s *Schema) validateField(value any, rule *SchemaRule, path string, parentConfig map[string]any) error {
	debugPrintf("DEBUG: validateField called for '%s' with value type: %T\n", path, value)

	// Basic type validation
	if err := s.validateType(value, rule, path); err != nil {
		return err
	}

	// Format validation
	if err := s.validateFormat(value, rule, path); err != nil {
		return err
	}

	// Length validation
	if err := s.validateLength(value, rule, path); err != nil {
		return err
	}

	// Not equals validation
	if err := s.validateNotEquals(value, rule, path); err != nil {
		return err
	}

	// Required with validation
	if err := s.validateRequiredWith(value, rule, path, parentConfig); err != nil {
		return err
	}

	// Fast path for common built-in validators to reduce map lookups
	switch rule.Type {
	case "ansible_bool":
		if !rule.Required && isEmptyValue(value) {
			debugPrintf("DEBUG: Skipping ansible_bool validator for non-required empty field '%s'\n", path)
		} else {
			debugPrintf("DEBUG: Running built-in ansible_bool validator for field '%s'\n", path)
			if err := validateAnsibleBoolValue(value); err != nil {
				return fmt.Errorf("field '%s': %w", path, err)
			}
		}
	case "subdomain":
		if !rule.Required && isEmptyValue(value) {
			debugPrintf("DEBUG: Skipping subdomain validator for non-required empty field '%s'\n", path)
		} else {
			debugPrintf("DEBUG: Running built-in subdomain validator for field '%s'\n", path)
			if validator, exists := customValidators["validate_subdomain"]; exists {
				if err := validator(value, parentConfig); err != nil {
					return fmt.Errorf("field '%s': %w", path, err)
				}
			}
		}
	case "timezone":
		if !rule.Required && isEmptyValue(value) {
			debugPrintf("DEBUG: Skipping timezone validator for non-required empty field '%s'\n", path)
		} else {
			debugPrintf("DEBUG: Running built-in timezone validator for field '%s'\n", path)
			if validator, exists := customValidators["validate_timezone"]; exists {
				if err := validator(value, parentConfig); err != nil {
					return fmt.Errorf("field '%s': %w", path, err)
				}
			}
		}
	default:
		// Fallback to map lookup for less common validators
		builtInValidators := map[string]string{
			"hostname":        "validate_hostname",
			"directory_path":  "validate_directory_path",
			"url":             "validate_url",
			"cron_time":       "validate_cron_time",
			"rclone_template": "validate_rclone_template",
			"ssh_key_or_url":  "validate_ssh_key_or_url",
			"password":        "validate_password_strength",
		}

		if validatorName, isBuiltIn := builtInValidators[rule.Type]; isBuiltIn {
			if !rule.Required && isEmptyValue(value) {
				debugPrintf("DEBUG: Skipping built-in %s validator for non-required empty field '%s'\n", rule.Type, path)
			} else {
				debugPrintf("DEBUG: Running built-in %s validator for field '%s'\n", rule.Type, path)
				if validator, exists := customValidators[validatorName]; exists {
					if err := validator(value, parentConfig); err != nil {
						return fmt.Errorf("field '%s': %w", path, err)
					}
				}
			}
		}
	}

	// Custom validator
	if rule.CustomValidator != "" {
		debugPrintf("DEBUG: Running custom validator '%s' for field '%s'\n", rule.CustomValidator, path)
		if validator, exists := customValidators[rule.CustomValidator]; exists {
			if err := validator(value, parentConfig); err != nil {
				return fmt.Errorf("field '%s': %w", path, err)
			}
		} else {
			return fmt.Errorf("unknown custom validator '%s' for field '%s'", rule.CustomValidator, path)
		}
	}

	// Nested object validation
	if rule.Type == "object" && rule.Properties != nil {
		if objMap, ok := value.(map[string]any); ok {
			return s.validateObject(objMap, rule.Properties, path)
		}
	}

	// Array validation
	if rule.Type == "array" && rule.Items != nil {
		if arr, ok := value.([]any); ok {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				if err := s.validateField(item, rule.Items, itemPath, parentConfig); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateType validates the basic type of a value
func (s *Schema) validateType(value any, rule *SchemaRule, path string) error {
	if rule.Type == "" {
		return nil // No type constraint
	}

	// Skip type validation if field is not required and value is empty
	if !rule.Required && isEmptyValue(value) {
		debugPrintf("DEBUG: validateType - skipping type check for non-required empty field '%s'\n", path)
		return nil
	}

	valueType := getValueType(value)
	debugPrintf("DEBUG: validateType for '%s': expected=%s, actual=%s, custom_validator=%s\n", path, rule.Type, valueType, rule.CustomValidator)

	// Handle special types that have built-in validation
	if rule.Type == "ansible_bool" {
		// "ansible_bool" type accepts strings and booleans, validation happens automatically
		if valueType == "string" || valueType == "boolean" {
			debugPrintf("DEBUG: validateType - ansible_bool field accepts string/boolean, allowing %s\n", valueType)
			return nil
		}
	}

	// Handle built-in validator types that accept strings
	builtInStringTypes := map[string]bool{
		"subdomain":       true,
		"hostname":        true,
		"directory_path":  true,
		"url":             true,
		"timezone":        true,
		"cron_time":       true,
		"rclone_template": true,
		"ssh_key_or_url":  true,
		"password":        true,
	}

	if builtInStringTypes[rule.Type] {
		// Built-in validator types accept strings, validation happens automatically
		if valueType == "string" {
			debugPrintf("DEBUG: validateType - built-in type '%s' accepts string, allowing %s\n", rule.Type, valueType)
			return nil
		}
	}

	// Handle flexible numeric types
	if rule.Type == "number" {
		// "number" type accepts strings and integers, but NOT floats (for whole numbers with flexibility)
		if valueType == "string" || valueType == "integer" {
			debugPrintf("DEBUG: validateType - number field accepts string/integer, allowing %s\n", valueType)
			return nil
		}
	}

	if rule.Type == "integer" {
		// "integer" type only accepts actual integers (strict)
		if valueType == "integer" {
			debugPrintf("DEBUG: validateType - integer field accepts only integer, allowing %s\n", valueType)
			return nil
		}
	}

	if rule.Type == "float" {
		// "float" type accepts strings and actual floats, but not integers (to be explicit about decimals)
		if valueType == "string" || valueType == "float" {
			debugPrintf("DEBUG: validateType - float field accepts string/float, allowing %s\n", valueType)
			return nil
		}
	}

	if valueType != rule.Type {
		return fmt.Errorf("field '%s' must be of type '%s', got '%s'", path, rule.Type, valueType)
	}

	return nil
}

// validateFormat validates the format of a string value
func (s *Schema) validateFormat(value any, rule *SchemaRule, path string) error {
	if rule.Format == "" {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return nil // Format only applies to strings
	}

	debugPrintf("DEBUG: validateFormat for '%s': format=%s, value=%s\n", path, rule.Format, str)

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
	str, ok := value.(string)
	if !ok {
		return nil // Length only applies to strings
	}

	length := len(str)
	debugPrintf("DEBUG: validateLength for '%s': length=%d, min=%d, max=%d\n", path, length, rule.MinLength, rule.MaxLength)

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

	debugPrintf("DEBUG: validateNotEquals for '%s': value=%v, forbidden=%v\n", path, value, rule.NotEquals)

	if reflect.DeepEqual(value, rule.NotEquals) {
		return fmt.Errorf("field '%s' must not equal the default value: %v", path, rule.NotEquals)
	}

	return nil
}

// validateRequiredWith validates conditional requirements
func (s *Schema) validateRequiredWith(value any, rule *SchemaRule, path string, parentConfig map[string]any) error {
	if len(rule.RequiredWith) == 0 {
		return nil
	}

	debugPrintf("DEBUG: validateRequiredWith for '%s': required_with=%v\n", path, rule.RequiredWith)

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

func getValueType(value any) string {
	if value == nil {
		return "null"
	}

	switch value.(type) {
	case string:
		return "string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "float"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", value)
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
