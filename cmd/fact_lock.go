package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const (
	factLockTimeout      = 30 * time.Second
	factLockPollInterval = 50 * time.Millisecond
)

func withFactFileLock(filePath string, action func() error) error {
	return withFactFileLockTimeout(filePath, factLockTimeout, action)
}

func withFactFileLockTimeout(filePath string, timeout time.Duration, action func() error) error {
	lockPath := filePath + ".lock"
	if info, err := os.Lstat(lockPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("facts lock must not be a symlink: %s", lockPath)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("facts lock must be a regular file: %s", lockPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect facts lock: %w", err)
	}

	descriptor, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0640)
	if err != nil {
		return fmt.Errorf("open facts lock %s: %w", lockPath, err)
	}
	lock := os.NewFile(uintptr(descriptor), lockPath)
	if lock == nil {
		_ = unix.Close(descriptor)
		return fmt.Errorf("open facts lock %s", lockPath)
	}
	defer func() { _ = lock.Close() }()

	info, err := lock.Stat()
	if err != nil {
		return fmt.Errorf("inspect open facts lock %s: %w", lockPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("facts lock must be a regular file: %s", lockPath)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := unix.Flock(descriptor, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return fmt.Errorf("acquire facts lock %s: %w", lockPath, err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timed out after %s waiting for facts lock %s", timeout, lockPath)
		}
		time.Sleep(min(factLockPollInterval, remaining))
	}
	defer func() { _ = unix.Flock(descriptor, unix.LOCK_UN) }()

	return action()
}
