package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/logging"
	"github.com/saltyorg/sb-go/internal/utils"
)

// Define custom errors for specific conditions.
var (
	ErrRcloneNotInstalled   = errors.New("rclone is not installed")
	ErrRcloneConfigNotFound = errors.New("rclone config file not found")
	ErrSystemUserNotFound   = errors.New("system user not found")
)

// ValidateRcloneRemote checks if the given rclone remote exists.
func ValidateRcloneRemote(remoteName string, verbose bool) error {
	logging.DebugBool(verbose, "ValidateRcloneRemote called with remoteName: '%s'", remoteName)
	// Check if rclone is installed.
	_, err := exec.LookPath("rclone")
	if err != nil {
		err := fmt.Errorf("%w: %v", ErrRcloneNotInstalled, err)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verbose, "ValidateRcloneRemote - rclone is installed")
	// Get the Saltbox user.
	rcloneUser, err := utils.GetSaltboxUser()
	if err != nil {
		logging.DebugBool(verbose, "ValidateRcloneRemote - error getting Saltbox user: %v", err)
		return fmt.Errorf("%w: %v", ErrSystemUserNotFound, err)
	}
	logging.DebugBool(verbose, "ValidateRcloneRemote - Saltbox user: '%s'", rcloneUser)

	// Check if the user exists on the system.
	_, err = user.Lookup(rcloneUser)
	if err != nil {
		logging.DebugBool(verbose, "ValidateRcloneRemote - error looking up user")
		var unknownUserError user.UnknownUserError
		if errors.As(err, &unknownUserError) {
			err := fmt.Errorf("%w: user '%s' does not exist", ErrSystemUserNotFound, rcloneUser)
			logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
			return err
		}
		// Some other error occurred during user lookup.
		err := fmt.Errorf("error looking up user '%s': %w", rcloneUser, err)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verbose, "ValidateRcloneRemote - user exists")

	// Define the rclone config path (standard location).
	rcloneConfigPath := fmt.Sprintf("/home/%s/.config/rclone/rclone.conf", rcloneUser)
	logging.DebugBool(verbose, "ValidateRcloneRemote - rcloneConfigPath: '%s'", rcloneConfigPath)

	// Check if the rclone config file exists
	_, err = os.Stat(rcloneConfigPath)
	if os.IsNotExist(err) {
		err := fmt.Errorf("%w: %v", ErrRcloneConfigNotFound, err)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verbose, "ValidateRcloneRemote - rclone config file exists")

	// Use context with timeout for external command execution
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := executor.Run(ctx, "sudo",
		executor.WithArgs("-u", rcloneUser, "rclone", "config", "show"),
		executor.WithInheritEnv(fmt.Sprintf("RCLONE_CONFIG=%s", rcloneConfigPath)),
		executor.WithOutputMode(executor.OutputModeCombined),
	)
	if err != nil {
		err := fmt.Errorf("failed to execute rclone config show: %w, output: %s", err, result.Combined)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	output := result.Combined
	logging.DebugBool(verbose, "ValidateRcloneRemote - rclone config show output: '%s'", string(output))

	// Use a regular expression to search for the remote within the rclone config show output.
	remoteRegex := fmt.Sprintf(`(?m)^\[%s\]$`, regexp.QuoteMeta(remoteName))
	re, err := regexp.Compile(remoteRegex)
	if err != nil {
		err := fmt.Errorf("failed to compile regex for remote name: %w", err)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}
	logging.DebugBool(verbose, "ValidateRcloneRemote - remoteRegex: '%s'", remoteRegex)

	if !re.MatchString(string(output)) {
		err := fmt.Errorf("rclone remote '%s' not found in configuration", remoteName)
		logging.DebugBool(verbose, "ValidateRcloneRemote - %v", err)
		return err
	}

	logging.DebugBool(verbose, "ValidateRcloneRemote - rclone remote exists")
	return nil
}
