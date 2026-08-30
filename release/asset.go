package release

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Asset describes an executable release artifact reported by the GitHub API.
type Asset struct {
	ID       int64
	Name     string
	URL      string
	Size     int64
	Digest   string
	Expected [sha256.Size]byte
}

// ValidateLinuxAMD64 verifies that path is a 64-bit x86 ELF executable.
func ValidateLinuxAMD64(path string) error {
	binary, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open ELF executable: %w", err)
	}
	defer func() { _ = binary.Close() }()
	if binary.Class != elf.ELFCLASS64 || binary.Machine != elf.EM_X86_64 {
		return fmt.Errorf("executable is not a 64-bit x86 ELF binary")
	}
	return nil
}

// NewAsset validates release metadata before any artifact is downloaded.
func NewAsset(id int64, name, downloadURL string, size int64, digest string) (Asset, error) {
	if strings.TrimSpace(name) == "" {
		return Asset{}, fmt.Errorf("release asset is missing a name")
	}
	if size <= 0 {
		return Asset{}, fmt.Errorf("release asset %q has invalid size %d", name, size)
	}
	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return Asset{}, fmt.Errorf("release asset %q has invalid URL: %w", name, err)
	}
	if parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return Asset{}, fmt.Errorf("release asset %q must use an absolute HTTPS URL", name)
	}

	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || algorithm != "sha256" {
		return Asset{}, fmt.Errorf("release asset %q is missing a SHA-256 digest", name)
	}
	digestBytes, err := hex.DecodeString(encoded)
	if err != nil || len(digestBytes) != sha256.Size {
		return Asset{}, fmt.Errorf("release asset %q has an invalid SHA-256 digest", name)
	}

	asset := Asset{ID: id, Name: name, URL: downloadURL, Size: size, Digest: digest}
	copy(asset.Expected[:], digestBytes)
	return asset, nil
}

// HTTPClient returns a client that refuses redirects to plaintext HTTP.
func HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("release asset redirect must use HTTPS")
			}
			return nil
		},
	}
}

// DownloadVerified retries transient download failures into fresh staging files,
// verifies the exact size and digest, and returns the staged path. The caller owns the returned file.
func DownloadVerified(ctx context.Context, client *http.Client, asset Asset, maxSize int64, dir, pattern string) (path string, err error) {
	return downloadVerified(ctx, client, asset, maxSize, dir, pattern, defaultRetryPolicy())
}

func downloadVerified(ctx context.Context, client *http.Client, asset Asset, maxSize int64, dir, pattern string, policy retryPolicy) (string, error) {
	if client == nil {
		return "", fmt.Errorf("release asset HTTP client is required")
	}
	if asset.Size > maxSize {
		return "", fmt.Errorf("release asset %q exceeds the %d byte limit", asset.Name, maxSize)
	}
	policy = normalizeRetryPolicy(policy)
	var lastErr error
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		path, headers, retryable, err := downloadVerifiedAttempt(ctx, client, asset, dir, pattern)
		if err == nil {
			return path, nil
		}
		lastErr = err
		if !retryable || attempt == policy.maxAttempts || ctx.Err() != nil {
			break
		}

		var response *http.Response
		if headers != nil {
			response = &http.Response{Header: headers}
		}
		delay, retry := retryDelay(response, attempt, policy)
		if !retry {
			return "", fmt.Errorf("%w; retry wait %s exceeds %s limit", lastErr, delay, policy.maxHeaderWait)
		}
		if err := policy.wait(ctx, delay); err != nil {
			return "", err
		}
	}
	return "", lastErr
}

func downloadVerifiedAttempt(ctx context.Context, client *http.Client, asset Asset, dir, pattern string) (path string, headers http.Header, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", nil, false, fmt.Errorf("create release asset request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, true, fmt.Errorf("download release asset %q: %w", asset.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", resp.Header.Clone(), isRetryableHTTPStatus(resp.StatusCode), fmt.Errorf("download release asset %q: unexpected HTTP status %s", asset.Name, resp.Status)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", nil, false, fmt.Errorf("create release asset staging directory: %w", err)
	}
	staged, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, false, fmt.Errorf("create staged release asset: %w", err)
	}
	path = staged.Name()
	stagedPath := path
	keep := false
	defer func() {
		if !keep {
			_ = staged.Close()
			_ = os.Remove(stagedPath)
		}
	}()

	hash := sha256.New()
	written, retryable, err := copyDownload(staged, hash, resp.Body, asset.Size+1)
	if err != nil {
		var headers http.Header
		if retryable {
			headers = resp.Header.Clone()
		}
		return "", headers, retryable, fmt.Errorf("copy release asset %q into staging: %w", asset.Name, err)
	}
	if written != asset.Size {
		if written < asset.Size {
			return "", resp.Header.Clone(), true, fmt.Errorf("release asset %q size mismatch: expected %d bytes, got %d", asset.Name, asset.Size, written)
		}
		return "", nil, false, fmt.Errorf("release asset %q size mismatch: expected %d bytes, got %d", asset.Name, asset.Size, written)
	}
	if !equalDigest(hash.Sum(nil), asset.Expected[:]) {
		return "", nil, false, fmt.Errorf("release asset %q SHA-256 mismatch", asset.Name)
	}
	if err := staged.Sync(); err != nil {
		return "", nil, false, fmt.Errorf("sync staged release asset %q: %w", asset.Name, err)
	}
	if err := staged.Close(); err != nil {
		return "", nil, false, fmt.Errorf("close staged release asset %q: %w", asset.Name, err)
	}
	keep = true
	return path, nil, false, nil
}

func copyDownload(destination, digest io.Writer, source io.Reader, maxBytes int64) (written int64, retryable bool, err error) {
	reader := io.LimitReader(source, maxBytes)
	buffer := make([]byte, 32<<10)
	for {
		read, readErr := reader.Read(buffer)
		if read > 0 {
			if err := writeDownloadChunk(destination, buffer[:read]); err != nil {
				return written, false, fmt.Errorf("write staged file: %w", err)
			}
			if err := writeDownloadChunk(digest, buffer[:read]); err != nil {
				return written, false, fmt.Errorf("hash staged file: %w", err)
			}
			written += int64(read)
		}
		switch {
		case readErr == io.EOF:
			return written, false, nil
		case readErr != nil:
			return written, true, fmt.Errorf("read response body: %w", readErr)
		}
	}
}

func writeDownloadChunk(writer io.Writer, chunk []byte) error {
	written, err := writer.Write(chunk)
	if err != nil {
		return err
	}
	if written != len(chunk) {
		return io.ErrShortWrite
	}
	return nil
}

// Activate atomically replaces target with an already verified staged file.
// staged and target must be in the same directory.
func Activate(staged, target string, mode os.FileMode) error {
	if filepath.Dir(staged) != filepath.Dir(target) {
		return fmt.Errorf("staged release asset must be in the target directory")
	}
	if err := os.Chmod(staged, mode); err != nil {
		return fmt.Errorf("set staged release asset permissions: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("activate release asset: %w", err)
	}
	dir, err := os.Open(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("open release asset directory for sync: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync release asset directory: %w", err)
	}
	return nil
}

func equalDigest(actual, expected []byte) bool {
	if len(actual) != len(expected) {
		return false
	}
	var mismatch byte
	for i := range actual {
		mismatch |= actual[i] ^ expected[i]
	}
	return mismatch == 0
}
