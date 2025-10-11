package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Display logs of managed systemd services",
	Long:  `Displays a list of managed systemd services.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleLogs()
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}

const logPageSize = 500

type serviceItem struct {
	name string
}

func (i serviceItem) Title() string       { return i.name }
func (i serviceItem) Description() string { return "" }
func (i serviceItem) FilterValue() string { return i.name }

type model struct {
	list                list.Model
	viewport            viewport.Model
	serviceItems        []list.Item
	selectedService     string
	beforeCursor        string
	afterCursor         string
	width               int
	height              int
	logs                string
	activeView          string
	viewportInitialized bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Always use a split view layout
		listWidth := msg.Width / 4
		logsWidth := msg.Width - listWidth

		m.list.SetWidth(listWidth)
		m.list.SetHeight(msg.Height)

		if m.viewportInitialized {
			m.viewport.Width = logsWidth
			m.viewport.Height = msg.Height
		}

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			if m.activeView == "list" {
				if i, ok := m.list.SelectedItem().(serviceItem); ok {
					newService := i.name
					m.activeView = "logs"

					// Initialize the viewport if necessary
					if !m.viewportInitialized {
						// Split view layout
						listWidth := m.width / 4
						logsWidth := m.width - listWidth

						m.viewport = viewport.New(logsWidth, m.height)
						m.viewport.Style = lipgloss.NewStyle().Padding(1, 2)
						m.viewportInitialized = true
					}

					// Only fetch new logs if a different service was selected
					if newService != m.selectedService {
						m.selectedService = newService
						m.beforeCursor = ""
						m.afterCursor = ""
						return m, fetchLogs(m.selectedService, false, "")
					} else {
						// Make sure we re-apply the current log content
						m.viewport.SetContent(m.logs)
					}
				}
			}

		case "left", "esc":
			if m.activeView == "logs" {
				m.activeView = "list"
				// Focus back on the list but keep the split view
				return m, nil
			}

		case "up":
			if m.activeView == "logs" && m.beforeCursor != "" {
				return m, fetchLogs(m.selectedService, true, m.beforeCursor)
			}

		case "down":
			if m.activeView == "logs" && m.afterCursor != "" {
				return m, fetchLogs(m.selectedService, false, m.afterCursor)
			}
		}

	case logsMsg:
		if m.activeView == "logs" {
			m.logs = string(msg.logs)
			m.viewport.SetContent(m.logs)
			if msg.reverse {
				// For reverse (older logs), we want to show from the beginning
				m.viewport.GotoTop()
				m.beforeCursor = msg.cursor
			} else {
				// For newer logs, we want to show the end
				m.viewport.GotoBottom()
				m.afterCursor = msg.cursor
			}
		}
		return m, nil
	}

	// Handle list navigation
	if m.activeView == "list" {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		// Handle viewport navigation when showing logs
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	// Always show a split view
	listView := lipgloss.NewStyle().
		Width(m.list.Width()).
		Height(m.height).
		Render(m.list.View())

	// If the viewport isn't initialized yet, show an empty space
	var logsView string
	if m.viewportInitialized {
		logsView = lipgloss.NewStyle().
			Width(m.viewport.Width).
			Height(m.height).
			Render(m.viewport.View())
	} else {
		logsView = lipgloss.NewStyle().
			Width(m.width - m.list.Width()).
			Height(m.height).
			Render("Select a service to view logs")
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, logsView)
}

type logsMsg struct {
	logs    []byte
	cursor  string
	reverse bool
}

func fetchLogs(service string, reverse bool, cursor string) tea.Cmd {
	return func() tea.Msg {
		// Use short-iso output format instead of cat to keep timestamps
		// This format looks like: "2023-03-14 15:30:45 Log message here"
		args := []string{"journalctl", "-u", service, "-n", fmt.Sprintf("%d", logPageSize), "-o", "short-iso"}
		if cursor != "" {
			if reverse {
				args = append(args, "--before-cursor", cursor)
			} else {
				args = append(args, "--after-cursor", cursor)
			}
		}
		if reverse {
			args = append(args, "-r")
		}

		// Use context with timeout for the journalctl command
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			output = fmt.Appendf(nil, "Error fetching logs: %v", err)
		}

		// Handle potential encoding issues
		text := string(output)
		if !strings.HasPrefix(text, "\xef\xbb\xbf") {
			text = strings.ToValidUTF8(text, "")
		}

		cursor = extractCursor(output)
		return logsMsg{logs: []byte(text), cursor: cursor, reverse: reverse}
	}
}

func extractCursor(output []byte) string {
	lines := strings.Split(string(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "-- cursor:") {
			parts := strings.Split(lines[i], ": ")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func handleLogs() error {
	filters := []string{"saltbox_managed_"}

	services, err := getFilteredSystemdServices(filters)
	if err != nil {
		return fmt.Errorf("error getting systemd services: %w", err)
	}

	if len(services) == 0 {
		fmt.Println("No matching systemd services found.")
		return nil
	}

	// Convert string slice to list items
	items := make([]list.Item, len(services))
	for i, service := range services {
		items[i] = serviceItem{name: service}
	}

	// Create a list with styling
	listDelegate := list.NewDefaultDelegate()
	listDelegate.ShowDescription = false

	listModel := list.New(items, listDelegate, 0, 0)
	listModel.Title = "Systemd Services"
	listModel.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).UnsetBackground()
	listModel.SetShowStatusBar(false)
	listModel.SetFilteringEnabled(false)
	listModel.SetShowHelp(true)

	// Initial model
	initialModel := model{
		list:                listModel,
		serviceItems:        items,
		activeView:          "list",
		viewportInitialized: false,
	}

	// Run the program with the initial model
	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running logs UI: %w", err)
	}

	return nil
}

func getFilteredSystemdServices(filters []string) ([]string, error) {
	// Use context with timeout for systemctl command
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "list-unit-files", "--type=service", "--state=enabled")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list systemd services: %w", err)
	}

	var filteredServices []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ".service") && strings.Contains(line, "enabled") {
			serviceName := strings.Split(line, ".service")[0]
			for _, filter := range filters {
				if strings.HasPrefix(serviceName, filter) {
					filteredServices = append(filteredServices, serviceName)
					break
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning systemd services output failed: %w", err)
	}

	return filteredServices, nil
}
