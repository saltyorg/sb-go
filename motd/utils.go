package motd

import (
	"os/exec"
	"strings"
)

// ExecCommand executes a command and returns its output as a string
func ExecCommand(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "Not available"
	}
	return strings.TrimSpace(string(output))
}
