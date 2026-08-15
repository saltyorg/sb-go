package cmd

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
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
