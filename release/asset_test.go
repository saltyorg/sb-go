package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadVerified(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	path, err := DownloadVerified(context.Background(), server.Client(), asset, 1024, t.TempDir(), ".asset-*")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestDownloadVerifiedRetriesTransientStatus(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 4 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	path, err := DownloadVerified(t.Context(), server.Client(), asset, 1024, t.TempDir(), ".asset-*")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 4 {
		t.Fatalf("download attempts = %d, want 4", attempts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestDownloadVerifiedRetriesShortBody(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			_, _ = w.Write(payload[:5])
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var delays []time.Duration
	path, err := downloadVerified(t.Context(), server.Client(), asset, 1024, dir, ".asset-*", retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("download attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("retry delays = %v, want [1s]", delays)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Join(dir, entries[0].Name()) != path {
		t.Fatalf("staged files = %v, want only %s", entries, path)
	}
}

func TestDownloadVerifiedRetriesInterruptedBody(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	attempts := 0
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		body := io.Reader(bytes.NewReader(payload))
		if attempts == 1 {
			body = io.MultiReader(bytes.NewReader(payload[:5]), errorReader{})
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(body),
		}, nil
	})}

	asset, err := NewAsset(1, "binary", "https://example.com/binary", int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	var delays []time.Duration
	path, err := downloadVerified(t.Context(), client, asset, 1024, t.TempDir(), ".asset-*", retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("download attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("retry delays = %v, want [1s]", delays)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestDownloadVerifiedDoesNotRetryOverlongBody(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		_, _ = w.Write(append(payload, '!'))
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	var delays []time.Duration
	_, err = downloadVerified(t.Context(), server.Client(), asset, 1024, t.TempDir(), ".asset-*", retryPolicy{
		maxAttempts:   4,
		baseDelay:     time.Second,
		maxHeaderWait: time.Minute,
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		now: time.Now,
	})
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("download error = %v, want size mismatch", err)
	}
	if attempts != 1 {
		t.Fatalf("download attempts = %d, want 1", attempts)
	}
	if len(delays) != 0 {
		t.Fatalf("retry delays = %v, want none", delays)
	}
}

func TestCopyDownloadDoesNotRetryWriterFailure(t *testing.T) {
	wantErr := fmt.Errorf("disk full")
	_, retryable, err := copyDownload(failingWriter{err: wantErr}, sha256.New(), bytes.NewReader([]byte("payload")), 8)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("copyDownload() error = %v, want %v", err, wantErr)
	}
	if retryable {
		t.Fatal("copyDownload() classified a local writer failure as retryable")
	}
}

func TestDownloadVerifiedDoesNotWaitPastBound(t *testing.T) {
	payload := []byte("verified executable")
	digest := sha256.Sum256(payload)
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	asset, err := NewAsset(1, "binary", server.URL, int64(len(payload)), fmt.Sprintf("sha256:%x", digest))
	if err != nil {
		t.Fatal(err)
	}
	_, err = DownloadVerified(t.Context(), server.Client(), asset, 1024, t.TempDir(), ".asset-*")
	if err == nil {
		t.Fatal("DownloadVerified() returned nil error for an excessive retry wait")
	}
	for _, want := range []string{"2m0s", "1m0s"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("DownloadVerified() error = %q, want %q", err, want)
		}
	}
	if attempts != 1 {
		t.Fatalf("download attempts = %d, want 1", attempts)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestDownloadVerifiedRejectsMismatchAndPreservesTarget(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		_, _ = w.Write([]byte("malicious"))
	}))
	defer server.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "binary")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	asset, err := NewAsset(1, "binary", server.URL, int64(len("malicious")), "sha256:"+strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DownloadVerified(context.Background(), server.Client(), asset, 1024, dir, ".asset-*"); err == nil {
		t.Fatal("expected digest mismatch")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("target = %q, want old", got)
	}
	if attempts != 1 {
		t.Fatalf("download attempts = %d, want 1 for a digest mismatch", attempts)
	}
}

func TestNewAssetRejectsMissingDigestAndHTTP(t *testing.T) {
	if _, err := NewAsset(1, "binary", "https://example.com/binary", 1, ""); err == nil {
		t.Fatal("expected missing digest error")
	}
	if _, err := NewAsset(1, "binary", "http://example.com/binary", 1, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected HTTP URL error")
	}
}

func TestHTTPClientRejectsPlaintextRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, "http://example.com/binary", http.StatusFound)
	}))
	defer server.Close()
	client := HTTPClient(time.Second)
	client.Transport = server.Client().Transport
	if _, err := client.Get(server.URL); err == nil {
		t.Fatal("expected plaintext redirect rejection")
	}
}
