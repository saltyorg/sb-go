package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/signals"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
)

// ConfigItem represents a configuration file option
type ConfigItem struct {
	title       string
	description string
	path        string
}

// Title returns the title for the list item
func (i ConfigItem) Title() string { return i.title }

// Description returns the description for the list item
func (i ConfigItem) Description() string { return i.description }

// FilterValue returns the filter value for the list item
func (i ConfigItem) FilterValue() string { return i.title }

// Styling
var docStyle = lipgloss.NewStyle().Margin(1, 2)

// ConfigSelectorModel represents the bubbletea model for selecting configuration files
type ConfigSelectorModel struct {
	ctx    context.Context
	list   list.Model
	stderr io.Writer
	err    error
}

func (m ConfigSelectorModel) Init() tea.Cmd {
	return nil
}

// editorFinishedMsg is sent when the editor process exits.
type editorFinishedMsg struct{ err error }

func (m ConfigSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			signals.Shutdown(m.ctx, 130)
			return m, tea.Quit
		}
		if msg.String() == "q" {
			return m, tea.Quit
		}

		if msg.String() == "enter" {
			selectedItem, ok := m.list.SelectedItem().(ConfigItem)
			if ok {
				c, err := editorCommand(m.ctx, selectedItem.path)
				if err != nil {
					m.err = err
					return m, tea.Quit
				}
				c.Stderr = m.stderr
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return editorFinishedMsg{err}
				})
			}
		}
	case editorFinishedMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("error opening editor: %w", msg.err)
		}
		return m, tea.Quit
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m ConfigSelectorModel) View() tea.View {
	v := tea.NewView("\nSelect a Saltbox configuration file to edit:\n\n" + m.list.View())
	v.AltScreen = true
	return v
}

// editorCommand builds an *exec.Cmd for the user's preferred editor.
func editorCommand(ctx context.Context, path string) (*exec.Cmd, error) {
	// Check if file exists
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration file does not yet exist: %s", path)
		}
		return nil, fmt.Errorf("inspect configuration file %s: %w", path, err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano" // Default to nano if EDITOR is not set
	}

	// Parse quoting and escapes without expanding or executing shell syntax.
	editorParts, err := shellquote.Split(editor)
	if err != nil {
		return nil, fmt.Errorf("invalid EDITOR environment variable: %w", err)
	}
	if len(editorParts) == 0 || editorParts[0] == "" {
		return nil, fmt.Errorf("invalid EDITOR environment variable")
	}

	// Get the absolute path of the editor executable
	editorPath, err := exec.LookPath(editorParts[0])
	if err != nil {
		return nil, fmt.Errorf("find editor %q: %w", editorParts[0], err)
	}

	// Construct command with validated editor and additional args if any
	var args []string
	if len(editorParts) > 1 {
		args = append(editorParts[1:], path)
	} else {
		args = []string{path}
	}

	return exec.CommandContext(ctx, editorPath, args...), nil
}

func openEditor(cmd *cobra.Command, path string) error {
	c, err := editorCommand(cmd.Context(), path)
	if err != nil {
		return err
	}
	c.Stdin = cmd.InOrStdin()
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return fmt.Errorf("error opening editor: %w", err)
	}
	return nil
}

func runBubbleTeaList(cmd *cobra.Command) error {
	configItems := []list.Item{
		ConfigItem{
			title:       "Accounts",
			description: "accounts.yml",
			path:        layout.SaltboxAccountsConfigPath,
		},
		ConfigItem{
			title:       "Settings",
			description: "settings.yml",
			path:        layout.SaltboxSettingsConfigPath,
		},
		ConfigItem{
			title:       "Advanced Settings",
			description: "adv_settings.yml",
			path:        layout.SaltboxAdvancedSettingsConfigPath,
		},
		ConfigItem{
			title:       "Backup Settings",
			description: "backup_config.yml",
			path:        layout.SaltboxBackupConfigPath,
		},
		ConfigItem{
			title:       "Hetzner VLAN Settings",
			description: "hetzner_vlan.yml",
			path:        layout.SaltboxHetznerVLANConfigPath,
		},
		ConfigItem{
			title:       "Inventory Settings",
			description: "localhost.yml",
			path:        layout.SaltboxInventoryConfigPath,
		},
	}

	// Initialize a list with proper dimensions
	delegate := list.NewDefaultDelegate()
	m := ConfigSelectorModel{ctx: cmd.Context(), list: list.New(configItems, delegate, 30, 10), stderr: cmd.ErrOrStderr()} // Set width and height
	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(false)
	m.list.SetShowHelp(true)
	m.list.SetShowFilter(false)
	m.list.SetShowPagination(false)

	// Get terminal dimensions
	return runConfigSelector(m, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
}

func runConfigSelector(m ConfigSelectorModel, options ...tea.ProgramOption) error {
	options = append([]tea.ProgramOption{tea.WithContext(m.ctx)}, options...)
	p := tea.NewProgram(m, options...)
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running bubbletea program: %w", err)
	}
	return final.(ConfigSelectorModel).err
}

func addEditCommand(rootCmd *cobra.Command) {
	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit Saltbox configuration files",
		Long:  `Edit Saltbox configuration files using your default editor.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBubbleTeaList(cmd)
		},
	}

	// Subcommands for each configuration file
	editCmd.AddCommand(
		newEditFileCommand("accounts", "Accounts", layout.SaltboxAccountsConfigPath),
		newEditFileCommand("adv_settings", "Advanced Settings", layout.SaltboxAdvancedSettingsConfigPath),
		newEditFileCommand("backup_config", "Backup Settings", layout.SaltboxBackupConfigPath),
		newEditFileCommand("hetzner_vlan", "Hetzner VLAN Settings", layout.SaltboxHetznerVLANConfigPath),
		newEditFileCommand("settings", "Settings", layout.SaltboxSettingsConfigPath),
		newEditFileCommand("inventory", "Inventory Settings", layout.SaltboxInventoryConfigPath),
	)

	rootCmd.AddCommand(editCmd)
}

func newEditFileCommand(use, short, path string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return openEditor(cmd, path)
		},
	}
}
