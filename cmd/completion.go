package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/saltyorg/sb-go/completion"
	"github.com/saltyorg/sb-go/terminal"

	"github.com/spf13/cobra"
)

func addCompletionCommand(rootCmd *cobra.Command) {
	addCompletionCommandWithManager(rootCmd, completion.NewManager())
}

func addCompletionCommandWithManager(rootCmd *cobra.Command, manager *completion.Manager) {
	command := &cobra.Command{
		Use:    "completion",
		Hidden: true,
		Short:  "Install shell completion for sb",
		Long: "Install system-wide completion for installed Bash, Zsh, and Fish shells.\n\n" +
			"Bash requires the bash-completion package. Scripts use each shell's shared\n" +
			"loading directory. Generated legacy files are migrated after successful installation.\n\n" +
			"Restart an existing shell, or source its installed file.\n" +
			"Use \"completion generate <shell>\" to write a script to standard output.",
		Args: cobra.NoArgs,
	}
	install := func(cmd *cobra.Command, shells []completion.Shell, explicit bool) error {
		names, err := getAllBinaryNames()
		if err != nil {
			return err
		}
		results, installErr := manager.Install(cmd.Context(), shells, names, func(shell completion.Shell, name string) ([]byte, error) {
			return generateCompletion(rootCmd, shell, name)
		})
		if err := writeCompletionResults(cmd.OutOrStdout(), results); err != nil {
			return errors.Join(installErr, err)
		}
		if explicit {
			for _, result := range results {
				if result.Status == completion.Skipped && result.Name == "" {
					installErr = errors.Join(installErr, fmt.Errorf("%s completion unavailable: %s", result.Shell, result.Reason))
				}
			}
		}
		return installErr
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		return install(cmd, completion.SupportedShells(), false)
	}
	for _, shell := range completion.SupportedShells() {
		command.AddCommand(&cobra.Command{
			Use:   string(shell),
			Short: fmt.Sprintf("Install %s completion", shell),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return install(cmd, []completion.Shell{shell}, true)
			},
		})
	}
	command.AddCommand(&cobra.Command{
		Use:       "install [bash|zsh|fish]",
		Short:     "Install completion for installed shells",
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return install(cmd, completion.SupportedShells(), false)
			}
			return install(cmd, []completion.Shell{completion.Shell(args[0])}, true)
		},
	})
	command.AddCommand(&cobra.Command{
		Use:       "generate <bash|zsh|fish>",
		Short:     "Write a completion script to standard output",
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := generateCompletion(rootCmd, completion.Shell(args[0]), rootCmd.Name())
			if err != nil {
				return err
			}
			if _, err := cmd.OutOrStdout().Write(script); err != nil {
				return fmt.Errorf("write completion script: %w", err)
			}
			return nil
		},
	})
	rootCmd.AddCommand(command)
}

func generateCompletion(rootCmd *cobra.Command, shell completion.Shell, name string) ([]byte, error) {
	originalUse := rootCmd.Use
	rootCmd.Use = name
	defer func() { rootCmd.Use = originalUse }()

	var script bytes.Buffer
	var err error
	switch shell {
	case completion.Bash:
		err = rootCmd.GenBashCompletionV2(&script, true)
	case completion.Zsh:
		err = rootCmd.GenZshCompletion(&script)
	case completion.Fish:
		err = rootCmd.GenFishCompletion(&script, true)
	default:
		return nil, fmt.Errorf("unsupported completion shell %q", shell)
	}
	if err != nil {
		return nil, fmt.Errorf("generate %s completion for %s: %w", shell, name, err)
	}
	return script.Bytes(), nil
}

func writeCompletionResults(output io.Writer, results []completion.Result) error {
	for _, result := range results {
		var message string
		switch result.Status {
		case completion.Installed:
			message = fmt.Sprintf("Installed %s completion: %s", result.Shell, result.Path)
		case completion.Cleaned:
			message = fmt.Sprintf("Removed obsolete %s completion: %s", result.Shell, result.Path)
		case completion.Skipped:
			if result.Name != "" {
				message = fmt.Sprintf("Preserved %s completion: %s; %s", result.Shell, result.Path, result.Reason)
			} else {
				message = fmt.Sprintf("Skipped %s completion: %s", result.Shell, result.Reason)
			}
		case completion.Failed:
			message = fmt.Sprintf("Failed %s completion: %s", result.Shell, result.Reason)
		}
		if _, err := fmt.Fprintln(output, message); err != nil {
			return fmt.Errorf("write completion result: %w", err)
		}
		for _, removed := range result.Removed {
			if result.Status == completion.Cleaned && removed == result.Path {
				continue
			}
			if _, err := fmt.Fprintf(output, "Removed old completion: %s\n", removed); err != nil {
				return fmt.Errorf("write completion cleanup result: %w", err)
			}
		}
	}
	return nil
}

func regenerateInstalledCompletions(ctx context.Context, rootCmd *cobra.Command, runner *terminal.Runner) {
	names, err := getAllBinaryNames()
	if err != nil {
		runner.Warning(fmt.Sprintf("Shell completion refresh failed: %v", err))
		return
	}
	results, err := completion.NewManager().Install(ctx, completion.SupportedShells(), names, func(shell completion.Shell, name string) ([]byte, error) {
		return generateCompletion(rootCmd, shell, name)
	})
	for _, result := range results {
		if result.Status != completion.Failed {
			var message bytes.Buffer
			if writeErr := writeCompletionResults(&message, []completion.Result{result}); writeErr != nil {
				runner.Warning(writeErr.Error())
				continue
			}
			runner.Info(strings.TrimSpace(message.String()))
		}
	}
	if err != nil {
		runner.Warning(fmt.Sprintf("Shell completion refresh failed: %v", err))
	}
}

func getAllBinaryNames() ([]string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get completion executable: %w", err)
	}
	return binaryNamesForExecutable(executable, []string{"/usr/local/bin", "/usr/bin", "/bin"})
}

func binaryNamesForExecutable(executable string, directories []string) ([]string, error) {
	realExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve completion executable: %w", err)
	}
	names := map[string]bool{filepath.Base(realExecutable): true}
	seenDirectories := make(map[string]bool)
	directories = append(slices.Clone(directories), filepath.Dir(executable), filepath.Dir(realExecutable))
	for _, directory := range directories {
		realDirectory, err := filepath.EvalSymlinks(directory)
		if err != nil || seenDirectories[realDirectory] {
			continue
		}
		seenDirectories[realDirectory] = true
		entries, err := os.ReadDir(realDirectory)
		if err != nil {
			return nil, fmt.Errorf("read completion executable directory %s: %w", realDirectory, err)
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			target, err := filepath.EvalSymlinks(filepath.Join(realDirectory, entry.Name()))
			if err == nil && target == realExecutable {
				names[entry.Name()] = true
			}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	slices.Sort(result)
	return result, nil
}
