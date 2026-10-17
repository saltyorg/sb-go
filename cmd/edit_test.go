package cmd

import (
	"bytes"
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
	"github.com/creack/pty"
	"github.com/spf13/cobra"

	"github.com/saltyorg/sb-go/host"
)

func TestEditFileCommandUsesCobraStreams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "editor")
	script := "#!/bin/sh\nIFS= read -r line\nprintf 'out:%s:%s' \"$line\" \"$1\"\nprintf 'err:%s' \"$line\" >&2\n"
	if err := os.WriteFile(editor, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	setTestEditor(t, editor)
	var output, stderr bytes.Buffer
	root := &cobra.Command{Use: "sb"}
	root.SetIn(strings.NewReader("typed input\n"))
	root.SetOut(&output)
	root.SetErr(&stderr)
	root.AddCommand(newEditFileCommand("fixture", "Fixture", path))
	root.SetArgs([]string{"fixture"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := "out:typed input:" + path; output.String() != want {
		t.Errorf("editor output = %q, want %q", output.String(), want)
	}
	if stderr.String() != "err:typed input" {
		t.Errorf("editor stderr = %q", stderr.String())
	}
}

func TestConfigSelectorEditorUsesTerminalStreams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "editor")
	script := "#!/bin/sh\nprintf '\\nEDITOR_READY\\n'\nIFS= read -r line\nprintf 'OUT:%s\\n' \"$line\"\nprintf 'ERR:%s' \"$line\" >&2\n"
	if err := os.WriteFile(editor, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	setTestEditor(t, editor)
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	if err := pty.Setsize(master, &pty.Winsize{Rows: 24, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	model := selectorTestModel(ctx, path)
	model.stderr = &stderr
	done := make(chan error, 1)
	go func() {
		done <- runConfigSelector(model, tea.WithInput(slave), tea.WithOutput(slave), tea.WithoutSignalHandler())
	}()
	selectorReady, ready, output := make(chan struct{}), make(chan struct{}), make(chan string, 1)
	go func() {
		var transcript strings.Builder
		data := make([]byte, 4096)
		selectorSeen, editorSeen := false, false
		for {
			n, err := master.Read(data)
			if err != nil {
				output <- "read error: " + err.Error()
				return
			}
			transcript.Write(data[:n])
			text := transcript.String()
			if !selectorSeen && strings.Contains(text, "Select a Saltbox configuration file to edit:") {
				selectorSeen = true
				close(selectorReady)
			}
			if !editorSeen && strings.Contains(text, "EDITOR_READY") {
				editorSeen = true
				close(ready)
			}
			if strings.Contains(text, "OUT:typed input") {
				output <- text
				return
			}
		}
	}()
	select {
	case <-selectorReady:
	case <-ctx.Done():
		t.Fatal("selector did not render on its output terminal")
	}
	if _, err := master.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("selector did not start editor on its output terminal")
	}
	if _, err := master.Write([]byte("typed input\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-output:
		if !strings.Contains(got, "OUT:typed input") {
			t.Errorf("editor output = %q", got)
		}
	case <-ctx.Done():
		t.Fatal("editor did not read from selector input")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "ERR:typed input" {
		t.Errorf("editor stderr = %q", stderr.String())
	}
}

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
			setTestEditor(t, editor)
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
			setTestEditor(t, "'"+editor+"' "+tt.args)
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
			setTestEditor(t, value)
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
			setTestEditor(t, editor)
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
	setTestEditor(t, editor)
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
if [ "$1" = "--preserve-env=VISUAL,EDITOR" ]; then
    shift
else
    unset VISUAL EDITOR
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
	setTestEditor(t, editor)
	t.Setenv("SB_TEST_EDITOR_RELAUNCH", dir)
	originalArgs := os.Args
	os.Args = []string{os.Args[0], "-test.run=^TestEditorRelaunchHelper$"}
	t.Cleanup(func() { os.Args = originalArgs })
	for _, variable := range []string{"EDITOR", "VISUAL"} {
		t.Run(variable, func(t *testing.T) {
			setTestEditor(t, "")
			t.Setenv(variable, editor)
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
		})
	}
}

func TestEditorCommandPreferences(t *testing.T) {
	for _, tt := range []struct {
		name      string
		visual    string
		editor    string
		installed []string
		want      string
	}{
		{"visual first", "custom-visual", "custom-editor", []string{"custom-visual", "custom-editor"}, "custom-visual"},
		{"editor next", "", "custom-editor", []string{"custom-editor", "nano"}, "custom-editor"},
		{"system helper", "", "", []string{"sensible-editor", "editor", "nano", "vi"}, "sensible-editor"},
		{"system alternative", "", "", []string{"editor", "nano", "vi"}, "editor"},
		{"nano fallback", "", "", []string{"nano", "vi"}, "nano"},
		{"vi fallback", "", "", []string{"vi"}, "vi"},
		{"no editor", "", "", nil, ""},
		{"invalid explicit editor", "", "missing", []string{"nano"}, ""},
		{"invalid explicit visual", "missing", "custom-editor", []string{"custom-editor"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.yml")
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			for _, name := range tt.installed {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf '%s' \"$0\"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir)
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)
			command, err := editorCommand(t.Context(), path)
			if tt.want == "" {
				if err == nil {
					t.Fatal("expected an editor selection error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(dir, tt.want)
			if command.Path != want {
				t.Fatalf("editor = %q, want %q", command.Path, want)
			}
			output, err := command.Output()
			if err != nil || string(output) != want {
				t.Fatalf("editor execution = %q, %v, want %q", output, err, want)
			}
		})
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
			setTestEditor(t, os.Args[0]+" -test.run=^TestEditorCancellationHelper$")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if mode == "direct" {
					command := &cobra.Command{}
					command.SetContext(ctx)
					done <- openEditor(command, path)
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

func setTestEditor(t *testing.T, value string) {
	t.Helper()
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", value)
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
