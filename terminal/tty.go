package terminal

import (
	"os"

	"github.com/mattn/go-isatty"
)

// isInteractive is checked once because the process stdout does not change.
var isInteractive = isatty.IsTerminal(os.Stdout.Fd())

// IsInteractive returns whether stdout is connected to a terminal.
// Returns false if output is redirected, piped, or in a non-interactive environment.
func IsInteractive() bool {
	return isInteractive
}
