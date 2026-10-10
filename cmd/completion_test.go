package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/completion"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

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
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SB_COMPLETION_REFRESH_ARGS\"\nprintf 'Installed new generator\\n'\n"
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
	if string(got) != "completion\ninstall\n" {
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
