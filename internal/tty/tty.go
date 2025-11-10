package tty

import (
	"os"

	"github.com/mattn/go-isatty"
)

// isInteractive stores whether stdout is connected to a terminal.
// This is checked once at package initialization to avoid repeated syscalls.
var isInteractive bool

func init() {
	isInteractive = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// IsInteractive returns whether stdout is connected to a terminal.
// Returns false if output is redirected, piped, or in a non-interactive environment.
func IsInteractive() bool {
	return isInteractive
}
