package saltbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/terminal"
)

// Define custom errors for specific conditions.
var (
	ErrRcloneNotInstalled   = errors.New("rclone is not installed")
	ErrRcloneConfigNotFound = errors.New("rclone config file not found")
	ErrSystemUserNotFound   = errors.New("system user not found")
)

// ValidateRcloneRemote checks if the given rclone remote exists.
func ValidateRcloneRemote(remoteName string, verbose bool) error {
	terminal.DebugBool(verbose, "ValidateRcloneRemote called with remoteName: '%s'", remoteName)
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
	if err != nil {
		err := fmt.Errorf("%w: %v", ErrRcloneNotInstalled, err)
		terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	terminal.DebugBool(verbose, "ValidateRcloneRemote - rclone is installed")
	// Get the Saltbox user.
	rcloneUser, err := host.GetSaltboxUser()
	if err != nil {
		terminal.DebugBool(verbose, "ValidateRcloneRemote - error getting Saltbox user: %v", err)
		return fmt.Errorf("%w: %v", ErrSystemUserNotFound, err)
	}
	terminal.DebugBool(verbose, "ValidateRcloneRemote - Saltbox user: '%s'", rcloneUser)

	// Check if the user exists on the system.
	_, err = user.Lookup(rcloneUser)
	if err != nil {
		terminal.DebugBool(verbose, "ValidateRcloneRemote - error looking up user")
		if _, ok := errors.AsType[user.UnknownUserError](err); ok {
			err := fmt.Errorf("%w: user '%s' does not exist", ErrSystemUserNotFound, rcloneUser)
			terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
			return err
		}
		// Some other error occurred during user lookup.
		err := fmt.Errorf("error looking up user '%s': %w", rcloneUser, err)
		terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	terminal.DebugBool(verbose, "ValidateRcloneRemote - user exists")

	// Define the rclone config path (standard location).
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	terminal.DebugBool(verbose, "ValidateRcloneRemote - rcloneConfigPath: '%s'", rcloneConfigPath)

	// Check if the rclone config file exists
	_, err = os.Stat(rcloneConfigPath)
	if os.IsNotExist(err) {
		err := fmt.Errorf("%w: %v", ErrRcloneConfigNotFound, err)
		terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	terminal.DebugBool(verbose, "ValidateRcloneRemote - rclone config file exists")

	// Use context with timeout for external command execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "sudo",
		executor.WithArgs("-u", rcloneUser, "rclone", "listremotes"),
		executor.WithInheritEnv(fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath)),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		err := fmt.Errorf("failed to list rclone remotes: %w", err)
		terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	output := result.Combined
	remoteFound, remoteCount := rcloneRemoteListed(output, remoteName)
	terminal.DebugBool(verbose, "ValidateRcloneRemote - rclone returned %d remote names", remoteCount)
	if !remoteFound {
		err := fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
		terminal.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}

	terminal.DebugBool(verbose, "ValidateRcloneRemote - rclone remote exists")
	return nil
}

func rcloneRemoteListed(output []byte, remoteName string) (bool, int) {
	found := false
	count := 0
	for line := range strings.SplitSeq(string(output), "\n") {
		candidate := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if candidate == "" {
			continue
		}
		count++
		if candidate == remoteName {
			found = true
		}
	}
	return found, count
}
