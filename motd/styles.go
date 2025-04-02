package motd

import (
	"github.com/saltyorg/sb-go/styles"
)

// Use styles from the global styles package
var (
	// DefaultStyle inherits from the global default
	DefaultStyle = styles.DefaultStyle

	// Specific MOTD styles based on global styles
	GreenStyle  = styles.SuccessStyle.Bold(true)
	RedStyle    = styles.ErrorStyle
	YellowStyle = styles.WarningStyle
	BlueStyle   = styles.InfoStyle
	DimStyle    = styles.DimStyle

	// Specific use case styles
	KeyStyle   = styles.KeyStyle
	ValueStyle = styles.ValueStyle
)
