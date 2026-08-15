package cmd

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func encryptRestorePayload(t *testing.T, plaintext []byte, password string) []byte {
	t.Helper()
	salt := []byte("12345678")
	key, iv := deriveKeyAndIV([]byte(password), salt, false)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte(nil), plaintext...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return append(append([]byte("Salted__"), salt...), ciphertext...)
}

func TestRestoreTempDirectoryIsPrivate(t *testing.T) {
	path, err := newRestoreTempDir()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(path) }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("restore directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestValidateAndRestoreCleansTempDirectoryWhenSetupFails(t *testing.T) {
	folder, err := newRestoreTempDir()
	if err != nil {
		t.Fatal(err)
	}
	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := validateAndRestore(context.Background(), "user", "password", "https://example.invalid", filepath.Join(blockedParent, "restore"), folder, false); err == nil {
		t.Fatal("validateAndRestore() succeeded despite an invalid restore directory")
	}
	if _, err := os.Lstat(folder); !os.IsNotExist(err) {
		t.Fatalf("restore temp directory remains after setup failure: %v", err)
	}
}

func TestDownloadFileDoesNotFollowExistingSymlink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("malicious"))
	}))
	defer server.Close()
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "download")
	if err := os.Symlink(victim, destination); err != nil {
		t.Fatal(err)
	}
	if err := downloadFile(context.Background(), server.URL, destination); err == nil {
		t.Fatal("expected exclusive create to reject existing symlink")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "safe" {
		t.Fatalf("symlink victim = %q, want safe", got)
	}
}

func TestDownloadFileRemovesOversizedPartialFile(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), (16<<20)+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "download")
	if err := downloadFile(context.Background(), server.URL, destination); err == nil {
		t.Fatal("expected oversized download error")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial restore file remains: %v", err)
	}
}

func TestMoveFileAppliesRequestedModeOnRename(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "accounts.yml")
	if err := os.WriteFile(source, []byte("credential"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(source, destination, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("restored config mode = %04o, want 0600", mode)
	}
}

func TestMoveFileReplacesDestinationSymlinkWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	destination := filepath.Join(dir, "accounts.yml")
	if err := os.WriteFile(source, []byte("restored"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(source, destination, 0600); err != nil {
		t.Fatal(err)
	}
	targetContents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetContents) != "original" {
		t.Fatalf("symlink target changed to %q", targetContents)
	}
	destinationInfo, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !destinationInfo.Mode().IsRegular() || destinationInfo.Mode().Perm() != 0600 {
		t.Fatalf("restored destination mode = %v, want regular 0600", destinationInfo.Mode())
	}
}

func TestProcessRestoredFileWritesPrivateRegularFile(t *testing.T) {
	root := t.TempDir()
	temporary := filepath.Join(root, "temporary")
	destinationDir := filepath.Join(root, "saltbox")
	if err := os.MkdirAll(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		t.Fatal(err)
	}
	const (
		fileName = "accounts.yml"
		password = "restore-password"
		contents = "user:\n  pass: restored-credential\n"
	)
	encryptedPath := filepath.Join(temporary, fileName+".enc")
	if err := os.WriteFile(encryptedPath, encryptRestorePayload(t, []byte(contents), password), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	destination := filepath.Join(destinationDir, fileName)
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	if err := processRestoredFile(fileName, temporary, destinationDir, password, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != contents {
		t.Fatalf("restored contents = %q, want %q", got, contents)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("restored destination mode = %v, want regular 0600", info.Mode())
	}
	targetContents, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetContents) != "original" {
		t.Fatalf("symlink target changed to %q", targetContents)
	}
}
