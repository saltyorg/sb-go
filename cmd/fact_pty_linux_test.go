//go:build linux

package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/saltyorg/sb-go/facts"
	"github.com/saltyorg/sb-go/factui"
	"github.com/saltyorg/sb-go/layout"
	"golang.org/x/term"
)

func TestFactEditorCobraStreams(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFactFixture(t, master)
	defer closeFactFixture(t, slave)
	wantErr := errors.New("runner result")
	ctx := t.Context()
	opened, ran := false, false
	var openedSession *facts.Session
	command := newFactEditorCommand(func(path string) (*facts.Session, error) {
		opened = true
		if path != layout.Current().SaltboxFactsPath {
			t.Fatalf("facts path = %q", path)
		}
		openedSession, err = facts.OpenSession(t.TempDir())
		return openedSession, err
	}, func(got context.Context, session factui.Session, in io.Reader, out io.Writer) error {
		ran = true
		if got != ctx || in != slave || out != slave || session != openedSession {
			t.Error("runner did not receive Cobra context, streams, and opened session")
		}
		return errors.Join(wantErr, session.Close())
	})
	command.SetIn(slave)
	command.SetOut(slave)
	command.SetErr(io.Discard)
	command.SetArgs(nil)
	if err := command.ExecuteContext(ctx); !errors.Is(err, wantErr) || !opened || !ran {
		t.Fatalf("execute: %v, opened=%v, ran=%v", err, opened, ran)
	}
}

func TestFactEditorRejectsEitherNonTTYBeforeOpening(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFactFixture(t, master)
	defer closeFactFixture(t, slave)
	file, err := os.CreateTemp(t.TempDir(), "regular")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFactFixture(t, file)
	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
	}{
		{"piped input", strings.NewReader(""), slave},
		{"piped output", slave, io.Discard},
		{"regular input descriptor", file, slave},
		{"regular output descriptor", slave, file},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := newFactEditorCommand(func(string) (*facts.Session, error) {
				t.Fatal("opened a session before rejecting non-TTY streams")
				return nil, nil
			}, nil)
			command.SetIn(tc.in)
			command.SetOut(tc.out)
			command.SetErr(io.Discard)
			command.SetArgs(nil)
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
				t.Fatalf("non-TTY error = %v", err)
			}
		})
	}
}

const factPTYEnv = "SB_GO_FACT_PTY_FIXTURE"
const factPTYOriginal = "[default]\npassword = plaintext-secret\ntoken = keep\n[second]\nname = extra\n"

func TestFactEditorPTY(t *testing.T) {
	t.Run("edit delete save return apply", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("xjj", "plaintext-secret")
		p.send("e", "EDIT FACT VALUE")
		p.send("\x01\x0bupdated-secret\r", "staged edit", "updated-secret")
		p.send("jd", "staged deletion")
		p.send("s", "REVIEW 2 PENDING CHANGE(S)", "Before: plaintext-secret", "After:  updated-secret", "Delete fact plex / default / token")
		p.unchanged()
		p.send("r", "◆ Saltbox Facts")
		p.send("s", "REVIEW 2 PENDING CHANGE(S)")
		p.send("a", "◆ Saltbox Facts")
		p.send("q")
		p.finish()
		data, err := os.ReadFile(p.path)
		if err != nil || !bytes.Contains(data, []byte("updated-secret")) || bytes.Contains(data, []byte("token")) {
			t.Fatalf("applied facts = %q, %v", data, err)
		}
	})
	t.Run("save discard keeps editor open", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("d", "staged deletion")
		p.send("s", "REVIEW 1 PENDING CHANGE(S)", "Delete instance default", "Delete instance second")
		p.send("d", "◆ Saltbox Facts")
		p.send("\x03")
		p.finish()
		p.unchanged()
	})
	t.Run("ctrl-c reviews dirty exit and discards", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("d", "staged deletion")
		p.send("\x03", "REVIEW 1 PENDING CHANGE(S) BEFORE EXITING", "Discard-and-exit")
		p.unchanged()
		p.send("d")
		p.finish()
		p.unchanged()
	})
	t.Run("q reviews dirty exit and applies", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("d", "staged deletion")
		p.send("q", "REVIEW 1 PENDING CHANGE(S) BEFORE EXITING", "Apply-and-exit")
		p.unchanged()
		p.send("a")
		p.finish()
		if _, err := os.Stat(p.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("role still exists after Apply-and-exit: %v", err)
		}
	})
	t.Run("sigterm restores terminal without writing", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("d", "staged deletion")
		if err := p.command.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		p.finish()
		p.unchanged()
	})
	t.Run("mouse context deletion can be discarded", func(t *testing.T) {
		p := startFactPTY(t)
		p.send("\x1b[<2;11;8M", "ACTIONS", "Stage fact deletion")
		p.send("\x1b[<0;16;13M", "staged deletion")
		p.send("s", "REVIEW 1 PENDING CHANGE(S)", "Delete fact plex / default / token")
		p.send("d", "◆ Saltbox Facts")
		p.send("q")
		p.finish()
		p.unchanged()
	})
}

func TestFactEditorPTYHelper(t *testing.T) {
	dir := os.Getenv(factPTYEnv)
	if dir == "" {
		t.Skip("PTY subprocess helper")
	}
	ctx, cancel := signal.NotifyContext(t.Context(), syscall.SIGTERM)
	defer cancel()
	command := newFactEditorCommand(func(string) (*facts.Session, error) {
		return facts.OpenSession(dir)
	}, factui.Run)
	command.SetIn(os.Stdin)
	command.SetOut(os.Stdout)
	command.SetErr(os.Stderr)
	command.SetArgs(nil)
	if err := command.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
}

type factPTY struct {
	t             *testing.T
	master, slave *os.File
	command       *exec.Cmd
	state         *term.State
	path          string
	chunks        chan string
	done          chan error
	output        string
}

func startFactPTY(t *testing.T) *factPTY {
	t.Helper()
	dir := t.TempDir()
	p := &factPTY{t: t, path: filepath.Join(dir, "plex.ini"), chunks: make(chan string, 128), done: make(chan error, 1)}
	if err := os.WriteFile(p.path, []byte(factPTYOriginal), 0640); err != nil {
		t.Fatal(err)
	}
	var err error
	p.master, p.slave, err = pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeFactFixture(t, p.master)
		closeFactFixture(t, p.slave)
	})
	if err := pty.Setsize(p.master, &pty.Winsize{Rows: 36, Cols: 120}); err != nil {
		t.Fatal(err)
	}
	p.state, err = term.GetState(int(p.slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	p.command = exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestFactEditorPTYHelper$")
	p.command.Env = append(os.Environ(), factPTYEnv+"="+dir, "TERM=xterm-256color")
	p.command.Stdin, p.command.Stdout, p.command.Stderr = p.slave, p.slave, p.slave
	p.command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := p.command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.command.Process.Kill() })
	go func() { p.done <- p.command.Wait() }()
	go func() {
		defer close(p.chunks)
		buffer := make([]byte, 8192)
		for {
			n, err := p.master.Read(buffer)
			if n > 0 {
				select {
				case p.chunks <- string(buffer[:n]):
				case <-t.Context().Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	p.wait(0, "◆ Saltbox Facts", "\x1b[?1049h", "\x1b[?1002h", "\x1b[?1006h")
	raw, err := term.GetState(int(p.slave.Fd()))
	if err != nil || reflect.DeepEqual(raw, p.state) {
		t.Fatalf("terminal did not enter raw mode: %v", err)
	}
	return p
}

func (p *factPTY) send(keys string, want ...string) {
	p.t.Helper()
	start := len(p.output)
	if _, err := io.WriteString(p.master, keys); err != nil {
		p.t.Fatal(err)
	}
	p.wait(start, want...)
}

func (p *factPTY) wait(start int, want ...string) {
	p.t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		complete := true
		for _, text := range want {
			if !strings.Contains(p.output[start:], text) && !strings.Contains(ansi.Strip(p.output[start:]), text) {
				complete = false
			}
		}
		if complete {
			return
		}
		select {
		case chunk, ok := <-p.chunks:
			if !ok {
				p.t.Fatalf("PTY closed waiting for %q: %q", want, p.output[start:])
			}
			p.output += chunk
		case <-timer.C:
			p.t.Fatalf("PTY timed out waiting for %q: %q", want, p.output[start:])
		}
	}
}

func (p *factPTY) finish() {
	p.t.Helper()
	p.wait(0, "\x1b[?1049l", "\x1b[?25h", "\x1b[?1002l", "\x1b[?1006l")
	select {
	case err := <-p.done:
		if err != nil {
			p.t.Fatalf("PTY command failed: %v; output: %q", err, p.output)
		}
	case <-time.After(5 * time.Second):
		p.t.Fatal("PTY command did not exit")
	}
	state, err := term.GetState(int(p.slave.Fd()))
	if err != nil || !reflect.DeepEqual(state, p.state) {
		p.t.Fatalf("terminal state was not restored: %v", err)
	}
	session, err := facts.OpenSession(filepath.Dir(p.path))
	if err != nil {
		p.t.Fatalf("editor lock remains held after exit: %v", err)
	}
	defer closeFactFixture(p.t, session)
	if _, err := os.Stat(p.path); err == nil {
		ctx, cancel := context.WithTimeout(p.t.Context(), time.Second)
		defer cancel()
		if _, err := session.LockRole(ctx, "plex"); err != nil {
			p.t.Fatalf("role lock remains held after exit: %v", err)
		}
	}
}

func (p *factPTY) unchanged() {
	p.t.Helper()
	data, err := os.ReadFile(p.path)
	if err != nil || string(data) != factPTYOriginal {
		p.t.Fatalf("facts changed before Apply: %q, %v", data, err)
	}
}

func closeFactFixture(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Errorf("close fact fixture: %v", err)
	}
}
