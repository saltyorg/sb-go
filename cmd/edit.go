package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/saltyorg/sb-go/constants"

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
	return "\nSelect a configuration file to edit:\n\n" + m.list.View()
}

func openEditor(path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano" // Default to nano if EDITOR is not set
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error opening editor: %v\n", err)
	}
}

func runBubbleTeaList() {
	configItems := []list.Item{
		ConfigItem{
			title:       "accounts.yml",
			description: "Saltbox Accounts configuration",
			path:        constants.SaltboxAccountsPath,
		},
		ConfigItem{
			title:       "settings.yml",
			description: "Saltbox basic configuration",
			path:        constants.SaltboxSettingsPath,
		},
		ConfigItem{
			title:       "adv_settings.yml",
			description: "Saltbox advanced configuration",
			path:        constants.SaltboxAdvancedSettingsPath,
		},
		ConfigItem{
			title:       "backup_config.yml",
			description: "Saltbox backup configuration",
			path:        constants.SaltboxBackupConfigPath,
		},
		ConfigItem{
			title:       "hetzner_config.yml",
			description: "Saltbox Hetzner VLAN configuration",
			path:        constants.SaltboxHetznerVLANPath,
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
		fmt.Printf("Error running bubbletea program: %v\n", err)
		os.Exit(1)
	}
}

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:   "edit [config]",
	Short: "Edit Saltbox configuration files",
	Long: `Edit Saltbox configuration files using your default editor.
	
Available configurations:
  - accounts: Edit accounts.yml
  - adv_settings: Edit advanced settings (adv_settings.yml)
  - backup_config: Edit backup configuration (backup_config.yml)
  - hetzner_vlan: Edit Hetzner VLAN configuration (hetzner_vlan.yml)
  - settings: Edit general settings (settings.yml)

If no configuration is specified, an interactive menu will be shown.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			runBubbleTeaList()
			return
		}

		switch args[0] {
		case "accounts":
			openEditor(constants.SaltboxAccountsPath)
		case "adv_settings":
			openEditor(constants.SaltboxAdvancedSettingsPath)
		case "backup_config":
			openEditor(constants.SaltboxBackupConfigPath)
		case "hetzner_vlan":
			openEditor(constants.SaltboxHetznerVLANPath)
		case "settings":
			openEditor(constants.SaltboxSettingsPath)
		default:
			fmt.Printf("Unknown configuration: %s\n", args[0])
			fmt.Println("Run 'sb edit' to see all available configurations")
		}
	},
}

func init() {
	rootCmd.AddCommand(editCmd)

	// Subcommands for each configuration file
	editCmd.AddCommand(&cobra.Command{
		Use:   "accounts",
		Short: "Edit accounts.yml",
		Run: func(cmd *cobra.Command, args []string) {
			openEditor(constants.SaltboxAccountsPath)
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "adv_settings",
		Short: "Edit adv_settings.yml",
		Run: func(cmd *cobra.Command, args []string) {
			openEditor(constants.SaltboxAdvancedSettingsPath)
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "backup_config",
		Short: "Edit backup_config.yml",
		Run: func(cmd *cobra.Command, args []string) {
			openEditor(constants.SaltboxBackupConfigPath)
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "hetzner_vlan",
		Short: "Edit hetzner_vlan.yml",
		Run: func(cmd *cobra.Command, args []string) {
			openEditor(constants.SaltboxHetznerVLANPath)
		},
	})

	editCmd.AddCommand(&cobra.Command{
		Use:   "settings",
		Short: "Edit settings.yml",
		Run: func(cmd *cobra.Command, args []string) {
			openEditor(constants.SaltboxSettingsPath)
		},
	})
}
