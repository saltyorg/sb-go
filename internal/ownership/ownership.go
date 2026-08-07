package ownership

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

var lchown = os.Lchown

// EnsureForExistingUser assigns paths to username's primary group without
// traversing their contents. A missing user or path is intentionally ignored.
func EnsureForExistingUser(username string, paths ...string) error {
	uid, gid, exists, err := lookupIDs(username)
	if err != nil || !exists {
		return err
	}
	for _, path := range paths {
		if err := ensurePath(path, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// EnsureRecursiveForExistingUser assigns paths and their contents to username's
// primary group. A missing user or path is intentionally ignored.
func EnsureRecursiveForExistingUser(username string, paths ...string) error {
	uid, gid, exists, err := lookupIDs(username)
	if err != nil || !exists {
		return err
	}
	for _, root := range paths {
		if err := ensureRecursive(root, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

func lookupIDs(username string) (int, int, bool, error) {
	if username == "" {
		return 0, 0, false, nil
	}

	account, err := user.Lookup(username)
	if err != nil {
		var unknownUser user.UnknownUserError
		if errors.As(err, &unknownUser) {
			return 0, 0, false, nil
		}
		return 0, 0, false, fmt.Errorf("look up user %s: %w", username, err)
	}

	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse UID for user %s: %w", username, err)
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse primary GID for user %s: %w", username, err)
	}
	return uid, gid, true, nil
}

func ensurePath(path string, uid, gid int) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect ownership of %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read ownership of %s", path)
	}
	if int(stat.Uid) == uid && int(stat.Gid) == gid {
		return nil
	}
	if err := lchown(path, uid, gid); err != nil {
		return fmt.Errorf("set ownership of %s: %w", path, err)
	}
	return nil
}

func ensureRecursive(root string, uid, gid int) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("read ownership of %s", path)
		}
		if int(stat.Uid) == uid && int(stat.Gid) == gid {
			return nil
		}
		if err := lchown(path, uid, gid); err != nil {
			return fmt.Errorf("set ownership of %s: %w", path, err)
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("set recursive ownership of %s: %w", root, err)
	}
	return nil
}
