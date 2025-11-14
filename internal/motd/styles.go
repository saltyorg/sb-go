package motd

import (
	"os"

	"github.com/saltyorg/sb-go/internal/config"
	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/styles"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/charmtone"
)

// Default Fang colors
var (
	defaultKey     = charmtone.Cheeky.Hex()   // #FF79D0
	defaultValue   = charmtone.Guac.Hex()     // #12C78F
	defaultWarning = charmtone.Mustard.Hex()  // #F5EF34
	defaultSuccess = charmtone.Guac.Hex()     // #12C78F
	defaultError   = charmtone.Sriracha.Hex() // #EB4268
	defaultAppName = charmtone.Cumin.Hex()    // #BF976F

	// Default progress bar colors
	defaultProgressBarLow      = "#A8CC8C" // 0-79% usage (low/good)
	defaultProgressBarHigh     = "#DBAB79" // 80-89% usage (high/warning)
	defaultProgressBarCritical = "#E88388" // 90-100% usage (critical/danger)
)

// Use styles from the global styles package
//
//goland:noinspection GoUnusedGlobalVariable
var (
	// DefaultStyle inherits from the global default
	DefaultStyle = styles.DefaultStyle

	// Specific MOTD styles based on global styles
	BlueStyle = styles.InfoStyle
	DimStyle  = styles.DimStyle

	// Specific use case styles using Fang color scheme
	// These are initialized with defaults and can be overridden by config
	KeyStyle     = createColorStyle(defaultKey)
	ValueStyle   = createColorStyle(defaultValue)
	WarningStyle = createColorStyle(defaultWarning)
	SuccessStyle = createColorStyle(defaultSuccess)
	ErrorStyle   = createColorStyle(defaultError)
	AppNameStyle = createColorStyle(defaultAppName)

	// Progress bar colors
	// These are initialized with defaults and can be overridden by config
	ProgressBarLow      = defaultProgressBarLow
	ProgressBarHigh     = defaultProgressBarHigh
	ProgressBarCritical = defaultProgressBarCritical
)

// createColorStyle creates a simple colored style
func createColorStyle(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

// InitializeColors loads custom colors from the MOTD config if available
func InitializeColors() {
	configPath := constants.SaltboxMOTDConfigPath

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return // Use defaults
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil || cfg.Colors == nil {
		// Use defaults if config doesn't exist or no colors specified
		return
	}

	// Override with user-defined colors if they are valid hex colors
	key := defaultKey
	value := defaultValue
	warning := defaultWarning
	success := defaultSuccess
	errorColor := defaultError
	appName := defaultAppName
	progressBarLow := defaultProgressBarLow
	progressBarHigh := defaultProgressBarHigh
	progressBarCritical := defaultProgressBarCritical

	if cfg.Colors.Text != nil {
		if cfg.Colors.Text.Label != "" {
			key = cfg.Colors.Text.Label
		}
		if cfg.Colors.Text.Value != "" {
			value = cfg.Colors.Text.Value
		}
		if cfg.Colors.Text.AppName != "" {
			appName = cfg.Colors.Text.AppName
		}
	}
	if cfg.Colors.Status != nil {
		if cfg.Colors.Status.Warning != "" {
			warning = cfg.Colors.Status.Warning
		}
		if cfg.Colors.Status.Success != "" {
			success = cfg.Colors.Status.Success
		}
		if cfg.Colors.Status.Error != "" {
			errorColor = cfg.Colors.Status.Error
		}
	}
	if cfg.Colors.ProgressBar != nil {
		if cfg.Colors.ProgressBar.Low != "" {
			progressBarLow = cfg.Colors.ProgressBar.Low
		}
		if cfg.Colors.ProgressBar.High != "" {
			progressBarHigh = cfg.Colors.ProgressBar.High
		}
		if cfg.Colors.ProgressBar.Critical != "" {
			progressBarCritical = cfg.Colors.ProgressBar.Critical
		}
	}

	// Re-create the styles with the custom colors
	KeyStyle = createColorStyle(key)
	ValueStyle = createColorStyle(value)
	WarningStyle = createColorStyle(warning)
	SuccessStyle = createColorStyle(success)
	ErrorStyle = createColorStyle(errorColor)
	AppNameStyle = createColorStyle(appName)

	// Update progress bar colors
	ProgressBarLow = progressBarLow
	ProgressBarHigh = progressBarHigh
	ProgressBarCritical = progressBarCritical
}
