//go:build linux

package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/terminal"
)

const remoteFallbackTmuxHelperEnv = "SB_GO_GIT_FALLBACK_TMUX_HELPER"
const remoteFallbackTmuxComplete = "SB-GO-GIT-FALLBACK-COMPLETE"

func TestRunRemoteCommandShowsHTTP11FallbackChildInTerminal(t *testing.T) {
	if os.Getenv(remoteFallbackTmuxHelperEnv) == "1" {
		runRemoteFallbackTmuxHelper(t)
		return
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for Git fallback terminal coverage")
	}

	session := fmt.Sprintf("sb-go-git-fallback-%d-%d", os.Getpid(), time.Now().UnixNano())
	defer exec.Command("tmux", "kill-session", "-t", session).Run() //nolint:errcheck,gosec
	helperCommand := fmt.Sprintf(
		"env %s=1 TERM=xterm-256color %s -test.run=^TestRunRemoteCommandShowsHTTP11FallbackChildInTerminal$; sleep 2",
		remoteFallbackTmuxHelperEnv,
		strconv.Quote(os.Args[0]),
	)
	if output, err := exec.Command(
		"tmux", "new-session", "-d", "-s", session,
		"-x", "100", "-y", "16", helperCommand,
	).CombinedOutput(); err != nil {
		t.Fatalf("start tmux Git fallback session: %v: %s", err, output)
	}

	rendered := waitForRemoteFallbackTmuxText(t, session, remoteFallbackTmuxComplete)
	fetchLine := terminalLineContaining(t, rendered, "Fetching repository changes")
	retryLine := terminalLineContaining(t, rendered, "Retried Saltbox Git fetch over HTTP/1.1")
	if leadingSpaces(retryLine) <= leadingSpaces(fetchLine) {
		t.Fatalf("fallback task was not nested beneath fetch task: %q", rendered)
	}
	if strings.Contains(rendered, "terminal prompts disabled") {
		t.Fatalf("recovered Git error remained in final terminal output: %q", rendered)
	}
}

func runRemoteFallbackTmuxHelper(t *testing.T) {
	t.Helper()
	if !terminal.IsInteractive() {
		t.Fatal("tmux helper is not attached to an interactive terminal")
	}
	installFakeGit(t, "retry-succeeds")
	runner := terminal.NewRunner(terminal.RunnerOptions{})
	err := runner.Run(context.Background(), terminal.TaskSpec{Running: "Updating Saltbox repository"}, func(ctx context.Context, task *terminal.Task) error {
		return task.RunStreaming(ctx, terminal.TaskSpec{Running: "Fetching repository changes"}, func(taskCtx context.Context, fetchTask *terminal.Task) error {
			_, err := RunRemoteCommand(taskCtx, fetchTask, "Saltbox", t.TempDir(), executor.OutputModeCombined, "fetch", "--progress")
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(remoteFallbackTmuxComplete)
}

func waitForRemoteFallbackTmuxText(t *testing.T, session, want string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var rendered string
	for time.Now().Before(deadline) {
		output, err := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", "-").Output()
		if err == nil {
			rendered = strings.ReplaceAll(string(output), "\r", "")
			if strings.Contains(rendered, want) {
				return rendered
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("tmux session did not render %q before timeout: %q", want, rendered)
	return ""
}

func terminalLineContaining(t *testing.T, output, text string) string {
	t.Helper()
	var match string
	count := 0
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, text) {
			match = line
			count++
		}
	}
	if count != 1 {
		t.Fatalf("terminal output contains %q %d times, want once: %q", text, count, output)
	}
	return match
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
