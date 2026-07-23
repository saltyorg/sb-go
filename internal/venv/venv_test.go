package venv

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/saltyorg/sb-go/internal/spinners"
)

func TestRunCommandProvidesTerminalToVenvPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}

	venvPath := filepath.Join(t.TempDir(), "venv")
	if output, err := exec.Command(python, "-m", "venv", venvPath).CombinedOutput(); err != nil {
		t.Fatalf("create test venv: %v\n%s", err, output)
	}

	var output bytes.Buffer
	command := []string{
		filepath.Join(venvPath, "bin", "python"),
		"-c",
		"import sys; print(sys.stdout.isatty()); print(sys.stderr.isatty(), file=sys.stderr)",
	}
	if err := runCommand(context.Background(), command, nil, false, &output, &output); err != nil {
		t.Fatalf("run venv Python with managed output: %v", err)
	}

	if got := strings.ReplaceAll(output.String(), "\r\n", "\n"); got != "True\nTrue\n" {
		t.Fatalf("venv Python did not see a terminal: %q", got)
	}
}

func TestRunCommandPreservesRealPipProgress(t *testing.T) {
	if os.Getenv("SB_REAL_PIP_TEST") == "" {
		t.Skip("set SB_REAL_PIP_TEST=1 to run the networked pip integration test")
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	venvPath := filepath.Join(t.TempDir(), "venv")
	if output, err := exec.Command(python, "-m", "venv", venvPath).CombinedOutput(); err != nil {
		t.Fatalf("create test venv: %v\n%s", err, output)
	}

	var output bytes.Buffer
	command := []string{
		filepath.Join(venvPath, "bin", "python"),
		"-m", "pip", "install",
		"--disable-pip-version-check",
		"--no-cache-dir",
		"--force-reinstall",
		"--progress-bar", "on",
		"ansible-core",
		"requests",
		"rich",
		"docker",
		"cryptography",
		"PyYAML",
		"Jinja2",
		"packaging",
		"setuptools",
		"wheel",
	}
	if err := runCommand(context.Background(), command, os.Environ(), false, &output, &output); err != nil {
		t.Fatalf("run pip in test venv: %v\n%s", err, output.String())
	}

	if !bytes.Contains(output.Bytes(), []byte{'\r'}) {
		t.Fatalf("pip did not emit terminal progress updates: %q", output.String())
	}
	if !bytes.Contains(output.Bytes(), []byte("\x1b[?25l")) {
		t.Fatalf("pip did not enable its terminal progress renderer: %q", output.String())
	}
}

func TestRealPipThroughSpinner(t *testing.T) {
	if os.Getenv("SB_REAL_PIP_SPINNER_TEST") == "" {
		t.Skip("set SB_REAL_PIP_SPINNER_TEST=1 to run the interactive pip spinner test")
	}
	if os.Getenv("SB_REAL_PIP_SPINNER_HELPER") == "" {
		command := exec.Command(os.Args[0], "-test.run", "^TestRealPipThroughSpinner$", "-test.v")
		command.Env = append(os.Environ(), "SB_REAL_PIP_SPINNER_HELPER=1")
		terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 120})
		if err != nil {
			t.Fatalf("start spinner test in terminal: %v", err)
		}
		rendered, _ := io.ReadAll(terminal)
		if err := command.Wait(); err != nil {
			t.Fatalf("spinner test failed: %v\n%s", err, rendered)
		}

		if !bytes.Contains(rendered, []byte("  Collecting wheel")) ||
			!bytes.Contains(rendered, []byte("  Successfully installed")) {
			t.Fatalf("pip output was not rendered with child indentation:\n%q", rendered)
		}
		if !bytes.Contains(rendered, []byte("\x1b[?2026h")) ||
			!bytes.Contains(rendered, []byte("\x1b[?2026l")) {
			t.Fatalf("spinner frames were not rendered atomically:\n%q", rendered)
		}
		finalFrame := regexp.MustCompile(`(?:\x1b\[[0-9]+A|\r)\x1b\[J● Installing test package`)
		if !finalFrame.Match(rendered) {
			t.Fatalf("successful pip output was not collapsed into the final task:\n%q", rendered)
		}
		liveOutputAt := bytes.Index(rendered, []byte("  Collecting wheel"))
		animated := false
		if liveOutputAt >= 0 {
			for _, marker := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
				liveOutput := rendered[liveOutputAt:]
				if bytes.Contains(liveOutput, []byte("\r"+marker+" ")) ||
					bytes.Contains(liveOutput, []byte("A"+marker+" ")) {
					animated = true
					break
				}
			}
		}
		if !animated {
			t.Fatalf("active task stopped animating while pip output was visible:\n%q", rendered)
		}
		return
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	venvPath := filepath.Join(t.TempDir(), "venv")
	if output, err := exec.Command(python, "-m", "venv", venvPath).CombinedOutput(); err != nil {
		t.Fatalf("create test venv: %v\n%s", err, output)
	}

	spinners.SetVerboseMode(false)
	err = spinners.RunTaskWithSpinnerOutputContext(context.Background(), "Installing test package", func(stdout, stderr io.Writer) error {
		command := []string{
			filepath.Join(venvPath, "bin", "python"),
			"-m", "pip", "install",
			"--disable-pip-version-check",
			"--no-cache-dir",
			"--force-reinstall",
			"--progress-bar", "on",
			"ansible-core",
			"requests",
			"rich",
			"docker",
			"cryptography",
			"PyYAML",
			"Jinja2",
			"packaging",
			"setuptools",
			"wheel",
		}
		return runCommand(context.Background(), command, os.Environ(), false, stdout, stderr)
	})
	if err != nil {
		t.Fatalf("run real pip through spinner: %v", err)
	}
}
