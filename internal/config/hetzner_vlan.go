package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML called\n")
	var s string
	var i int

	// Try unmarshalling as a string first
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - trying to unmarshal as string\n")
	if err := unmarshal(&s); err == nil {
		*soi = StringOrInt(s)
		debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - unmarshaled as string: '%s'\n", *soi)
		return nil
	}
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - failed to unmarshal as string\n")

	// If that fails, try unmarshalling as an int
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - trying to unmarshal as int\n")
	if err := unmarshal(&i); err == nil {
		*soi = StringOrInt(fmt.Sprintf("%d", i)) // Convert int to string
		debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - unmarshaled as int: %d, converted to string: '%s'\n", i, *soi)
		return nil
	}
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - failed to unmarshal as int\n")

	err := fmt.Errorf("invalid value for StringOrInt: must be a string or an integer")
	debugPrintf("DEBUG: StringOrInt.UnmarshalYAML - %v\n", err)
	return err

}

// wholeNumberValidator is a custom validator to ensure the value is a whole number (int or string).
func wholeNumberValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	debugPrintf("DEBUG: wholeNumberValidator called with value: '%s'\n", value)

	_, err := strconv.Atoi(value) // Try converting to int
	isValid := err == nil         // If no error, it's a whole number
	debugPrintf("DEBUG: wholeNumberValidator - strconv.Atoi returned error: %v, is valid: %t\n", err, isValid)
	return isValid
}

// ValidateHetznerVLANConfig validates the HetznerVLANConfig struct.
func ValidateHetznerVLANConfig(config *HetznerVLANConfig, inputMap map[string]any) error {
	debugPrintf("\nDEBUG: ValidateHetznerVLANConfig called with config: %+v, inputMap: %+v\n", config, inputMap)
	validate := validator.New()
	debugPrintf("DEBUG: ValidateHetznerVLANConfig - registering custom validators\n")
	RegisterCustomValidators(validate)
	debugPrintf("DEBUG: ValidateHetznerVLANConfig - registering whole_number validator\n")
	err := validate.RegisterValidation("whole_number", wholeNumberValidator)
	if err != nil {
		err := fmt.Errorf("failed to register whole_number validator: %w", err)
		debugPrintf("DEBUG: ValidateHetznerVLANConfig - %v\n", err)
		return err
	}

	// Validate the overall structure.
	debugPrintf("DEBUG: ValidateHetznerVLANConfig - validating struct: %+v\n", config)
	if err := validate.Struct(config); err != nil {
		debugPrintf("DEBUG: ValidateHetznerVLANConfig - struct validation error: %v\n", err)
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				// Get the full path to the field based on the namespace
				fieldPath := e.Namespace()
				// Remove the "Config." prefix to make the error message cleaner
				fieldPath = strings.Replace(fieldPath, "HetznerVLANConfig.", "", 1)
				// Convert to lowercase for consistency
				fieldPath = strings.ToLower(fieldPath)

				debugPrintf("DEBUG: ValidateHetznerVLANConfig - validation error on field '%s', tag '%s', value '%v', param '%s'\n", fieldPath, e.Tag(), e.Value(), e.Param())

				switch e.Tag() {
				case "required":
					err := fmt.Errorf("field '%s' is required", fieldPath)
					debugPrintf("DEBUG: ValidateHetznerVLANConfig - %v\n", err)
					return err
				case "whole_number":
					err := fmt.Errorf("field '%s' must be a whole number (integer or string representation of an integer), got: %s", fieldPath, e.Value())
					debugPrintf("DEBUG: ValidateHetznerVLANConfig - %v\n", err)
					return err
				default:
					err := fmt.Errorf("field '%s' is invalid: %s", fieldPath, e.Error())
					debugPrintf("DEBUG: ValidateHetznerVLANConfig - %v\n", err)
					return err
				}
			}
		}
		return err
	}

	// Check for extra fields.
	debugPrintf("DEBUG: ValidateHetznerVLANConfig - checking for extra fields\n")
	if err := checkExtraFields(inputMap, config); err != nil {
		debugPrintf("DEBUG: ValidateHetznerVLANConfig - checkExtraFields returned error: %v\n", err)
		return err
	}

	debugPrintf("DEBUG: ValidateHetznerVLANConfig - validation successful\n")
	return nil
}
