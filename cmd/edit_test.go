package cmd

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/saltyorg/sb-go/host"
)

func TestEditorCommandValidatesExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	nonExecutable := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(nonExecutable, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, editor := range []string{filepath.Join(dir, "missing"), nonExecutable, dir, "sb-test-missing-editor"} {
		t.Run(editor, func(t *testing.T) {
			t.Setenv("EDITOR", editor)
			if _, err := editorCommand(t.Context(), path); err == nil {
				t.Fatalf("invalid executable %q was accepted", editor)
			}
		})
	}
}

func TestEditorCommandQuotedArguments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "editor with spaces")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '<%s>\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		args string
		want []string
	}{
		{"double quotes", `-c "set number"`, []string{"-c", "set number"}},
		{"single quotes", `-c 'set number'`, []string{"-c", "set number"}},
		{"backslash", `escaped\ argument`, []string{"escaped argument"}},
		{"empty arguments", `"" ''`, []string{"", ""}},
		{"literal shell syntax", `$HOME $(command) ; * ~`, []string{"$HOME", "$(command)", ";", "*", "~"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EDITOR", "'"+editor+"' "+tt.args)
			command, err := editorCommand(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			want := append(append([]string{editor}, tt.want...), path)
			if !slices.Equal(command.Args, want) {
				t.Fatalf("editor args = %q, want %q", command.Args, want)
			}
			output, err := command.Output()
			wantOutput := "<" + strings.Join(want[1:], ">\n<") + ">\n"
			if err != nil || string(output) != wantOutput {
				t.Fatalf("editor output = %q, %v, want %q", output, err, wantOutput)
			}
		})
	}
}

func TestEditorCommandRejectsInvalidQuoting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`vim "unterminated`, `vim 'unterminated`, `vim trailing\`, `"" -R`, "   "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("EDITOR", value)
			if _, err := editorCommand(t.Context(), path); err == nil || !strings.Contains(err.Error(), "invalid EDITOR") {
				t.Fatalf("invalid editor %q returned %v", value, err)
			}
		})
	}
}

func TestConfigSelectorReturnsEditorErrors(t *testing.T) {
	for _, kind := range []string{"editor exit", "missing editor", "missing configuration"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.yml")
			if kind != "missing configuration" {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			editor := filepath.Join(dir, "editor")
			if kind != "missing editor" {
				if err := os.WriteFile(editor, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				editor = "sb-test-missing-editor"
			}
			t.Setenv("EDITOR", editor)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			err := runConfigSelector(selectorTestModel(ctx, path), tea.WithInput(strings.NewReader("\r")),
				tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
			if err == nil || ctx.Err() != nil {
				t.Fatalf("selector error = %v, context = %v", err, ctx.Err())
			}
			if kind == "editor exit" {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
					t.Fatalf("selector lost editor exit error: %v", err)
				}
			}
		})
	}
}

func selectorTestModel(ctx context.Context, path string) ConfigSelectorModel {
	return ConfigSelectorModel{ctx: ctx, list: list.New([]list.Item{ConfigItem{title: "fixture", path: path}}, list.NewDefaultDelegate(), 80, 24)}
}

func TestEditorCommandUsesEditor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "vim.basic")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	c, err := editorCommand(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != editor {
		t.Fatalf("editor = %q, want %q", c.Path, editor)
	}
	if len(c.Args) != 2 || c.Args[1] != path {
		t.Fatalf("editor args = %q, want [%s %s]", c.Args, editor, path)
	}
	output, err := c.Output()
	if err != nil || string(output) != path {
		t.Fatalf("editor output = %q, %v, want %q", output, err, path)
	}
}

func TestEditorSurvivesRootRelaunch(t *testing.T) {
	dir := t.TempDir()
	// Model sudo's environment filtering without changing users or sudo policy.
	sudo := `#!/bin/sh
if [ "$1" = "--preserve-env=EDITOR" ]; then
    shift
else
    unset EDITOR
fi
exec "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nano"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.yml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	editor := filepath.Join(dir, "vim.basic")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	t.Setenv("SB_TEST_EDITOR_RELAUNCH", dir)
	originalArgs := os.Args
	os.Args = []string{os.Args[0], "-test.run=^TestEditorRelaunchHelper$"}
	t.Cleanup(func() { os.Args = originalArgs })
	if code, err := host.RelaunchAsRoot(); err != nil || code != 0 {
		t.Fatalf("relaunch = %d, %v", code, err)
	}
	selected, err := os.ReadFile(filepath.Join(dir, "selected-editor"))
	if err != nil {
		t.Fatal(err)
	}
	want := editor + "\n" + filepath.Join(dir, "settings.yml")
	if string(selected) != want {
		t.Fatalf("editor after sudo = %q, want %q", selected, want)
	}
}

func TestEditorRelaunchHelper(t *testing.T) {
	dir := os.Getenv("SB_TEST_EDITOR_RELAUNCH")
	if dir == "" {
		return
	}
	c, err := editorCommand(t.Context(), filepath.Join(dir, "settings.yml"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selected-editor"), []byte(strings.Join([]string{c.Path, string(output)}, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEditorCancellation(t *testing.T) {
	for _, mode := range []string{"direct", "selector"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.yml")
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(dir, "ready.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			t.Setenv("SB_TEST_EDITOR_CANCEL_SOCKET", socket)
			t.Setenv("EDITOR", os.Args[0]+" -test.run=^TestEditorCancellationHelper$")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if mode == "direct" {
					done <- openEditor(ctx, path)
					return
				}
				done <- runConfigSelector(selectorTestModel(ctx, path), tea.WithInput(strings.NewReader("\r")),
					tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
			}()
			if err := listener.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			connection, err := listener.Accept()
			if err != nil {
				t.Fatalf("editor did not start: %v", err)
			}
			defer connection.Close()
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Error("cancelled editing returned success")
				}
			case <-time.After(5 * time.Second):
				_ = connection.Close() // Release the helper if cancellation failed.
				<-done
				t.Fatal("cancellation did not stop the editor")
			}
		})
	}
}

func TestEditorCancellationHelper(t *testing.T) {
	socket := os.Getenv("SB_TEST_EDITOR_CANCEL_SOCKET")
	if socket == "" {
		t.Skip("editor subprocess helper")
	}
	connection, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, _ = io.Copy(io.Discard, connection)
}
