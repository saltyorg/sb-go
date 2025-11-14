package logging

import "fmt"

// Debug prints a debug message with the DEBUG prefix if verbosity level is greater than 0.
// This is a convenience function to standardize debug output across the codebase.
//
// Usage:
//
//	logging.Debug(verbosity, "Cache found for %s", repoPath)
//	logging.Debug(verbosity, "No suggestions needed, continuing")
func Debug(verbosity int, format string, args ...interface{}) {
	if verbosity > 0 {
		message := fmt.Sprintf(format, args...)
		fmt.Printf("DEBUG: %s\n", message)
	}
}

// DebugBool prints a debug message with the DEBUG prefix if verbose mode is enabled.
// This variant accepts a boolean flag instead of an integer verbosity level.
// Useful for packages that use a simple on/off verbose mode.
//
// Usage:
//
//	logging.DebugBool(verboseMode, "Processing field: %s", fieldName)
func DebugBool(verbose bool, format string, args ...interface{}) {
	if verbose {
		message := fmt.Sprintf(format, args...)
		fmt.Printf("DEBUG: %s\n", message)
	}
}

// Trace prints a trace message with the TRACE prefix if verbosity level is greater than 1.
// This is used for more detailed debugging output that's only needed for deep troubleshooting.
// Trace messages are more verbose than debug messages and typically include raw data dumps.
//
// Usage:
//
//	logging.Trace(verbosity, "Raw output:\n%s", string(output))
//	logging.Trace(verbosity, "Cache contents: %+v", cache)
func Trace(verbosity int, format string, args ...interface{}) {
	if verbosity > 1 {
		message := fmt.Sprintf(format, args...)
		fmt.Printf("TRACE: %s\n", message)
	}
}
