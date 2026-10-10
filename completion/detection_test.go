package completion

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFishMetadataUsesReportedConfigurationDirectory(t *testing.T) {
	for _, output := range []string{"3.3.1\n/etc/fish\n", "3.7.0\n/usr/local/etc/fish\n", "4.2.1\n/opt/fish/etc/fish\n"} {
		detection, err := fishMetadata(output)
		lines := strings.Split(output, "\n")
		if err != nil || !detection.Present || detection.Reason != "" || detection.Directory != filepath.Join(lines[1], "completions") {
			t.Fatalf("valid metadata rejected: %+v %v", detection, err)
		}
	}
	for _, output := range []string{"3.7.0\n", "3.7.0\n\n", "3.7.0\netc/fish\n", "3.7.0\n/etc/../fish\n", "3.7.0\n/\n", "3.7.0\n/etc/fish\nstartup output\n", "not-a-version\n/etc/fish\n"} {
		if _, err := fishMetadata(output); err == nil {
			t.Fatalf("invalid metadata accepted: %q", output)
		}
	}
	if detection, err := fishMetadata("3.2.2\n/etc/fish\n"); err != nil || detection.Reason == "" {
		t.Fatalf("unsupported version not distinguished: %+v %v", detection, err)
	}
}

func TestProbeUsesCleanStartupAndHonorsTimeout(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not available")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "startup-ran")
	startup := filepath.Join(root, "startup.bash")
	if err := os.WriteFile(startup, []byte("touch \"$COMPLETION_STARTUP_MARKER\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASH_ENV", startup)
	t.Setenv("ENV", startup)
	t.Setenv("COMPLETION_STARTUP_MARKER", marker)
	t.Setenv("COMPLETION_SECRET", "private-user-value")
	output, err := runProbe(context.Background(), bash, "--noprofile", "--norc", "-c", `printf '%s\n' "${COMPLETION_SECRET-unset}"`)
	if err != nil || output != "unset\n" {
		t.Fatalf("probe inherited user environment: %q %v", output, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe evaluated user startup: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = runProbe(ctx, bash, "--noprofile", "--norc", "-c", "while :; do :; done")
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("probe did not obey timeout: %v", err)
	}
}

func TestDetectionUsesInstalledSystemShell(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bash"), []byte("#!/bin/sh\nexit 42\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	m := NewManager()
	detection, err := m.detect(context.Background(), Bash)
	if err != nil || !detection.Present {
		t.Fatalf("inherited PATH replaced installed system shell: %+v %v", detection, err)
	}
}

func TestInstalledShellProbes(t *testing.T) {
	for _, shell := range []Shell{Bash, Zsh} {
		t.Run(string(shell), func(t *testing.T) {
			m := NewManager()
			executable, err := m.shellExecutable(string(shell))
			if err != nil {
				t.Fatal(err)
			}
			if executable == "" {
				t.Skip("shell is not installed")
			}
			detection, err := m.detect(context.Background(), shell)
			if err != nil || !detection.Present {
				t.Fatalf("installed shell probe failed: %+v %v", detection, err)
			}
			if shell == Zsh {
				m.Paths.ZshDir = "/sb-go-completion-not-in-the-function-path"
				detection, err = m.detect(context.Background(), shell)
				if err != nil || !strings.Contains(detection.Reason, "function path") {
					t.Fatalf("inactive custom target was accepted: %+v %v", detection, err)
				}
			}
		})
	}
}

func TestShellExecutableMissing(t *testing.T) {
	executable, err := NewManager().shellExecutable("sb-go-missing-shell-for-test")
	if err != nil || executable != "" {
		t.Fatalf("missing shell was detected: %q %v", executable, err)
	}
}

func TestFishRuntimeProbe(t *testing.T) {
	executable := os.Getenv("SB_COMPLETION_FISH")
	if executable == "" {
		var err error
		executable, err = exec.LookPath("fish")
		if err != nil {
			t.Skip("Fish runtime is not installed; set SB_COMPLETION_FISH for an isolated runtime")
		}
	}
	output, err := runProbe(context.Background(), executable, "--no-config", "-c", fishProbeCommand)
	if err != nil {
		t.Fatal(err)
	}
	detection, err := fishMetadata(output)
	if err != nil || !detection.Present || detection.Reason != "" || !filepath.IsAbs(detection.Directory) {
		t.Fatalf("real Fish metadata rejected: %q %+v %v", output, detection, err)
	}
}

func TestLiveExecutableCheckIncludesRunningBinaryDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	live, err := executableNameInUse(filepath.Base(executable))
	if err != nil || !live {
		t.Fatalf("running binary outside standard bin directories was missed: %v live=%v", err, live)
	}
}
