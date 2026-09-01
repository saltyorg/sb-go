package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func currentUsername(t *testing.T) string {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return current.Username
}

func TestSaveFactsRejectsExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.ini")
	const original = "[default]\ntoken = original\n"
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "role.ini")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	_, _, err := saveFacts(link, "default", map[string]string{"token": "changed"}, currentUsername(t))
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("saveFacts symlink error = %v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != original {
		t.Fatalf("symlink target changed to %q", contents)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("symlink target mode = %o, want 600", info.Mode().Perm())
	}
}

func TestFactReadsAndDeletesRejectExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.ini")
	if err := os.WriteFile(target, []byte("[default]\ntoken = original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "role.ini")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if _, err := loadFacts(link, "default", nil); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("loadFacts symlink error = %v", err)
	}
	if _, err := loadAllInstances(link); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("loadAllInstances symlink error = %v", err)
	}
	if _, err := deleteFacts(link, "key", "default", map[string]string{"token": ""}, currentUsername(t)); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("deleteFacts symlink error = %v", err)
	}
}

func TestSaveFactsUsesAtomicRegularFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "role.ini")
	if err := os.WriteFile(filePath, []byte("[default]\ntoken = original\n"), 0666); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}

	facts, changed, err := saveFacts(filePath, "default", map[string]string{"token": "changed"}, currentUsername(t))
	if err != nil {
		t.Fatal(err)
	}
	if !changed || facts["token"] != "changed" {
		t.Fatalf("saveFacts = facts %#v, changed %t", facts, changed)
	}
	replacementInfo, err := os.Lstat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !replacementInfo.Mode().IsRegular() || replacementInfo.Mode().Perm() != 0640 {
		t.Fatalf("replacement mode = %v", replacementInfo.Mode())
	}
	if os.SameFile(originalInfo, replacementInfo) {
		t.Fatal("facts file was modified in place instead of atomically replaced")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".role.ini-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestSaveFactsRejectsSymlinkedDirectory(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "facts")
	if err := os.Symlink(targetDir, linkDir); err != nil {
		t.Fatal(err)
	}

	_, _, err := saveFacts(filepath.Join(linkDir, "role.ini"), "default", map[string]string{"token": "changed"}, currentUsername(t))
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("saveFacts directory symlink error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "role.ini")); !os.IsNotExist(err) {
		t.Fatalf("file created through symlinked directory: %v", err)
	}
}

func TestSaveFactsSerializesConcurrentWriters(t *testing.T) {
	const writers = 32
	username := currentUsername(t)
	filePath := filepath.Join(t.TempDir(), "role.ini")
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup

	for writer := range writers {
		wg.Go(func() {
			<-start
			_, _, err := saveFacts(
				filePath,
				fmt.Sprintf("instance-%d", writer),
				map[string]string{"value": fmt.Sprint(writer)},
				username,
			)
			errs <- err
		})
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent save failed: %v", err)
		}
	}

	instances, err := loadAllInstances(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != writers {
		t.Fatalf("retained %d/%d concurrent updates", len(instances), writers)
	}
}

func TestSaveFactsWaitsForSharedSidecarLock(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "role.ini")
	lockPath := filePath + ".lock"
	username := currentUsername(t)
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := saveFacts(filePath, "default", map[string]string{"token": "value"}, username)
		done <- err
	}()

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-done:
		t.Fatalf("save completed while shared lock was held: %v", err)
	case <-timer.C:
	}

	if err := unix.Flock(int(lock.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("save did not resume after shared lock was released")
	}
}

func TestSaveFactsRejectsSymlinkedSidecarLock(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "role.ini")
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filePath+".lock"); err != nil {
		t.Fatal(err)
	}

	_, _, err := saveFacts(filePath, "default", map[string]string{"token": "value"}, currentUsername(t))
	if err == nil || !strings.Contains(err.Error(), "lock must not be a symlink") {
		t.Fatalf("saveFacts lock symlink error = %v", err)
	}
	contents, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "unchanged" {
		t.Fatalf("lock symlink target changed to %q", contents)
	}
}

func TestFactFileLockTimesOut(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "role.ini")
	lock, err := os.OpenFile(filePath+".lock", os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	called := false
	err = withFactFileLockTimeout(filePath, 25*time.Millisecond, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("lock timeout error = %v", err)
	}
	if called {
		t.Fatal("locked action ran after timeout")
	}
}

func TestDeleteFactsWaitsForSharedSidecarLock(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "role.ini")
	username := currentUsername(t)
	if _, _, err := saveFacts(filePath, "default", map[string]string{"token": "value"}, username); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filePath+".lock", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := deleteFacts(filePath, "key", "default", map[string]string{"token": ""}, username)
		done <- err
	}()

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-done:
		t.Fatalf("delete completed while shared lock was held: %v", err)
	case <-timer.C:
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("delete did not resume after shared lock was released")
	}
}
