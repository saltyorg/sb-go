//go:build linux

package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const ptySpinnerHelperEnv = "SB_GO_PTY_SPINNER_HELPER"
const ptyBackgroundLauncherEnv = "SB_GO_PTY_BACKGROUND_LAUNCHER"
const tmuxSpinnerScenarioEnv = "SB_GO_TMUX_SPINNER_SCENARIO"
const tmuxScenarioCompletePrefix = "SB-GO-TMUX-SCENARIO-COMPLETE"

func TestFinalTreeDoesNotLeaveLiveFrameInScrollback(t *testing.T) {
	requireTmux(t)
	rendered := runTmuxSpinnerScenario(t, "retained", 80, 12, "", nil)
	assertTmuxSpinnerCapture(t, rendered,
		[]string{
			"Retained tree validated",
			"Retained accounts validated",
			"Retained API credentials validated",
			"Retained settings validated",
			"WARNING: Password is shorter than 12 characters (7).",
		},
		[]string{
			"Validating retained tree",
			"Validating retained accounts",
			"Validating retained API credentials",
			"Validating retained settings",
		},
	)
}

func TestInlineRendererTerminalLifecycleScenarios(t *testing.T) {
	requireTmux(t)
	t.Run("temporary managed output", func(t *testing.T) {
		rendered := runTmuxSpinnerScenario(t, "transient-output", 80, 12, "", nil)
		assertTmuxSpinnerCapture(t, rendered,
			[]string{"Managed output complete", "Streamed package output"},
			[]string{
				"Processing managed output",
				"Streaming package output",
				"transient managed row",
			},
		)
	})

	t.Run("failure retains diagnostic", func(t *testing.T) {
		rendered := runTmuxSpinnerScenario(t, "failure", 80, 12, "", nil)
		assertTmuxSpinnerCapture(t, rendered,
			[]string{
				"Failed pipeline: Failed",
				"Failed preparation: Failed",
				"Failed package install: Failed",
				"fatal diagnostic: deliberately missing package",
			},
			[]string{
				"Running failing pipeline",
				"Preparing failing operation",
				"Installing deliberately missing package",
				"routine package progress",
			},
		)
	})

	t.Run("ctrl-c cancellation", func(t *testing.T) {
		rendered := runTmuxSpinnerScenario(t, "interrupt", 80, 12,
			"Waiting for terminal interrupt",
			func(t *testing.T, session string) {
				t.Helper()
				if output, err := exec.Command("tmux", "send-keys", "-t", session, "C-c").CombinedOutput(); err != nil {
					t.Fatalf("send Ctrl+C to tmux regression session: %v: %s", err, output)
				}
			},
		)
		assertTmuxSpinnerCapture(t, rendered,
			[]string{"interrupted"},
			[]string{"Waiting for terminal interrupt", "Terminal interrupt: Failed"},
		)
	})

	t.Run("resize live frame", func(t *testing.T) {
		rendered := runTmuxSpinnerScenario(t, "resize", 80, 14,
			"resize transient row 07",
			func(t *testing.T, session string) {
				t.Helper()
				if output, err := exec.Command(
					"tmux", "resize-window", "-t", session, "-x", "72", "-y", "18",
				).CombinedOutput(); err != nil {
					t.Fatalf("resize tmux regression session: %v: %s", err, output)
				}
			},
		)
		assertTmuxSpinnerCapture(t, rendered,
			[]string{"Live tree resized", "Resize output retained"},
			[]string{
				"Resizing live tree",
				"Holding output through resize",
				"resize transient row",
			},
		)
	})
}

func TestInlineRendererTmuxHelper(t *testing.T) {
	scenario := os.Getenv(tmuxSpinnerScenarioEnv)
	if scenario == "" {
		t.Skip("tmux subprocess helper")
	}
	for i := range 16 {
		fmt.Printf("sb-go-history %02d\n", i)
	}

	switch scenario {
	case "retained":
		runRetainedTreeScenario(t)
	case "transient-output":
		runTransientOutputScenario(t)
	case "failure":
		runFailureScenario(t)
	case "interrupt":
		runInterruptScenario(t)
	case "resize":
		runResizeScenario(t)
	default:
		t.Fatalf("unknown tmux spinner scenario %q", scenario)
	}
	fmt.Printf("%s %s\n", tmuxScenarioCompletePrefix, scenario)
}

func runRetainedTreeScenario(t *testing.T) {
	t.Helper()
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Validating retained tree",
		Success: "Retained tree validated",
	}, func(ctx context.Context, root *Task) error {
		if err := root.Run(ctx, TaskSpec{
			Running: "Validating retained accounts",
			Success: "Retained accounts validated",
		}, func(ctx context.Context, accounts *Task) error {
			time.Sleep(150 * time.Millisecond)
			accounts.Warning("WARNING: Password is shorter than 12 characters (7).")
			return accounts.Run(ctx, TaskSpec{
				Running: "Validating retained API credentials",
				Success: "Retained API credentials validated",
			}, func(context.Context, *Task) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}); err != nil {
			return err
		}
		return root.Run(ctx, TaskSpec{
			Running: "Validating retained settings",
			Success: "Retained settings validated",
		}, func(context.Context, *Task) error {
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runTransientOutputScenario(t *testing.T) {
	t.Helper()
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Processing managed output",
		Success: "Managed output complete",
	}, func(ctx context.Context, root *Task) error {
		return root.RunOutput(ctx, TaskSpec{
			Running: "Streaming package output",
			Success: "Streamed package output",
		}, func(_ context.Context, stdout, _ io.Writer) error {
			for i := range 8 {
				if _, err := fmt.Fprintf(stdout, "transient managed row %02d\n", i); err != nil {
					return err
				}
			}
			time.Sleep(250 * time.Millisecond)
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runFailureScenario(t *testing.T) {
	t.Helper()
	wantErr := errors.New("deliberate package failure")
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Running failing pipeline",
		Failure: "Failed pipeline",
	}, func(ctx context.Context, root *Task) error {
		return root.Run(ctx, TaskSpec{
			Running: "Preparing failing operation",
			Failure: "Failed preparation",
		}, func(ctx context.Context, parent *Task) error {
			return parent.RunOutput(ctx, TaskSpec{
				Running: "Installing deliberately missing package",
				Failure: "Failed package install",
			}, func(_ context.Context, stdout, stderr io.Writer) error {
				if _, err := fmt.Fprintln(stdout, "routine package progress"); err != nil {
					return err
				}
				time.Sleep(125 * time.Millisecond)
				if _, err := fmt.Fprintln(stderr, "fatal diagnostic: deliberately missing package"); err != nil {
					return err
				}
				time.Sleep(125 * time.Millisecond)
				return wantErr
			})
		})
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("failure scenario error = %v, want %v", err, wantErr)
	}
}

func runInterruptScenario(t *testing.T) {
	t.Helper()
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Waiting for terminal interrupt",
		Failure: "Terminal interrupt",
	}, func(ctx context.Context, _ *Task) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupt scenario error = %v, want context cancellation", err)
	}
}

func runResizeScenario(t *testing.T) {
	t.Helper()
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Resizing live tree",
		Success: "Live tree resized",
	}, func(ctx context.Context, root *Task) error {
		return root.RunOutput(ctx, TaskSpec{
			Running: "Holding output through resize",
			Success: "Resize output retained",
		}, func(_ context.Context, stdout, _ io.Writer) error {
			for i := range 8 {
				if _, err := fmt.Fprintf(stdout, "resize transient row %02d\n", i); err != nil {
					return err
				}
			}
			time.Sleep(750 * time.Millisecond)
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for inline scrollback regression coverage")
	}
}

func runTmuxSpinnerScenario(
	t *testing.T,
	scenario string,
	width, height int,
	waitFor string,
	action func(*testing.T, string),
) string {
	t.Helper()
	session := fmt.Sprintf("sb-go-spinner-%d-%d", os.Getpid(), time.Now().UnixNano())
	defer exec.Command("tmux", "kill-session", "-t", session).Run() //nolint:errcheck,gosec
	helperCommand := fmt.Sprintf(
		"env %s=%s TERM=xterm-256color %s -test.run=^TestInlineRendererTmuxHelper$; sleep 2",
		tmuxSpinnerScenarioEnv,
		strconv.Quote(scenario),
		strconv.Quote(os.Args[0]),
	)
	command := helperCommand
	if zsh, err := exec.LookPath("zsh"); err == nil {
		command = fmt.Sprintf("%s -c %s", strconv.Quote(zsh), strconv.Quote(helperCommand))
	}
	if output, err := exec.Command(
		"tmux", "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height), command,
	).CombinedOutput(); err != nil {
		t.Fatalf("start tmux regression session: %v: %s", err, output)
	}

	if waitFor != "" {
		waitForTmuxText(t, session, waitFor, 3*time.Second)
		if action == nil {
			t.Fatalf("tmux scenario %q has a wait condition without an action", scenario)
		}
		action(t, session)
	}
	completion := tmuxScenarioCompletePrefix + " " + scenario
	return waitForTmuxText(t, session, completion, 5*time.Second)
}

func waitForTmuxText(t *testing.T, session, want string, timeout time.Duration) string {
	t.Helper()
	var rendered string
	deadline := time.Now().Add(timeout)
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

func assertTmuxSpinnerCapture(t *testing.T, rendered string, once, absent []string) {
	t.Helper()
	for _, history := range []string{"sb-go-history 00", "sb-go-history 15"} {
		if count := strings.Count(rendered, history); count != 1 {
			t.Fatalf("terminal history row %q appeared %d times, want once: %q", history, count, rendered)
		}
	}
	for _, text := range once {
		if count := strings.Count(rendered, text); count != 1 {
			t.Fatalf("final terminal row %q appeared %d times, want once: %q", text, count, rendered)
		}
	}
	for _, text := range absent {
		if strings.Contains(rendered, text) {
			t.Fatalf("transient terminal row %q was stranded in scrollback: %q", text, rendered)
		}
	}
	for _, char := range rendered {
		if char >= '\u2800' && char <= '\u28ff' {
			t.Fatalf("spinner glyph %q was stranded in scrollback: %q", char, rendered)
		}
	}
}

func TestFastSpinnerConsumesDelayedTerminalCapabilityResponses(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer func() {
		if err := master.Close(); err != nil {
			t.Errorf("close pty master: %v", err)
		}
	}()
	defer func() {
		if err := slave.Close(); err != nil {
			t.Errorf("close pty slave: %v", err)
		}
	}()

	cmd := exec.Command(os.Args[0], "-test.run=^TestFastSpinnerPTYHelper$")
	cmd.Env = append(os.Environ(),
		ptySpinnerHelperEnv+"=1",
		"TERM=xterm-ghostty",
		"TERM_PROGRAM=ghostty",
	)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start pty helper: %v", err)
	}

	query := []byte("\x1b[?2026$p\x1b[?2027$p")
	responses := []byte("\x1b[?2026;2$y\x1b[?2027;1$y")
	replied := make(chan error, 1)
	go func() {
		var output bytes.Buffer
		buffer := make([]byte, 4096)
		for {
			n, readErr := master.Read(buffer)
			if n > 0 {
				output.Write(buffer[:n])
				if bytes.Contains(output.Bytes(), query) {
					time.Sleep(75 * time.Millisecond)
					_, writeErr := master.Write(responses)
					replied <- writeErr
					return
				}
			}
			if readErr != nil {
				replied <- readErr
				return
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("pty helper failed: %v", err)
	}
	select {
	case err := <-replied:
		if err != nil {
			t.Fatalf("reply to terminal capability query: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("spinner did not emit terminal capability queries")
	}

	if _, err := term.MakeRaw(int(slave.Fd())); err != nil {
		t.Fatalf("make retained pty raw: %v", err)
	}
	pending, err := unix.IoctlGetInt(int(slave.Fd()), unix.TIOCINQ)
	if err != nil {
		t.Fatalf("inspect retained pty input: %v", err)
	}
	if pending != 0 {
		t.Fatalf("%d terminal response bytes leaked past spinner shutdown", pending)
	}
}

// TestSpinnerCompletesInBackgroundProcessGroup reproduces `curl ... | sudo bash`
// under sudo-rs, which leaves a piped command in a background process group on
// its pseudo-terminal and never resumes it after a SIGTTOU stop.
func TestSpinnerCompletesInBackgroundProcessGroup(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer func() {
		if err := master.Close(); err != nil {
			t.Errorf("close pty master: %v", err)
		}
	}()
	defer func() {
		if err := slave.Close(); err != nil {
			t.Errorf("close pty slave: %v", err)
		}
	}()
	var outputMu sync.Mutex
	var output bytes.Buffer
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := master.Read(buffer)
			outputMu.Lock()
			output.Write(buffer[:n])
			outputMu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	capturedOutput := func() string {
		outputMu.Lock()
		defer outputMu.Unlock()
		return output.String()
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestBackgroundProcessGroupLauncher$")
	cmd.Env = append(os.Environ(), ptyBackgroundLauncherEnv+"=1", "TERM=xterm-256color")
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start background launcher: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("spinner in background process group failed: %v\n%s", err, capturedOutput())
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("background launcher timed out: %v\n%s", <-done, capturedOutput())
	}
}

// TestBackgroundProcessGroupLauncher holds the pty foreground and runs the
// spinner helper in its own background process group with piped stdin.
func TestBackgroundProcessGroupLauncher(t *testing.T) {
	if os.Getenv(ptyBackgroundLauncherEnv) != "1" {
		t.Skip("PTY background process group launcher")
	}
	stdin, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	defer func() { _ = stdinWriter.Close() }()

	cmd := exec.Command(os.Args[0], "-test.run=^TestFastSpinnerPTYHelper$")
	cmd.Env = append(os.Environ(), ptySpinnerHelperEnv+"=1")
	cmd.Stdin = stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start spinner helper: %v", err)
	}
	_ = stdin.Close()

	foreground, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	if err != nil {
		t.Fatalf("read pty foreground process group: %v", err)
	}
	if foreground == cmd.Process.Pid {
		t.Fatal("spinner helper unexpectedly owns the pty foreground")
	}

	var status syscall.WaitStatus
	if _, err := syscall.Wait4(cmd.Process.Pid, &status, syscall.WUNTRACED, nil); err != nil {
		t.Fatalf("wait for spinner helper: %v", err)
	}
	if status.Stopped() {
		stopSignal := status.StopSignal()
		_ = cmd.Process.Kill()
		_, _ = syscall.Wait4(cmd.Process.Pid, &status, 0, nil)
		t.Fatalf("spinner helper was stopped by %v in a background process group", stopSignal)
	}
	if !status.Exited() || status.ExitStatus() != 0 {
		t.Fatalf("spinner helper did not exit cleanly: %v", status)
	}
}

func TestFastSpinnerPTYHelper(t *testing.T) {
	if os.Getenv(ptySpinnerHelperEnv) != "1" {
		t.Skip("PTY subprocess helper")
	}
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{Running: "fast"}, func(context.Context, *Task) error {
		return nil
	})
	if err != nil {
		t.Fatalf("run fast spinner: %v", err)
	}
}
