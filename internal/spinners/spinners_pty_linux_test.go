//go:build linux

package spinners

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const ptySpinnerHelperEnv = "SB_GO_PTY_SPINNER_HELPER"

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
