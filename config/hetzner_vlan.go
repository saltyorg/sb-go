package config

import (
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"strconv"
	"strings"
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
func (soi *StringOrInt) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	var i int

	// Try unmarshalling as a string first
	if err := unmarshal(&s); err == nil {
		*soi = StringOrInt(s)
		return nil
	}

	// If that fails, try unmarshalling as an int
	if err := unmarshal(&i); err == nil {
		*soi = StringOrInt(fmt.Sprintf("%d", i)) // Convert int to string
		return nil
	}

	return fmt.Errorf("invalid value for StringOrInt: must be a string or an integer")

}

// wholeNumberValidator is a custom validator to ensure the value is a whole number (int or string).
func wholeNumberValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	_, err := strconv.Atoi(value) // Try converting to int
	return err == nil             // If no error, it's a whole number
}

// ValidateHetznerVLANConfig validates the HetznerVLANConfig struct.
func ValidateHetznerVLANConfig(config *HetznerVLANConfig, inputMap map[string]interface{}) error {
	validate := validator.New()
	RegisterCustomValidators(validate)
	err := validate.RegisterValidation("whole_number", wholeNumberValidator)
	if err != nil {
		return fmt.Errorf("failed to register whole_number validator: %w", err)
	}

	// Validate the overall structure.
	if err := validate.Struct(config); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				lowercaseField := strings.ToLower(e.Field())
				switch e.Tag() {
				case "required":
					return fmt.Errorf("field '%s' is required", lowercaseField)
				case "whole_number":
					return fmt.Errorf("field '%s' must be a whole number (integer or string representation of an integer), got: %s", lowercaseField, e.Value())
				default:
					return fmt.Errorf("field '%s' is invalid: %s", lowercaseField, e.Error())
				}
			}
		}
		return err
	}

	return checkExtraFields(inputMap, config) // Use the function from generic.go
}
