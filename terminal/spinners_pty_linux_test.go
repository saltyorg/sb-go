//go:build linux

package terminal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const ptySpinnerHelperEnv = "SB_GO_PTY_SPINNER_HELPER"
const ptyFinalTreeHelperEnv = "SB_GO_PTY_FINAL_TREE_HELPER"

func TestFinalTreeDoesNotLeaveLiveFrameInScrollback(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for inline scrollback regression coverage")
	}

	session := fmt.Sprintf("sb-go-spinner-%d", os.Getpid())
	defer exec.Command("tmux", "kill-session", "-t", session).Run() //nolint:errcheck,gosec
	helperCommand := fmt.Sprintf(
		"env %s=1 TERM=xterm-256color %s -test.run=^TestFinalTreePTYHelper$; sleep 1",
		ptyFinalTreeHelperEnv,
		strconv.Quote(os.Args[0]),
	)
	command := helperCommand
	if zsh, err := exec.LookPath("zsh"); err == nil {
		command = fmt.Sprintf("%s -c %s", strconv.Quote(zsh), strconv.Quote(helperCommand))
	}
	if output, err := exec.Command(
		"tmux", "new-session", "-d", "-s", session, "-x", "80", "-y", "12", command,
	).CombinedOutput(); err != nil {
		t.Fatalf("start tmux regression session: %v: %s", err, output)
	}

	var capture []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", "-").Output()
		if err == nil {
			capture = output
			if bytes.Contains(output, []byte("Validated tree")) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	rendered := string(capture)
	if !strings.Contains(rendered, "Validated tree") || !strings.Contains(rendered, "Validated settings") {
		t.Fatalf("final retained tree missing from terminal capture: %q", rendered)
	}
	if !strings.Contains(rendered, "WARNING: Password is shorter than 12 characters (7).") {
		t.Fatalf("task warning missing from terminal capture: %q", rendered)
	}
	for _, stale := range []string{"Validating tree", "Validating accounts", "Validating settings"} {
		if strings.Contains(rendered, stale) {
			t.Fatalf("live frame %q was stranded in scrollback: %q", stale, rendered)
		}
	}
}

func TestFinalTreePTYHelper(t *testing.T) {
	if os.Getenv(ptyFinalTreeHelperEnv) != "1" {
		t.Skip("PTY subprocess helper")
	}
	for i := range 12 {
		fmt.Printf("history %02d\n", i)
	}
	runner := NewRunner(RunnerOptions{})
	err := runner.Run(context.Background(), TaskSpec{
		Running: "Validating tree",
		Success: "Validated tree",
	}, func(ctx context.Context, root *Task) error {
		if err := root.Run(ctx, TaskSpec{
			Running: "Validating accounts",
			Success: "Validated accounts",
		}, func(ctx context.Context, accounts *Task) error {
			time.Sleep(150 * time.Millisecond)
			accounts.Warning("WARNING: Password is shorter than 12 characters (7).")
			return accounts.Run(ctx, TaskSpec{
				Running: "Validating API credentials",
				Success: "API credentials validated",
			}, func(context.Context, *Task) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}); err != nil {
			return err
		}
		return root.Run(ctx, TaskSpec{
			Running: "Validating settings",
			Success: "Validated settings",
		}, func(context.Context, *Task) error {
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFastSpinnerConsumesDelayedTerminalCapabilityResponses(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer master.Close()
	defer slave.Close()

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
