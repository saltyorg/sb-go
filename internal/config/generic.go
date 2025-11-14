package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/logging"

	"github.com/go-playground/validator/v10"
)

var verboseMode bool // Package-level variable to store verbosity

// SetVerbose sets the verbose mode for debugging.
func SetVerbose(v bool) {
	verboseMode = v
}

// checkExtraFields recursively checks for extra fields in nested maps AND slices.
func checkExtraFields(inputMap map[string]any, config any) error {
	return checkExtraFieldsInternal(inputMap, config, "")
}

// checkExtraFieldsInternal is a helper function that tracks the context information.
func checkExtraFieldsInternal(inputMap map[string]any, config any, context string) error {
	logging.DebugBool(verboseMode, "\ncheckExtraFields called with inputMap: %+v, config type: %T", inputMap, config)

	configValue := reflect.ValueOf(config).Elem()
	configType := configValue.Type()

	logging.DebugBool(verboseMode, "configType: %v", configType)

	for i := 0; i < configType.NumField(); i++ {
		field := configType.Field(i)
		yamlTag := field.Tag.Get("yaml")
		yamlKey := strings.Split(yamlTag, ",")[0]

		logging.DebugBool(verboseMode, "Checking field: %s (YAML key: %s)", field.Name, yamlKey)

		if value, ok := inputMap[yamlKey]; ok {
			logging.DebugBool(verboseMode, "Found YAML key '%s' in inputMap", yamlKey)

			// Context for nested structures
			currentContext := yamlKey
			if context != "" {
				currentContext = context + "." + yamlKey
			}

			switch v := value.(type) {
			case map[string]any:
				logging.DebugBool(verboseMode, "Field '%s' is a map, recursing...", yamlKey)
				nestedFieldValue := configValue.Field(i)
				if nestedFieldValue.Kind() == reflect.Struct {
					if err := checkExtraFieldsInternal(v, nestedFieldValue.Addr().Interface(), currentContext); err != nil {
						return err
					}
				} else {
					logging.DebugBool(verboseMode, "Field '%s' is a map, but struct field is not a struct. Skipping recursion.", yamlKey)
				}
			case []any:
				logging.DebugBool(verboseMode, "Field '%s' is a slice", yamlKey)
				nestedFieldValue := configValue.Field(i)
				if nestedFieldValue.Kind() == reflect.Slice {
					elementType := nestedFieldValue.Type().Elem()
					logging.DebugBool(verboseMode, "Slice element type: %v", elementType)
					if elementType.Kind() == reflect.Struct {
						for j, sliceElement := range v {
							logging.DebugBool(verboseMode, "Checking slice element %d", j)

							// For array elements, include the index in the context
							elementContext := fmt.Sprintf("%s[%d]", currentContext, j)

							if elementMap, ok := sliceElement.(map[string]any); ok {
								if elementType.Kind() == reflect.Ptr {
									newElement := reflect.New(elementType.Elem()).Interface() // Create a new instance
									if err := checkExtraFieldsInternal(elementMap, newElement, elementContext); err != nil {
										return err
									}
								} else {
									newElement := reflect.New(elementType).Interface()
									if err := checkExtraFieldsInternal(elementMap, newElement, elementContext); err != nil {
										return err
									}
								}
							} else {
								return fmt.Errorf("field '%s' elements must be maps, got: %T", yamlKey, sliceElement)
							}
						}
					} else {
						logging.DebugBool(verboseMode, "Field '%s' is a slice of a basic type. Skipping recursion.", yamlKey)
					}
				} else {
					logging.DebugBool(verboseMode, "Field '%s' is NOT slice. Skipping recursion.", yamlKey)
				}
			default:
				logging.DebugBool(verboseMode, "Field '%s' value: %+v (Type: %T)", yamlKey, value, value)
				if _, ok := value.(string); !ok && field.Type.Kind() != reflect.String {
					fieldType := configValue.Field(i).Type().String()
					return fmt.Errorf("field '%s' must be of type '%s', got: %T", yamlKey, fieldType, value)
				}
			}
		} else {
			logging.DebugBool(verboseMode, "YAML key '%s' NOT found in inputMap", yamlKey)
		}
	}

	for key := range inputMap {
		logging.DebugBool(verboseMode, "Checking for extra field: %s", key)
		found := false
		for i := 0; i < configType.NumField(); i++ {
			field := configType.Field(i)
			yamlTag := field.Tag.Get("yaml")
			yamlKey := strings.Split(yamlTag, ",")[0]
			if yamlKey == key {
				found = true
				break
			}
		}
		if !found {
			// Provide context information about where the unknown field was found
			if context == "" {
				// Root level field
				return fmt.Errorf("unknown field '%s' in configuration (root level)", key)
			} else {
				// Field within a nested structure
				return fmt.Errorf("unknown field '%s' in configuration (inside '%s')", key, context)
			}
		}
	}

	return nil
}

// custom validator for AnsibleBool
func ansibleBoolValidator(fl validator.FieldLevel) bool {
	value := strings.ToLower(fl.Field().String())
	switch value {
	case "yes", "true", "on", "1", "no", "false", "off", "0":
		return true
	default:
		return false
	}
}

// custom validator for timezone or "auto"
func timezoneOrAutoValidator(fl validator.FieldLevel) bool {
	tz := fl.Field().String()
	if strings.ToLower(tz) == "auto" {
		return true
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// custom validator for cron_special_time
func cronSpecialTimeValidator(fl validator.FieldLevel) bool {
	value := strings.ToLower(fl.Field().String())
	switch value {
	case "annually", "daily", "hourly", "monthly", "reboot", "weekly", "yearly":
		return true
	default:
		return false
	}
}

// custom validator for hex color codes
func hexColorValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true // Allow empty values (omitempty will handle required)
	}
	// Match #RGB, #RRGGBB, or #RRGGBBAA formats
	matched, _ := regexp.MatchString(`^#([A-Fa-f0-9]{3}|[A-Fa-f0-9]{6}|[A-Fa-f0-9]{8})$`, value)
	return matched
}

// RegisterCustomValidators registers all the custom validators
func RegisterCustomValidators(validate *validator.Validate) {
	validate.RegisterValidation("ansiblebool", ansibleBoolValidator)
	validate.RegisterValidation("timezone_or_auto", timezoneOrAutoValidator)
	validate.RegisterValidation("cron_special_time", cronSpecialTimeValidator)
	validate.RegisterValidation("hexcolor", hexColorValidator)
}
