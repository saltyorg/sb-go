package saltbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyConfigFileCreatesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "accounts.yml.default")
	destination := filepath.Join(dir, "accounts.yml")
	const contents = "user:\n  pass: credential-sentinel\n"
	if err := os.WriteFile(source, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyConfigFile(source, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Fatalf("copied contents = %q, want %q", got, contents)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("config mode = %04o, want 0600", mode)
	}
}

func TestCopyConfigFileRejectsExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "accounts.yml.default")
	target := filepath.Join(dir, "target")
	destination := filepath.Join(dir, "accounts.yml")
	if err := os.WriteFile(source, []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	if err := copyConfigFile(source, destination); err == nil {
		t.Fatal("expected existing symlink to be rejected")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestSecureExistingConfigFileHardensMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.yml")
	if err := os.WriteFile(path, []byte("credential"), 0755); err != nil {
		t.Fatal(err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureExistingConfigFile(path, pathInfo); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("config mode = %04o, want 0600", mode)
	}
}

func TestSecureExistingConfigFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "accounts.yml")
	if err := os.WriteFile(target, []byte("credential"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureExistingConfigFile(path, pathInfo); err == nil {
		t.Fatal("expected existing symlink to be rejected")
	}
}

func TestSecureExistingConfigFilesMigratesOnlyPresentConfigs(t *testing.T) {
	repoPath := t.TempDir()
	defaultsPath := filepath.Join(repoPath, "defaults")
	if err := os.Mkdir(defaultsPath, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"accounts.yml", "settings.yml"} {
		if err := os.WriteFile(filepath.Join(defaultsPath, name+".default"), []byte("default"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	accountsPath := filepath.Join(repoPath, "accounts.yml")
	const contents = "password: credential-sentinel\n"
	if err := os.WriteFile(accountsPath, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(accountsPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := secureExistingConfigFiles(repoPath); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(accountsPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(accountsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Fatalf("config content changed to %q", got)
	}
	if !os.SameFile(before, after) {
		t.Fatal("config inode changed during permission migration")
	}
	if after.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %04o, want 0600", after.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(repoPath, "settings.yml")); !os.IsNotExist(err) {
		t.Fatalf("missing config was created: %v", err)
	}
}
