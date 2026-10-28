package terminal

import (
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

// isInteractive is checked once because the process stdout does not change.
var isInteractive = isatty.IsTerminal(os.Stdout.Fd())

// IsInteractive returns whether stdout is connected to a terminal.
// Returns false if output is redirected, piped, or in a non-interactive environment.
func IsInteractive() bool {
	return isInteractive
}

// CanReadTerminalInput reports whether this process may switch its terminal to
// raw mode and read keys from it. That requires the process to be in the
// terminal's foreground process group: from a background group the kernel
// stops the process with SIGTTOU or SIGTTIN instead.
//
// This matters for `curl ... | sudo bash`: with use_pty, sudo starts a command
// whose stdin is a pipe in a background process group on its pseudo-terminal.
// Classic sudo moves the command to the foreground once it stops, but sudo-rs
// does not, so the command stays stopped forever.
func CanReadTerminalInput() bool {
	// Bubble Tea reads from stdin when it is a terminal and opens /dev/tty otherwise.
	if isatty.IsTerminal(os.Stdin.Fd()) {
		return isForegroundProcessGroup(os.Stdin)
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer func() { _ = tty.Close() }()
	return isForegroundProcessGroup(tty)
}

// isForegroundProcessGroup reports whether this process belongs to the
// foreground process group of the terminal behind file.
func isForegroundProcessGroup(file *os.File) bool {
	foreground, err := unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
	return err == nil && foreground == unix.Getpgrp()
}
