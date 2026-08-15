package host

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	assetrelease "github.com/saltyorg/sb-go/release"
	"github.com/saltyorg/sb-go/signals"
)

const (
	speedtestURL    = "https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-linux-x86_64.tgz"
	speedtestSize   = 1106829
	speedtestDigest = "sha256:5690596c54ff9bed63fa3732f818a05dbc2db19ad36ed68f21ca5f64d5cfeeb7"
)

// RunBenchmark prepares the pinned Speedtest executable and runs the reviewed
// benchmark script without allowing the script to download executable code.
func RunBenchmark(ctx context.Context, scriptContents string) error {
	workDir, err := os.MkdirTemp("", "sb-bench-*")
	if err != nil {
		return fmt.Errorf("create benchmark working directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	asset, err := assetrelease.NewAsset(0, "ookla-speedtest-1.2.0-linux-x86_64.tgz", speedtestURL, speedtestSize, speedtestDigest)
	if err != nil {
		return err
	}
	archivePath, err := assetrelease.DownloadVerified(ctx, assetrelease.HTTPClient(30*time.Second), asset, 64<<20, workDir, "speedtest-*.tgz")
	if err != nil {
		return fmt.Errorf("prepare Speedtest: %w", err)
	}
	speedtestPath := filepath.Join(workDir, "speedtest")
	if err := extractSpeedtest(archivePath, speedtestPath); err != nil {
		return fmt.Errorf("prepare Speedtest: %w", err)
	}
	if err := assetrelease.ValidateLinuxAMD64(speedtestPath); err != nil {
		return fmt.Errorf("prepare Speedtest: %w", err)
	}

	scriptPath := filepath.Join(workDir, "bench.sh")
	script, err := os.OpenFile(scriptPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create benchmark script: %w", err)
	}
	if _, err := io.WriteString(script, scriptContents); err != nil {
		_ = script.Close()
		return fmt.Errorf("write benchmark script: %w", err)
	}
	if err := script.Sync(); err != nil {
		_ = script.Close()
		return fmt.Errorf("sync benchmark script: %w", err)
	}
	if err := script.Close(); err != nil {
		return fmt.Errorf("close benchmark script: %w", err)
	}

	err = runBenchmarkCommand(ctx, scriptPath, append(os.Environ(),
		"SB_SPEEDTEST_BIN="+speedtestPath,
		"SB_BENCH_WORKDIR="+workDir,
	), os.Stdout, os.Stderr)
	if err != nil {
		if signals.HandleInterruptError(ctx, err) {
			return fmt.Errorf("benchmark execution interrupted by user")
		}
		return err
	}
	return nil
}

// runBenchmarkCommand places the shell and everything it launches in a
// dedicated process group. Cancelling the command signals the entire group so
// Speedtest, fio, and other grandchildren cannot outlive sb.
func runBenchmarkCommand(ctx context.Context, scriptPath string, env []string, stdout, stderr io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Give the reviewed shell script and its current child a short grace
		// period to handle the same interrupt the user sent to sb.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case err := <-done:
			if err == nil {
				return ctx.Err()
			}
			return err
		case <-timer.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return <-done
		}
	}
}

func extractSpeedtest(archivePath, destination string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = archive.Close() }()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	found := false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Name != "speedtest" {
			continue
		}
		if found {
			return fmt.Errorf("archive contains duplicate speedtest entries")
		}
		if !header.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("speedtest archive entry is not a regular file")
		}
		if header.Size <= 0 || header.Size > 64<<20 {
			return fmt.Errorf("speedtest archive entry has invalid size %d", header.Size)
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			return fmt.Errorf("create Speedtest executable: %w", err)
		}
		written, copyErr := io.Copy(output, io.LimitReader(tarReader, header.Size+1))
		if copyErr == nil && written != header.Size {
			copyErr = fmt.Errorf("entry size mismatch: expected %d bytes, got %d", header.Size, written)
		}
		if copyErr == nil {
			copyErr = output.Sync()
		}
		closeErr := output.Close()
		if copyErr != nil {
			_ = os.Remove(destination)
			return fmt.Errorf("write Speedtest executable: %w", copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(destination)
			return fmt.Errorf("close Speedtest executable: %w", closeErr)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("archive does not contain speedtest")
	}
	return nil
}
