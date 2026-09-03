package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/terminal"
)

func TestRunRemoteCommandRetriesGitHubAuthenticationOverHTTP11(t *testing.T) {
	callLog := installFakeGit(t, "retry-succeeds")

	_, err := RunRemoteCommand(t.Context(), nil, "Saltbox", t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
	if err != nil {
		t.Fatalf("RunRemoteCommand() error = %v", err)
	}

	assertGitCalls(t, callLog, []string{
		"fetch --progress",
		"-c http.version=HTTP/1.1 fetch --progress",
	})
}

func TestRunRemoteCommandShowsHTTP11FallbackTask(t *testing.T) {
	installFakeGit(t, "retry-succeeds")
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})

	err := runner.Run(t.Context(), terminal.TaskSpec{Running: "Updating Saltbox repository"}, func(ctx context.Context, task *terminal.Task) error {
		return task.RunStreaming(ctx, terminal.TaskSpec{Running: "Fetching repository changes"}, func(taskCtx context.Context, fetchTask *terminal.Task) error {
			_, err := RunRemoteCommand(taskCtx, fetchTask, "Saltbox", t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
			return err
		})
	})
	if err != nil {
		t.Fatalf("RunRemoteCommand() error = %v", err)
	}
	for _, message := range []string{
		"Retrying Saltbox Git fetch over HTTP/1.1...",
		"Retried Saltbox Git fetch over HTTP/1.1",
	} {
		if strings.Count(output.String(), message) != 1 {
			t.Fatalf("fallback task lifecycle missing from output:\n%s", output.String())
		}
	}
}

func TestRunRemoteCommandReturnsRepositorySpecificGitHubError(t *testing.T) {
	for _, repository := range []string{"Saltbox", "Sandbox"} {
		t.Run(repository, func(t *testing.T) {
			callLog := installFakeGit(t, "always-auth")

			_, err := RunRemoteCommand(t.Context(), nil, repository, t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
			if err == nil {
				t.Fatal("RunRemoteCommand() error = nil, want authentication error")
			}
			want := "GitHub unexpectedly requested authentication for the public " + repository + " repository"
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("RunRemoteCommand() error = %q, want substring %q", err, want)
			}
			if !strings.Contains(err.Error(), "retry using HTTP/1.1 also failed") {
				t.Fatalf("RunRemoteCommand() error does not explain the failed fallback: %q", err)
			}
			if !strings.Contains(err.Error(), "Git details:\nfatal: could not read Username") {
				t.Fatalf("RunRemoteCommand() error does not retain Git details: %q", err)
			}

			assertGitCalls(t, callLog, []string{
				"fetch --progress",
				"-c http.version=HTTP/1.1 fetch --progress",
			})
		})
	}
}

func TestRunRemoteCommandPreservesUnknownGitError(t *testing.T) {
	callLog := installFakeGit(t, "unknown-failure")

	_, err := RunRemoteCommand(t.Context(), nil, "Saltbox", t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
	if err == nil {
		t.Fatal("RunRemoteCommand() error = nil, want Git error")
	}
	const rawError = "fatal: unable to access repository: connection reset"
	if !strings.Contains(err.Error(), rawError) {
		t.Fatalf("RunRemoteCommand() error = %q, want raw Git error %q", err, rawError)
	}
	if strings.Contains(err.Error(), "unexpectedly requested authentication") {
		t.Fatalf("RunRemoteCommand() misclassified unknown Git error: %q", err)
	}

	assertGitCalls(t, callLog, []string{"fetch --progress"})
}

func TestRunRemoteCommandDoesNotRetrySuccess(t *testing.T) {
	callLog := installFakeGit(t, "success")

	_, err := RunRemoteCommand(t.Context(), nil, "Saltbox", t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
	if err != nil {
		t.Fatalf("RunRemoteCommand() error = %v", err)
	}

	assertGitCalls(t, callLog, []string{"fetch --progress"})
}

func TestGitHubAuthenticationDiagnosticCompatibility(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("git is required for compatibility testing")
	}
	version, err := exec.CommandContext(t.Context(), gitPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("read Git version: %v", err)
	}
	t.Log(strings.TrimSpace(string(version)))

	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_ASKPASS", "/bin/false")
	t.Setenv("SSH_ASKPASS", "/bin/false")
	result, err := executor.Run(t.Context(), gitPath,
		executor.WithArgs("-c", "credential.helper=", "credential", "fill"),
		executor.WithInheritEnv(
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
		),
		executor.WithStdin(strings.NewReader("protocol=https\nhost=github.com\n\n")),
	)
	if err == nil {
		t.Fatal("Git credential lookup succeeded without credentials")
	}
	if !isGitHubAuthenticationFailure(result) {
		t.Fatalf("installed Git authentication diagnostic was not recognized:\n%s", resultOutput(result))
	}
}

func TestCloneRepositoryUsesRemoteGitRetry(t *testing.T) {
	callLog := installFakeGit(t, "retry-succeeds")
	destination := filepath.Join(t.TempDir(), "saltbox")

	err := CloneRepository(
		t.Context(),
		nil,
		"Saltbox",
		"https://github.com/saltyorg/saltbox.git",
		destination,
		"master",
		false,
	)
	if err != nil {
		t.Fatalf("CloneRepository() error = %v", err)
	}

	assertGitCalls(t, callLog, []string{
		"clone --progress --depth 1 -b master https://github.com/saltyorg/saltbox.git " + destination,
		"-c http.version=HTTP/1.1 clone --progress --depth 1 -b master https://github.com/saltyorg/saltbox.git " + destination,
	})
}

func TestFetchAndResetBranchUsesRemoteGitRetry(t *testing.T) {
	callLog := installFakeGit(t, "remote-retry-succeeds")
	repositoryPath := t.TempDir()
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})

	err := runner.Run(t.Context(), terminal.TaskSpec{Running: "test"}, func(ctx context.Context, task *terminal.Task) error {
		return FetchAndResetBranch(ctx, task, repositoryPath, "master", "root", nil, "Saltbox")
	})
	if err != nil {
		t.Fatalf("FetchAndResetBranch() error = %v", err)
	}
	for _, message := range []string{
		"Retried Saltbox Git fetch over HTTP/1.1",
		"Retried Saltbox Git submodule update over HTTP/1.1",
	} {
		if !strings.Contains(output.String(), message) {
			t.Fatalf("fallback task %q missing from output:\n%s", message, output.String())
		}
	}

	assertGitCalls(t, callLog, []string{
		"fetch --progress",
		"-c http.version=HTTP/1.1 fetch --progress",
		"clean --quiet -df",
		"reset --quiet --hard @{u}",
		"checkout --quiet master",
		"clean --quiet -df",
		"reset --quiet --hard @{u}",
		"submodule update --progress --init --recursive",
		"-c http.version=HTTP/1.1 submodule update --progress --init --recursive",
	})
}

func installFakeGit(t *testing.T, behavior string) string {
	t.Helper()
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	callLog := filepath.Join(dir, "calls.log")
	const script = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$SB_GIT_TEST_CALL_LOG"
case "$SB_GIT_TEST_BEHAVIOR" in
  success)
    exit 0
    ;;
  retry-succeeds)
    if [ "${1:-}" = "-c" ] && [ "${2:-}" = "http.version=HTTP/1.1" ]; then
      exit 0
    fi
    ;;
  remote-retry-succeeds)
    if [ "${1:-}" = "-c" ] && [ "${2:-}" = "http.version=HTTP/1.1" ]; then
      exit 0
    fi
    if [ "${1:-}" != "fetch" ] && [ "${1:-}" != "submodule" ]; then
      exit 0
    fi
    ;;
  unknown-failure)
    printf '%s\n' 'fatal: unable to access repository: connection reset' >&2
    exit 128
    ;;
  always-auth)
    ;;
  *)
    printf 'unknown test behavior: %s\n' "$SB_GIT_TEST_BEHAVIOR" >&2
    exit 2
    ;;
esac
printf '%s\n' "fatal: could not read Username for 'https://github.com': terminal prompts disabled" >&2
printf '%s\n' 'fatal: expected flush after ref listing' >&2
exit 128
`
	if err := os.WriteFile(gitPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake Git executable: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SB_GIT_TEST_CALL_LOG", callLog)
	t.Setenv("SB_GIT_TEST_BEHAVIOR", behavior)
	return callLog
}

func assertGitCalls(t *testing.T, path string, want []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake Git call log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Git calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
