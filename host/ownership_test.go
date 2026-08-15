package host

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func TestEnsureRecursiveForExistingUserSkipsUnknownUser(t *testing.T) {
	name := "sb-go-user-that-does-not-exist-7fcd24d1"
	if _, err := user.Lookup(name); err == nil {
		t.Fatalf("test user %q unexpectedly exists", name)
	}
	if err := EnsureRecursiveForExistingUser(name, filepath.Join(t.TempDir(), "managed")); err != nil {
		t.Fatalf("unknown user should be skipped: %v", err)
	}
}

func TestEnsureRecursiveForExistingUserSkipsMissingPath(t *testing.T) {
	account, err := user.Current()
	if err != nil {
		t.Fatalf("look up current user: %v", err)
	}
	if err := EnsureRecursiveForExistingUser(account.Username, filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("missing path should be skipped: %v", err)
	}
}

func TestEnsureRecursiveIncludesSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	link := filepath.Join(root, "active")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	originalLchown := lchown
	t.Cleanup(func() { lchown = originalLchown })
	visited := make(map[string]bool)
	lchown = func(path string, _, _ int) error {
		visited[path] = true
		return nil
	}
	if err := ensureRecursive(root, 1<<30, 1<<30); err != nil {
		t.Fatalf("ensure ownership: %v", err)
	}
	if !visited[root] {
		t.Fatal("managed root was not included")
	}
	if !visited[link] {
		t.Fatal("managed symlink was not included")
	}
	if visited[target] {
		t.Fatal("symlink target outside the managed root was followed")
	}
}
