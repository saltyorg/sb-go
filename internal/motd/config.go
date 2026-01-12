package motd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/validate"
)

// sectionOrder defines the order of sections in generated config (alphabetical).
// This is needed because YAML maps don't preserve key order.
var sectionOrder = []string{
	"colors",
	"emby",
	"jellyfin",
	"lidarr",
	"nzbget",
	"plex",
	"qbittorrent",
	"radarr",
	"readarr",
	"rtorrent",
	"sabnzbd",
	"sonarr",
	"systemd",
}

// GenerateExampleConfig returns a YAML string with an example MOTD configuration
// where all sections are disabled and contain placeholder values.
// It reads the schema file to dynamically generate the config based on
// descriptions and example values defined in the schema.
// Color values are taken from the actual defaults defined in this package.
func GenerateExampleConfig() (string, error) {
	schema, err := validate.LoadSchema(constants.SaltboxMOTDSchemaPath)
	if err != nil {
		return "", fmt.Errorf("failed to load schema: %w", err)
	}

	var buf strings.Builder
	buf.WriteString("# Saltbox MOTD Configuration\n")
	buf.WriteString("# All sections are disabled by default. Set enabled: true and fill in the values to use.\n")

	for _, sectionName := range sectionOrder {
		rule, exists := schema.Rules[sectionName]
		if !exists {
			continue
		}

		buf.WriteString("\n")

		// Handle colors section specially - use Go defaults for values
		if sectionName == "colors" {
			buf.WriteString(generateColorsSection(rule))
			continue
		}

		// Write section comment from description
		if rule.Description != "" {
			for line := range strings.SplitSeq(strings.TrimSpace(rule.Description), "\n") {
				buf.WriteString(fmt.Sprintf("# %s\n", line))
			}
		}

		// Generate the section based on its structure
		buf.WriteString(fmt.Sprintf("%s:\n", sectionName))
		generateSection(&buf, rule, "  ")
	}

	return buf.String(), nil
}

// generateSection generates YAML for a section based on its schema rule.
func generateSection(buf *strings.Builder, rule *validate.SchemaRule, indent string) {
	if rule.Properties == nil {
		return
	}

	// Define field order for section properties
	fieldOrder := []string{"enabled", "instances", "additional_services", "display_names"}

	for _, fieldName := range fieldOrder {
		propRule, exists := rule.Properties[fieldName]
		if !exists {
			continue
		}

		// Handle instances array specially - generate example item
		if fieldName == "instances" && propRule.Type == "array" && propRule.Items != nil {
			buf.WriteString(fmt.Sprintf("%sinstances:\n", indent))
			generateInstanceExample(buf, propRule.Items, indent+"  ")
			continue
		}

		// Use example if available
		if propRule.Example != nil {
			buf.WriteString(fmt.Sprintf("%s%s: %s\n", indent, fieldName, formatValue(propRule.Example)))
		}
	}
}

// formatValue formats a value for YAML output.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return fmt.Sprintf("%v", val)
	case int, int64, float64:
		return fmt.Sprintf("%v", val)
	default:
		encoded, err := json.Marshal(val)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("%v", val)
	}
}

// generateInstanceExample generates an example instance entry from schema.
func generateInstanceExample(buf *strings.Builder, itemRule *validate.SchemaRule, indent string) {
	if itemRule.Properties == nil {
		return
	}

	buf.WriteString(fmt.Sprintf("%s- ", indent))
	first := true

	// Define field order for instances
	fieldOrder := []string{"name", "url", "apikey", "token", "user", "password", "timeout", "enabled"}

	for _, fieldName := range fieldOrder {
		fieldRule, exists := itemRule.Properties[fieldName]
		if !exists {
			continue
		}

		// Only include fields that have an example value defined
		if fieldRule.Example == nil {
			continue
		}

		if first {
			first = false
		} else {
			buf.WriteString(fmt.Sprintf("%s  ", indent))
		}

		buf.WriteString(fmt.Sprintf("%s: %s\n", fieldName, formatValue(fieldRule.Example)))
	}
}

// generateColorsSection generates the colors section with Go defaults.
// It uses the description from schema but values from Go constants.
func generateColorsSection(rule *validate.SchemaRule) string {
	var buf strings.Builder

	// Write description from schema
	if rule.Description != "" {
		for line := range strings.SplitSeq(strings.TrimSpace(rule.Description), "\n") {
			buf.WriteString(fmt.Sprintf("# %s\n", line))
		}
	}

	buf.WriteString(fmt.Sprintf(`colors:
  text:
    label: "%s"
    value: "%s"
    app_name: "%s"
  status:
    warning: "%s"
    success: "%s"
    error: "%s"
  progress_bar:
    low: "%s"
    high: "%s"
    critical: "%s"
`, defaultKey, defaultValue, defaultAppName,
		defaultWarning, defaultSuccess, defaultError,
		defaultProgressBarLow, defaultProgressBarHigh, defaultProgressBarCritical))

	return buf.String()
}
