package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/saltyorg/sb-go/internal/logging"

	"github.com/go-playground/validator/v10"
)

// HetznerVLANConfig represents the Hetzner VLAN configuration.
type HetznerVLANConfig struct {
	HetznerVLAN HetznerVLANSection `yaml:"hetzner_vlan" validate:"required"`
}

// HetznerVLANSection holds the Hetzner VLAN settings.
type HetznerVLANSection struct {
	ClientNumber StringOrInt `yaml:"client_number" validate:"required,whole_number"`
	VLANID       StringOrInt `yaml:"vlan_id" validate:"required,whole_number"`
}

// StringOrInt is a custom type that can be either a string or an int,
// but if it's a string, it must represent a whole number.
type StringOrInt string

// UnmarshalYAML implements custom unmarshalling for StringOrInt.
func (soi *StringOrInt) UnmarshalYAML(unmarshal func(any) error) error {
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML called")
	var s string
	var i int

	// Try unmarshalling as a string first
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - trying to unmarshal as string")
	if err := unmarshal(&s); err == nil {
		*soi = StringOrInt(s)
		logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - unmarshaled as string: '%s'", *soi)
		return nil
	}
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - failed to unmarshal as string")

	// If that fails, try unmarshalling as an int
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - trying to unmarshal as int")
	if err := unmarshal(&i); err == nil {
		*soi = StringOrInt(fmt.Sprintf("%d", i)) // Convert int to string
		logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - unmarshaled as int: %d, converted to string: '%s'", i, *soi)
		return nil
	}
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - failed to unmarshal as int")

	err := fmt.Errorf("invalid value for StringOrInt: must be a string or an integer")
	logging.DebugBool(verboseMode, "StringOrInt.UnmarshalYAML - %v", err)
	return err

}

// wholeNumberValidator is a custom validator to ensure the value is a whole number (int or string).
func wholeNumberValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	logging.DebugBool(verboseMode, "wholeNumberValidator called with value: '%s'", value)

	_, err := strconv.Atoi(value) // Try converting to int
	isValid := err == nil         // If no error, it's a whole number
	logging.DebugBool(verboseMode, "wholeNumberValidator - strconv.Atoi returned error: %v, is valid: %t", err, isValid)
	return isValid
}

// ValidateHetznerVLANConfig validates the HetznerVLANConfig struct.
func ValidateHetznerVLANConfig(config *HetznerVLANConfig, inputMap map[string]any) error {
	logging.DebugBool(verboseMode, "\nDEBUG: ValidateHetznerVLANConfig called with config: %+v, inputMap: %+v", config, inputMap)
	validate := validator.New()
	logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - registering custom validators")
	if err := RegisterCustomValidators(validate); err != nil {
		return err
	}
	logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - registering whole_number validator")
	err := validate.RegisterValidation("whole_number", wholeNumberValidator)
	if err != nil {
		err := fmt.Errorf("failed to register whole_number validator: %w", err)
		logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - %v", err)
		return err
	}

	// Validate the overall structure.
	logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - validating struct: %+v", config)
	if err := validate.Struct(config); err != nil {
		logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - struct validation error: %v", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "HetznerVLANConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - validation error on field '%s', tag '%s', value '%v', param '%s'", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - %v", err)
					return err
				case "whole_number":
					err := fmt.Errorf("field '%s' must be a whole number (integer or string representation of an integer), got: %s", fieldPath, e.Value())
					logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - %v", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - %v", err)
					return err
				}
			}
		}
		return err
	}

	// Check for extra fields.
	logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - checking for extra fields")
	if err := checkExtraFields(inputMap, config); err != nil {
		logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - checkExtraFields returned error: %v", err)
		return err
	}

	logging.DebugBool(verboseMode, "ValidateHetznerVLANConfig - validation successful")
	return nil
}
