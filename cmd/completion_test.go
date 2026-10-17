package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"

	"github.com/saltyorg/sb-go/completion"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func completionTaskManager(t *testing.T) *completion.Manager {
	t.Helper()
	dir := t.TempDir()
	manager := completion.NewManager()
	manager.Paths = completion.Paths{
		BashDir: filepath.Join(dir, "bash"), ZshDir: filepath.Join(dir, "zsh"),
		BashLegacyDir: filepath.Join(dir, "legacy-bash"), ZshLegacyDir: filepath.Join(dir, "legacy-zsh"),
	}
	manager.Probe = func(_ context.Context, shell completion.Shell) (completion.Detection, error) {
		return completion.Detection{Present: shell == completion.Bash}, nil
	}
	manager.NameInUse = func(string) (bool, error) { return false, nil }
	return manager
}

func TestAutomaticCompletionRefreshAllowsUnavailableShell(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		root := &cobra.Command{Use: "sb", SilenceErrors: true, SilenceUsage: true}
		addCompletionCommandWithManager(root, completionTaskManager(t))
		root.SetOut(&bytes.Buffer{})
		args := []string{"completion", "install", "fish"}
		if automatic {
			args = append(args, "--allow-unavailable")
		}
		root.SetArgs(args)
		err := root.ExecuteContext(t.Context())
		if (err == nil) != automatic {
			t.Fatalf("automatic=%v error=%v", automatic, err)
		}
	}
}

func TestCompletionTasksContinueAfterShellFailure(t *testing.T) {
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})
	wantErr := errors.New("fixture bash failure")
	var visited []completion.Shell
	err := runShellCompletionTasks(t.Context(), runner, func(_ context.Context, shell completion.Shell, _, _ io.Writer) error {
		visited = append(visited, shell)
		if shell == completion.Bash {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) || !slices.Equal(visited, completion.SupportedShells()) {
		t.Fatalf("refresh returned %v after visiting %v", err, visited)
	}
}

func TestCompletionRefreshRetainsRunnerTaskTree(t *testing.T) {
	for _, mode := range []string{"update", "self-update"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			process := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCompletionTaskPTYHelper$")
			process.Env = append(os.Environ(), "SB_TEST_COMPLETION_TASK_PTY="+mode, "TERM=xterm-256color")
			master, err := pty.StartWithSize(process, &pty.Winsize{Rows: 24, Cols: 100})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := master.Close(); err != nil {
					t.Error(err)
				}
			}()
			data, readErr := io.ReadAll(master)
			if err := process.Wait(); err != nil {
				t.Fatalf("completion helper: %v\n%s", err, data)
			}
			if readErr != nil && !errors.Is(readErr, syscall.EIO) {
				t.Fatal(readErr)
			}
			text := strings.ReplaceAll(ansi.Strip(string(data)), "\r", "")
			start := strings.LastIndex(text, "● Shell completions refreshed")
			if start < 0 {
				t.Fatalf("missing completion parent task in %q", text)
			}
			final := text[start:]
			for _, child := range []string{"Bash", "Zsh", "Fish"} {
				if !strings.Contains(final, "\n  ● "+child+" completion checked") {
					t.Errorf("missing nested %s task in %q", child, final)
				}
			}
			for _, detail := range []string{"Installed bash completion:", "Skipped zsh completion:", "TEMP_COMPLETION_DETAIL"} {
				if strings.Contains(final, detail) {
					t.Errorf("successful task leaked detail %q into %q", detail, final)
				}
			}
		})
	}
}

func TestCompletionTaskPTYHelper(t *testing.T) {
	mode := os.Getenv("SB_TEST_COMPLETION_TASK_PTY")
	if mode == "" {
		t.Skip("completion progress subprocess helper")
	}
	runner := terminal.NewRunner(terminal.RunnerOptions{Output: os.Stdout})
	if mode == "self-update" {
		executable := filepath.Join(t.TempDir(), "updated-sb")
		if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'TEMP_COMPLETION_DETAIL\\n'\n"), 0700); err != nil {
			t.Fatal(err)
		}
		refreshCompletionsAfterSelfUpdate(t.Context(), runner, executable)
		return
	}
	regenerateInstalledCompletionsWith(t.Context(), &cobra.Command{Use: "sb"}, runner, completionTaskManager(t), []string{"sb"})
}

func TestCompletionRefreshUsesSharedTaskAndShellChildren(t *testing.T) {
	manager := completionTaskManager(t)
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})
	regenerateInstalledCompletionsWith(t.Context(), &cobra.Command{Use: "sb"}, runner, manager, []string{"sb"})
	text := output.String()
	previous := -1
	for _, message := range []string{
		"Refreshing shell completions...", "Refreshing Bash completion...", "Bash completion checked",
		"Refreshing Zsh completion...", "Zsh completion checked", "Refreshing Fish completion...", "Fish completion checked",
		"Shell completions refreshed",
	} {
		at := strings.Index(text, message)
		if at <= previous {
			t.Fatalf("missing or out-of-order task %q in %q", message, text)
		}
		previous = at
	}
}

func TestCompletionGenerateDoesNotProbeOrLoadTags(t *testing.T) {
	for _, shell := range completion.SupportedShells() {
		t.Run(string(shell), func(t *testing.T) {
			root := &cobra.Command{Use: "sb"}
			root.AddCommand(newInstallCommandWithTagLoader(func() ([]string, error) {
				t.Fatal("script generation loaded tags")
				return nil, nil
			}))
			manager := completion.NewManager()
			manager.Probe = func(context.Context, completion.Shell) (completion.Detection, error) {
				t.Fatal("script generation probed installed shells")
				return completion.Detection{}, nil
			}
			addCompletionCommandWithManager(root, manager)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetArgs([]string{"completion", "generate", string(shell)})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if output.Len() == 0 || strings.Contains(output.String(), "Custom tag completion") {
				t.Fatalf("expected native script, got %q", output.String())
			}
			if root.Use != "sb" {
				t.Fatalf("generation changed root command use: %q", root.Use)
			}
		})
	}
}

func TestCompletionRejectsUnsupportedShell(t *testing.T) {
	root := &cobra.Command{Use: "sb", SilenceErrors: true, SilenceUsage: true}
	addCompletionCommand(root)
	root.SetArgs([]string{"completion", "generate", "powershell"})
	if err := root.Execute(); err == nil {
		t.Fatal("unsupported shell accepted")
	}
	if _, err := generateCompletion(root, "unknown", "sb"); err == nil {
		t.Fatal("unknown generator accepted")
	}
	if root.Use != "sb" {
		t.Fatalf("failed generation changed command use: %q", root.Use)
	}
}

func TestBinaryNamesIncludeOwnDirectoryAndDeduplicateSymlinks(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "sb")
	if err := os.WriteFile(executable, []byte("binary fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sb2", "saltbox"} {
		if err := os.Symlink("sb", filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("missing", filepath.Join(directory, "broken")); err != nil {
		t.Fatal(err)
	}
	aliasDirectory := filepath.Join(t.TempDir(), "bin")
	if err := os.Symlink(directory, aliasDirectory); err != nil {
		t.Fatal(err)
	}
	names, err := binaryNamesForExecutable(filepath.Join(directory, "sb2"), []string{directory, aliasDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"saltbox", "sb", "sb2"}; !slices.Equal(names, want) {
		t.Fatalf("binary names = %v, want %v", names, want)
	}
}

type failedCompletionWriter struct{}

func (failedCompletionWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture output failure")
}

func TestCompletionOutputErrorsPropagate(t *testing.T) {
	err := writeCompletionResults(failedCompletionWriter{}, []completion.Result{{Shell: completion.Bash, Status: completion.Installed, Path: "/fixture/sb.bash"}})
	if err == nil || !strings.Contains(err.Error(), "fixture output failure") {
		t.Fatalf("output failure was lost: %v", err)
	}
}

func TestSelfUpdateRefreshExecutesSelectedNewBinary(t *testing.T) {
	directory := t.TempDir()
	arguments := filepath.Join(directory, "arguments")
	t.Setenv("SB_COMPLETION_REFRESH_ARGS", arguments)
	executable := filepath.Join(directory, "updated sb")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$SB_COMPLETION_REFRESH_ARGS\"\nprintf 'Installed new generator\\n'\n"
	if err := os.WriteFile(executable, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Output: &output})
	refreshCompletionsAfterSelfUpdate(t.Context(), runner, executable)
	got, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, shell := range completion.SupportedShells() {
		want += "completion\ninstall\n" + string(shell) + "\n--allow-unavailable\n"
	}
	if string(got) != want {
		t.Fatalf("new binary args = %q", got)
	}
	if !strings.Contains(output.String(), "Installed new generator") {
		t.Fatalf("refresh output missing: %q", output.String())
	}
}

func TestSelfUpdateRefreshReportsFailure(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "updated-sb")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'fixture migration conflict\\n' >&2\nexit 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Output: &output})
	refreshCompletionsAfterSelfUpdate(t.Context(), runner, executable)
	for _, expected := range []string{"CLI updated, but shell completion refresh failed", "fixture migration conflict"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %q", expected, output.String())
		}
	}
}
