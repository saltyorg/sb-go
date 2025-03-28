package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

var verboseMode bool // Add a package-level variable to store verbosity

// SetVerbose sets the verbose mode for debugging.
func SetVerbose(v bool) {
	verboseMode = v
}

// debugPrintf prints a debug message if verbose mode is enabled.
func debugPrintf(format string, a ...interface{}) {
	if verboseMode {
		fmt.Printf(format, a...)
	}
}

// checkExtraFields recursively checks for extra fields in nested maps AND slices.
func checkExtraFields(inputMap map[string]interface{}, config interface{}) error {
	debugPrintf("\nDEBUG: checkExtraFields called with inputMap: %+v, config type: %T\n", inputMap, config)

	configValue := reflect.ValueOf(config).Elem()
	configType := configValue.Type()

	debugPrintf("DEBUG: configType: %v\n", configType)

	for i := 0; i < configType.NumField(); i++ {
		field := configType.Field(i)
		yamlTag := field.Tag.Get("yaml")
		yamlKey := strings.Split(yamlTag, ",")[0]

		debugPrintf("DEBUG: Checking field: %s (YAML key: %s)\n", field.Name, yamlKey)

		if value, ok := inputMap[yamlKey]; ok {
			debugPrintf("DEBUG: Found YAML key '%s' in inputMap\n", yamlKey)

			switch v := value.(type) {
			case map[string]interface{}:
				debugPrintf("DEBUG: Field '%s' is a map, recursing...\n", yamlKey)
				nestedFieldValue := configValue.Field(i)
				if nestedFieldValue.Kind() == reflect.Struct {
					if err := checkExtraFields(v, nestedFieldValue.Addr().Interface()); err != nil {
						return err
					}
				} else {
					debugPrintf("DEBUG: Field '%s' is a map, but struct field is not a struct. Skipping recursion.\n", yamlKey)
				}
			case []interface{}:
				debugPrintf("DEBUG: Field '%s' is a slice\n", yamlKey)
				nestedFieldValue := configValue.Field(i)
				if nestedFieldValue.Kind() == reflect.Slice {
					elementType := nestedFieldValue.Type().Elem()
					debugPrintf("DEBUG: Slice element type: %v\n", elementType)
					if elementType.Kind() == reflect.Struct {
						for j, sliceElement := range v {
							debugPrintf("DEBUG: Checking slice element %d\n", j)
							if elementMap, ok := sliceElement.(map[string]interface{}); ok {
								if elementType.Kind() == reflect.Ptr {
									newElement := reflect.New(elementType.Elem()).Interface() // Create a new instance
									if err := checkExtraFields(elementMap, newElement); err != nil {
										return err
									}
								} else {
									newElement := reflect.New(elementType).Interface()
									if err := checkExtraFields(elementMap, newElement); err != nil {
										return err
									}
								}
							} else {
								return fmt.Errorf("field '%s' elements must be maps, got: %T", yamlKey, sliceElement)
							}
						}
					} else {
						debugPrintf("DEBUG: Field '%s' is a slice of a basic type. Skipping recursion.\n", yamlKey)
					}
				} else {
					debugPrintf("DEBUG: Field '%s' is NOT slice. Skipping recursion.\n", yamlKey)
				}
			default:
				debugPrintf("DEBUG: Field '%s' value: %+v (Type: %T)\n", yamlKey, value, value)
				if _, ok := value.(string); !ok && field.Type.Kind() != reflect.String {
					fieldType := configValue.Field(i).Type().String()
					return fmt.Errorf("field '%s' must be of type '%s', got: %T", yamlKey, fieldType, value)
				}
			}
		} else {
			debugPrintf("DEBUG: YAML key '%s' NOT found in inputMap\n", yamlKey)
		}
	}

	for key := range inputMap {
		debugPrintf("DEBUG: Checking for extra field: %s\n", key)
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
			return fmt.Errorf("unknown field '%s' in configuration", key)
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

// RegisterCustomValidators registers all the custom validators
func RegisterCustomValidators(validate *validator.Validate) {
	validate.RegisterValidation("ansiblebool", ansibleBoolValidator)
	validate.RegisterValidation("timezone_or_auto", timezoneOrAutoValidator)
	validate.RegisterValidation("cron_special_time", cronSpecialTimeValidator)
}
