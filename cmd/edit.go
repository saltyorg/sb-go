package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
	"github.com/saltyorg/sb-go/internal/executor"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	list list.Model
}

func (m ConfigSelectorModel) Init() tea.Cmd {
	return nil
}

func (m ConfigSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		if msg.String() == "enter" {
			selectedItem, ok := m.list.SelectedItem().(ConfigItem)
			if ok {
				openEditor(selectedItem.path)
				return m, tea.Quit
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m ConfigSelectorModel) View() string {
	return "\nSelect a Saltbox configuration file to edit:\n\n" + m.list.View()
}

func openEditor(path string) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Error: the configuration file does not yet exist: %s\n", path)
		return
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano" // Default to nano if EDITOR is not set
	}

	// Validate and sanitize editor command
	// Split on whitespace to get the base command
	editorParts := strings.Fields(editor)
	if len(editorParts) == 0 {
		fmt.Printf("Error: Invalid EDITOR environment variable\n")
		return
	}

	// Get the absolute path of the editor executable
	editorPath, err := exec.LookPath(editorParts[0])
	if err != nil {
		// If not in PATH, check if it's an absolute path
		if filepath.IsAbs(editorParts[0]) {
			editorPath = editorParts[0]
		} else {
			fmt.Printf("Error: Editor '%s' not found in PATH\n", editorParts[0])
			return
		}
	}

	// Construct command with validated editor and additional args if any
	var args []string
	if len(editorParts) > 1 {
		// Include any additional arguments from EDITOR variable
		args = append(editorParts[1:], path)
	} else {
		args = []string{path}
	}

	// Run editor using unified executor in interactive mode
	_, err = executor.Run(context.Background(), editorPath,
		executor.WithArgs(args...),
		executor.WithOutputMode(executor.OutputModeInteractive),
	)
	if err != nil {
		fmt.Printf("Error opening editor: %v\n", err)
	}
}

func runBubbleTeaList() error {
	configItems := []list.Item{
		ConfigItem{
			title:       "Accounts",
			description: "accounts.yml",
			path:        constants.SaltboxAccountsConfigPath,
		},
		ConfigItem{
			title:       "Settings",
			description: "settings.yml",
			path:        constants.SaltboxSettingsConfigPath,
		},
		ConfigItem{
			title:       "Advanced Settings",
			description: "adv_settings.yml",
			path:        constants.SaltboxAdvancedSettingsConfigPath,
		},
		ConfigItem{
			title:       "Backup Settings",
			description: "backup_config.yml",
			path:        constants.SaltboxBackupConfigPath,
		},
		ConfigItem{
			title:       "Hetzner VLAN Settings",
			description: "hetzner_vlan.yml",
			path:        constants.SaltboxHetznerVLANConfigPath,
		},
		ConfigItem{
			title:       "Inventory Settings",
			description: "localhost.yml",
			path:        constants.SaltboxInventoryConfigPath,
		},
	}

	// Initialize a list with proper dimensions
	delegate := list.NewDefaultDelegate()
	m := ConfigSelectorModel{list: list.New(configItems, delegate, 30, 10)} // Set width and height
	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(false)
	m.list.SetShowHelp(true)
	m.list.SetShowFilter(false)
	m.list.SetShowPagination(false)

	// Get terminal dimensions
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running bubbletea program: %w", err)
	}
	return nil
}

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit Saltbox configuration files",
	Long:  `Edit Saltbox configuration files using your default editor.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			if err := runBubbleTeaList(); err != nil {
				return err
			}
			return nil
		}

		switch args[0] {
		case "accounts":
			openEditor(constants.SaltboxAccountsConfigPath)
		case "adv_settings":
			openEditor(constants.SaltboxAdvancedSettingsConfigPath)
		case "backup_config":
			openEditor(constants.SaltboxBackupConfigPath)
		case "hetzner_vlan":
			openEditor(constants.SaltboxHetznerVLANConfigPath)
		case "inventory":
			openEditor(constants.SaltboxInventoryConfigPath)
		case "settings":
			openEditor(constants.SaltboxSettingsConfigPath)
		default:
			fmt.Printf("Unknown configuration: %s\n", args[0])
			fmt.Println("Run 'sb edit' to see all available configurations")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)

	// Subcommands for each configuration file
	editCmd.AddCommand(&cobra.Command{
		Use:   "accounts",
		Short: "Accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxAccountsConfigPath)
			return nil
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "adv_settings",
		Short: "Advanced Settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxAdvancedSettingsConfigPath)
			return nil
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "backup_config",
		Short: "Backup Settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxBackupConfigPath)
			return nil
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "hetzner_vlan",
		Short: "Hetzner VLAN Settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxHetznerVLANConfigPath)
			return nil
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "settings",
		Short: "Settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxSettingsConfigPath)
			return nil
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "inventory",
		Short: "Inventory Settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			openEditor(constants.SaltboxInventoryConfigPath)
			return nil
		},
	})
}
