package host

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExtractSpeedtestRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "speedtest.tgz")
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "speedtest", Typeflag: tar.TypeSymlink, Linkname: "/bin/sh"}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractSpeedtest(archivePath, filepath.Join(dir, "speedtest")); err == nil {
		t.Fatal("expected symlink archive entry to be rejected")
	}
}

func TestRunBenchmarkCommandCancelsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	scriptPath := filepath.Join(dir, "bench.sh")
	script := "#!/bin/bash\nset -eu\nsleep 30 &\nchild=$!\nprintf '%s\\n' \"$child\" > \"$PID_FILE\"\nwait \"$child\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runBenchmarkCommand(ctx, scriptPath, append(os.Environ(), "PID_FILE="+pidFile), &bytes.Buffer{}, &bytes.Buffer{})
	}()

	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(pidFile)
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(contents)))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("benchmark child did not start")
	}

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled benchmark returned nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled benchmark did not return")
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(childPID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("benchmark child %d survived cancellation", childPID)
}
