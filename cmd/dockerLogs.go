package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/signals"
	"github.com/saltyorg/sb-go/internal/styles"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
)

// dockerLogsCmd represents the docker logs command
var dockerLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Display logs of Docker containers",
	Long:  `Displays a list of Docker containers and allows viewing their logs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleDockerLogs()
	},
}

func init() {
	dockerCmd.AddCommand(dockerLogsCmd)
}

const (
	dockerLogPageSize        = 500   // Number of log entries per page
	dockerPrefetchPagesAhead = 10    // Number of pages to stay ahead when prefetching
	dockerMaxBufferEntries   = 20000 // Maximum entries to keep in memory
	dockerViewportsAhead     = 5     // Prefetch when within 5 viewports of edge
	dockerViewportsToKeep    = 10    // Keep 10 viewports on each side when trimming
)

type containerItem struct {
	name      string
	id        string
	status    string // running, exited, etc.
	state     string // running (healthy), running, exited, etc. (with health if available)
	maxLength int    // Maximum name length for alignment
}

func (i containerItem) Title() string {
	statusIndicator := formatContainerStatus(i.status, i.state)
	if statusIndicator != "" {
		// Pad the name to align status indicators
		padding := max(i.maxLength-len(i.name), 0)
		return fmt.Sprintf("%s%s  %s", i.name, strings.Repeat(" ", padding), statusIndicator)
	}
	return i.name
}
func (i containerItem) Description() string { return "" }
func (i containerItem) FilterValue() string { return i.name }

// formatContainerStatus creates a colored status indicator for container status
func formatContainerStatus(status, state string) string {
	var symbol string
	var style lipgloss.Style

	switch status {
	case "running":
		symbol = "✓"
		style = styles.SuccessStyle
	case "exited":
		symbol = "✗"
		style = styles.ErrorStyle
	case "created", "paused":
		symbol = "○"
		style = styles.WarningStyle
	case "restarting", "removing", "dead":
		symbol = "⚠"
		style = styles.WarningStyle
	default:
		symbol = "?"
		style = styles.DimStyle
	}

	statusText := state

	return style.Render(fmt.Sprintf("[%s %s]", symbol, statusText))
}

// Key bindings for help
type dockerKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Toggle   key.Binding
	Follow   key.Binding
}

func (k dockerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Quit}
}

func (k dockerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Enter, k.Back, k.Quit},
	}
}

// ShortHelpForList returns help bindings for list view (no "back" option)
func (k dockerKeyMap) ShortHelpForList() []key.Binding {
	return []key.Binding{k.Enter, k.Quit}
}

// ShortHelpForLogs returns help bindings for logs view
func (k dockerKeyMap) ShortHelpForLogs() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Toggle, k.Follow, k.Back, k.Quit}
}

// ShortHelpForFollow returns help bindings for follow mode
func (k dockerKeyMap) ShortHelpForFollow() []key.Binding {
	return []key.Binding{k.Left, k.Right, k.Toggle, k.Follow, k.Back, k.Quit}
}

var dockerKeys = dockerKeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "scroll up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "scroll down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "scroll left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "scroll right"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "view logs"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back to list"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "previous page"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdown", "next page"),
	),
	Toggle: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "toggle timestamp/stream"),
	),
	Follow: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "toggle follow"),
	),
}

type dockerLogsModel struct {
	list                list.Model
	viewport            viewport.Model
	spinner             spinner.Model
	help                help.Model
	keys                dockerKeyMap
	containerItems      []list.Item
	selectedContainer   string
	selectedContainerID string
	logBuf              *dockerLogBuffer // Manages log entries and prefetching
	width               int
	height              int
	activeView          string
	viewportInitialized bool
	loading             bool
	err                 error
	viewportYPosition   int // Store viewport scroll position
	quitting            bool
	showTimestampStream bool // Toggle for showing timestamp and stream columns
	followMode          bool // Follow mode enabled
	dockerClient        *client.Client
}

func (m dockerLogsModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m dockerLogsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		helpHeight := lipgloss.Height(m.help.View(m.keys))

		if m.activeView == "list" {
			// Full-screen list view in alt screen mode
			m.list.SetWidth(msg.Width)
			m.list.SetHeight(msg.Height - helpHeight)
		} else {
			// Logs view - viewport uses full screen in alt screen mode
			if m.viewportInitialized {
				// Store current position before resize
				m.viewportYPosition = m.viewport.YOffset()

				m.viewport.SetWidth(msg.Width)
				m.viewport.SetHeight(msg.Height - helpHeight)

				// Restore scroll position after resize
				m.viewport.SetYOffset(min(m.viewportYPosition, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
			}
		}

		m.help.SetWidth(msg.Width)

		return m, nil

	case tea.KeyPressMsg:
		// Don't process navigation keys if loading (except quit)
		if m.loading && msg.String() != "q" && msg.String() != "ctrl+c" {
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			signals.GetGlobalManager().Shutdown(130)
			m.quitting = true
			// Clean up follow mode
			if m.followMode && m.logBuf != nil {
				m.logBuf.StopFollow()
			}
			return m, tea.Quit
		case "q":
			m.quitting = true
			// Clean up follow mode
			if m.followMode && m.logBuf != nil {
				m.logBuf.StopFollow()
			}
			return m, tea.Quit

		case "enter":
			if m.activeView == "list" {
				if i, ok := m.list.SelectedItem().(containerItem); ok {
					newContainer := i.name
					newContainerID := i.id
					m.activeView = "logs"

					// Initialize the viewport if necessary
					if !m.viewportInitialized {
						helpHeight := lipgloss.Height(m.help.View(m.keys))
						// Use full terminal width and height for fullscreen viewport
						m.viewport = viewport.New(viewport.WithWidth(m.width), viewport.WithHeight(m.height-helpHeight))
						m.viewport.Style = lipgloss.NewStyle().Padding(1, 2)
						m.viewportInitialized = true
					}

					// Only fetch new logs if a different container was selected
					if newContainer != m.selectedContainer {
						// Clean up old buffer before switching
						if m.logBuf != nil {
							m.logBuf.Cleanup()
						}

						m.selectedContainer = newContainer
						m.selectedContainerID = newContainerID
						m.loading = true
						m.err = nil
						m.viewportYPosition = 0
						m.followMode = false
						// Create new log buffer
						m.logBuf = newDockerLogBuffer(m.selectedContainerID, dockerPrefetchPagesAhead*dockerLogPageSize, m.dockerClient)
						return m, fetchDockerLogs(m.dockerClient, m.selectedContainerID, "", false, false)
					} else {
						// Make sure we re-apply the current log content with boundaries
						if m.logBuf != nil {
							m.viewport.SetContent(m.logBuf.GetContent(m.followMode))
						}
						return m, nil
					}
				}
			}

		case "esc":
			if m.activeView == "logs" && !m.loading {
				// Stop follow mode if active
				if m.followMode && m.logBuf != nil {
					m.logBuf.StopFollow()
					m.followMode = false
				}
				m.activeView = "list"
				m.err = nil // Clear any errors when going back
				return m, nil
			}

		case "f":
			// Toggle follow mode
			if m.activeView == "logs" && !m.loading && m.logBuf != nil {
				m.followMode = !m.followMode
				if m.followMode {
					// Enable follow mode - scroll to bottom and start background fetcher
					m.viewport.SetContent(m.logBuf.GetContentFormatted(m.showTimestampStream, m.followMode))
					m.viewport.GotoBottom()
					m.viewportYPosition = m.viewport.YOffset()
					cmds = append(cmds, m.logBuf.StartFollow())
				} else {
					// Disable follow mode - stop background fetcher
					m.logBuf.StopFollow()
					// Update content to show "end of logs" instead of "watching"
					m.viewport.SetContent(m.logBuf.GetContentFormatted(m.showTimestampStream, m.followMode))
				}
			}

		case "pgup", "u":
			// Disable scrolling if in follow mode
			if m.followMode {
				return m, nil
			}
			// Fetch more older logs if we're at the top of viewport and have more to load
			if m.activeView == "logs" && !m.loading && m.logBuf != nil {
				atTop := m.viewport.YOffset() <= 0
				if atTop && m.logBuf.beforeTimestamp != "" && m.logBuf.hasMoreBefore {
					m.loading = true
					m.err = nil
					return m, fetchDockerLogs(m.dockerClient, m.selectedContainerID, m.logBuf.beforeTimestamp, true, false)
				}
				// Otherwise, let the viewport handle scrolling
			}

		case "pgdown", "d":
			// Disable scrolling if in follow mode
			if m.followMode {
				return m, nil
			}
			// Only fetch more logs if we're at the bottom of viewport and have more to load
			if m.activeView == "logs" && !m.loading && m.logBuf != nil {
				atBottom := m.viewport.YOffset() >= m.viewport.TotalLineCount()-m.viewport.Height()
				if atBottom && m.logBuf.afterTimestamp != "" && m.logBuf.hasMoreAfter {
					m.loading = true
					m.err = nil
					return m, fetchDockerLogs(m.dockerClient, m.selectedContainerID, m.logBuf.afterTimestamp, false, false)
				}
				// Otherwise, let the viewport handle scrolling
			}

		case "left":
			// Scroll viewport left for long lines (allowed in follow mode)
			if m.activeView == "logs" && m.viewportInitialized {
				m.viewport.ScrollLeft(10)
			}

		case "right":
			// Scroll viewport right for long lines (allowed in follow mode)
			if m.activeView == "logs" && m.viewportInitialized {
				m.viewport.ScrollRight(10)
			}

		case "t":
			// Toggle timestamp and stream visibility (allowed in follow mode)
			if m.activeView == "logs" && m.logBuf != nil {
				m.showTimestampStream = !m.showTimestampStream
				// Update viewport content with new formatting
				m.viewport.SetContent(m.logBuf.GetContentFormatted(m.showTimestampStream, m.followMode))
				// If in follow mode, scroll back to bottom after refresh
				if m.followMode {
					m.viewport.GotoBottom()
					m.viewportYPosition = m.viewport.YOffset()
				}
			}
		}

	case dockerLogsMsg:
		if msg.err != nil {
			m.err = msg.err
			if m.logBuf != nil {
				if msg.reverse {
					m.logBuf.prefetching = false
				} else {
					m.logBuf.prefetchingAfter = false
				}
			}
			m.loading = false
		} else {
			m.err = nil

			// Clear loading flag
			if msg.isPrefetch {
				if m.logBuf != nil {
					if msg.reverse {
						m.logBuf.prefetching = false
					} else {
						m.logBuf.prefetchingAfter = false
					}
				}
			} else {
				m.loading = false
			}

			if m.activeView == "logs" && m.logBuf != nil {
				if len(msg.entries) == 0 {
					// No entries returned - we hit a boundary
					if msg.reverse {
						m.logBuf.hasMoreBefore = false
						m.logBuf.prefetching = false
					} else {
						m.logBuf.hasMoreAfter = false
						m.logBuf.prefetchingAfter = false
					}
					// Don't update display, just stop loading
					return m, nil
				}

				if msg.reverse {
					// Prepend older logs
					oldLen := len(m.logBuf.entries)

					// Save current viewport position before updating content
					savedYOffset := m.viewport.YOffset()

					// Use firstTimestamp (oldest entry) to continue fetching even older logs
					prefetchCmd := m.logBuf.PrependOlder(msg.entries, msg.firstTimestamp, msg.hasMore)

					// Calculate how many lines were added
					entriesAdded := len(m.logBuf.entries) - oldLen
					linesAdded := 0
					for i := range entriesAdded {
						linesAdded += len(strings.Split(formatDockerLogEntry(m.logBuf.entries[i], m.showTimestampStream), "\n"))
					}
					// Add boundary markers if present (only if this update caused hasMoreBefore to become false)
					if !m.logBuf.hasMoreBefore && msg.hasMore {
						// Boundary marker was just added
						linesAdded += 2 // "--- start of logs ---" + blank line
					}

					// Update viewport content
					m.viewport.SetContent(m.logBuf.GetContent(m.followMode))

					// ALWAYS adjust viewport position when prepending to prevent scroll jumping
					// This keeps the user's view stable regardless of prefetch or user action
					m.viewport.SetYOffset(min(savedYOffset+linesAdded, m.viewport.TotalLineCount()-m.viewport.Height()))
					m.viewportYPosition = m.viewport.YOffset()

					// Check if logBuffer wants to prefetch more
					if prefetchCmd != nil {
						cmds = append(cmds, prefetchCmd)
					}
				} else {
					if len(m.logBuf.entries) == 0 {
						// Initial load
						prefetchCmd := m.logBuf.AppendInitial(msg.entries, msg.firstTimestamp, msg.lastTimestamp)

						// Update viewport and position at bottom
						m.viewport.SetContent(m.logBuf.GetContent(m.followMode))
						m.viewport.GotoBottom()
						m.viewportYPosition = m.viewport.YOffset()

						// Start prefetching if logBuffer wants to
						if prefetchCmd != nil {
							cmds = append(cmds, prefetchCmd)
						}
					} else {
						// Append newer logs
						prefetchCmd := m.logBuf.AppendNewer(msg.entries, msg.lastTimestamp, msg.hasMore)

						// Update viewport content
						m.viewport.SetContent(m.logBuf.GetContent(m.followMode))

						if !msg.isPrefetch || m.followMode {
							// User-initiated or follow mode: go to bottom
							m.viewport.GotoBottom()
							m.viewportYPosition = m.viewport.YOffset()
						}
						// For prefetch: viewport stays where it is

						// Check if logBuffer wants to prefetch more
						if prefetchCmd != nil {
							cmds = append(cmds, prefetchCmd)
						}
					}
				}
			}
		}
		return m, tea.Batch(cmds...)

	case followTickMsg:
		// Follow mode tick - fetch new logs
		if m.followMode && m.logBuf != nil && !m.loading {
			// Fetch new logs since last timestamp
			cmds = append(cmds, fetchDockerLogs(m.dockerClient, m.selectedContainerID, m.logBuf.afterTimestamp, false, true))
		}
		// Schedule next tick
		if m.followMode {
			cmds = append(cmds, tickFollow())
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.loading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Handle list navigation
	if m.activeView == "list" && !m.loading {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.activeView == "logs" && !m.loading && !m.followMode {
		// Handle viewport navigation when showing logs (disabled in follow mode)
		oldYOffset := m.viewport.YOffset()
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Track position changes
		m.viewportYPosition = m.viewport.YOffset()

		// Check if viewport position changed and we need to prefetch or trim
		if m.logBuf != nil && oldYOffset != m.viewport.YOffset() {
			// Check if we need to prefetch more logs based on viewport position
			prefetchCmds := m.logBuf.CheckPrefetchNeeds(
				m.viewport.YOffset(),
				m.viewport.Height(),
				m.viewport.TotalLineCount(),
			)
			if len(prefetchCmds) > 0 {
				cmds = append(cmds, prefetchCmds...)
			}

			// Trim buffer if it's too large
			linesTrimmed := m.logBuf.TrimBuffer(m.viewport.YOffset(), m.viewport.Height())
			if linesTrimmed > 0 {
				// Update viewport content after trimming
				m.viewport.SetContent(m.logBuf.GetContent(m.followMode))
				// Adjust viewport position to account for trimmed lines
				m.viewport.SetYOffset(max(0, m.viewport.YOffset()-linesTrimmed))
				m.viewportYPosition = m.viewport.YOffset()
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m dockerLogsModel) View() tea.View {
	// If quitting, return empty string to clean up viewport
	if m.quitting {
		return tea.NewView("")
	}

	// Get context-aware help based on active view
	var helpView string
	if m.activeView == "list" {
		helpView = m.help.ShortHelpView(m.keys.ShortHelpForList())
	} else if m.followMode {
		helpView = m.help.ShortHelpView(m.keys.ShortHelpForFollow())
	} else {
		helpView = m.help.ShortHelpView(m.keys.ShortHelpForLogs())
	}

	if m.activeView == "list" {
		// Inline list view - render list with help at bottom
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, m.list.View(), helpView))
	}

	// Fullscreen logs view (in alt screen)
	var logsContent string

	if m.err != nil {
		// Show error with styling
		errorMsg := styles.ErrorStyle.Render("Error: " + m.err.Error())
		logsContent = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - lipgloss.Height(helpView)).
			Padding(2).
			Render(errorMsg)
	} else if m.loading && len(m.logBuf.entries) == 0 {
		// Show loading spinner (only if no logs loaded yet)
		loadingMsg := fmt.Sprintf("%s Loading logs for %s...",
			m.spinner.View(),
			styles.InfoStyle.Render(m.selectedContainer))
		logsContent = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - lipgloss.Height(helpView)).
			Padding(2).
			Render(loadingMsg)
	} else if m.viewportInitialized && m.selectedContainer != "" {
		// Show viewport with logs
		logsContent = m.viewport.View()
	} else {
		// Show prompt to select a container
		promptMsg := styles.DimStyle.Render("Select a container to view logs")
		logsContent = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - lipgloss.Height(helpView)).
			Padding(2).
			Render(promptMsg)
	}

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, logsContent, helpView))
	v.AltScreen = true
	return v
}

// formatDockerLogEntriesWithBoundaries formats log entries with boundary indicators inline
func formatDockerLogEntriesWithBoundaries(entries []dockerLogEntry, hasMoreBefore, hasMoreAfter bool, showTimestampStream bool, followMode bool) string {
	if len(entries) == 0 {
		return "No log entries"
	}

	var lines []string

	// Add start indicator at the beginning if we've hit the start boundary
	if !hasMoreBefore {
		lines = append(lines, styles.DimStyle.Render("--- start of logs ---"))
		lines = append(lines, "")
	}

	// Add all log entries
	for _, entry := range entries {
		lines = append(lines, formatDockerLogEntry(entry, showTimestampStream))
	}

	// Add end indicator at the end if we've hit the end boundary
	if !hasMoreAfter {
		lines = append(lines, "")
		if followMode {
			lines = append(lines, styles.InfoStyle.Render("--- watching for new logs (press 'f' to disable) ---"))
		} else {
			lines = append(lines, styles.DimStyle.Render("--- end of logs ---"))
		}
	}

	return strings.Join(lines, "\n")
}

// formatDockerLogEntry formats a single log entry for display
func formatDockerLogEntry(entry dockerLogEntry, showTimestampStream bool) string {
	if showTimestampStream {
		// Format: timestamp stream │ message
		return fmt.Sprintf("%s %6s │ %s", entry.timestamp, entry.stream, entry.message)
	} else {
		// Simplified format: just the message (no timestamp, stream, or divider)
		return entry.message
	}
}

type dockerLogsMsg struct {
	entries        []dockerLogEntry // Parsed log entries
	firstTimestamp string           // First timestamp in the result (for bidirectional nav)
	lastTimestamp  string           // Last timestamp in the result
	reverse        bool
	hasMore        bool // Whether there are more entries in this direction
	isPrefetch     bool // Whether this is a background prefetch request
	err            error
}

type dockerLogEntry struct {
	timestamp string // RFC3339Nano format
	stream    string // stdout or stderr
	message   string
}

// followTickMsg is sent periodically when follow mode is active
type followTickMsg struct{}

func tickFollow() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return followTickMsg{}
	})
}

// dockerLogBuffer manages log entries and handles prefetching
type dockerLogBuffer struct {
	entries          []dockerLogEntry
	beforeTimestamp  string // Timestamp for fetching older logs
	afterTimestamp   string // Timestamp for fetching newer logs
	hasMoreBefore    bool
	hasMoreAfter     bool
	containerID      string
	prefetching      bool
	prefetchingAfter bool
	targetSize       int // Target number of entries to keep loaded
	dockerClient     *client.Client
	followActive     bool
}

func newDockerLogBuffer(containerID string, targetSize int, dockerClient *client.Client) *dockerLogBuffer {
	return &dockerLogBuffer{
		entries:       []dockerLogEntry{},
		containerID:   containerID,
		targetSize:    targetSize,
		hasMoreBefore: true,
		hasMoreAfter:  false,
		dockerClient:  dockerClient,
		followActive:  false,
	}
}

// GetContent returns formatted content for display with boundary markers (timestamp/stream shown)
func (lb *dockerLogBuffer) GetContent(followMode bool) string {
	return formatDockerLogEntriesWithBoundaries(lb.entries, lb.hasMoreBefore, lb.hasMoreAfter, true, followMode)
}

// GetContentFormatted returns formatted content with optional timestamp/stream visibility
func (lb *dockerLogBuffer) GetContentFormatted(showTimestampStream bool, followMode bool) string {
	return formatDockerLogEntriesWithBoundaries(lb.entries, lb.hasMoreBefore, lb.hasMoreAfter, showTimestampStream, followMode)
}

// ShouldPrefetch returns true if we need to fetch more older logs
func (lb *dockerLogBuffer) ShouldPrefetch() bool {
	return len(lb.entries) < lb.targetSize && lb.hasMoreBefore && lb.beforeTimestamp != "" && !lb.prefetching
}

// StartPrefetch marks prefetch as in progress and returns the fetch command
func (lb *dockerLogBuffer) StartPrefetch() tea.Cmd {
	if !lb.ShouldPrefetch() {
		return nil
	}
	lb.prefetching = true
	return fetchDockerLogs(lb.dockerClient, lb.containerID, lb.beforeTimestamp, true, true)
}

// AppendInitial sets initial logs (most recent)
func (lb *dockerLogBuffer) AppendInitial(entries []dockerLogEntry, firstTimestamp, lastTimestamp string) tea.Cmd {
	lb.entries = entries
	lb.beforeTimestamp = firstTimestamp
	lb.afterTimestamp = lastTimestamp
	lb.hasMoreBefore = true
	lb.hasMoreAfter = false
	return lb.StartPrefetch()
}

// PrependOlder adds older logs to the beginning
// oldestTimestamp should be the timestamp of the oldest entry in the batch (to fetch even older logs)
func (lb *dockerLogBuffer) PrependOlder(entries []dockerLogEntry, oldestTimestamp string, hasMore bool) tea.Cmd {
	lb.entries = append(entries, lb.entries...)
	lb.beforeTimestamp = oldestTimestamp
	lb.hasMoreBefore = hasMore
	lb.prefetching = false
	return lb.StartPrefetch()
}

// AppendNewer adds newer logs to the end
func (lb *dockerLogBuffer) AppendNewer(entries []dockerLogEntry, lastTimestamp string, hasMore bool) tea.Cmd {
	lb.entries = append(lb.entries, entries...)
	lb.afterTimestamp = lastTimestamp
	lb.hasMoreAfter = hasMore
	lb.prefetchingAfter = false
	return nil
}

// StartFollow enables follow mode and returns the tick command
func (lb *dockerLogBuffer) StartFollow() tea.Cmd {
	lb.followActive = true
	return tickFollow()
}

// StopFollow disables follow mode
func (lb *dockerLogBuffer) StopFollow() {
	lb.followActive = false
}

// Cleanup clears all log entries and resets state for memory cleanup
func (lb *dockerLogBuffer) Cleanup() {
	lb.entries = nil
	lb.beforeTimestamp = ""
	lb.afterTimestamp = ""
	lb.hasMoreBefore = false
	lb.hasMoreAfter = false
	lb.prefetching = false
	lb.prefetchingAfter = false
	lb.followActive = false
}

// TrimBuffer removes entries far from viewport to limit memory usage
// Returns the number of lines trimmed from the top (for viewport offset adjustment)
func (lb *dockerLogBuffer) TrimBuffer(viewportY, viewportHeight int) int {
	if len(lb.entries) <= dockerMaxBufferEntries {
		return 0 // No need to trim
	}

	// Calculate viewport position in terms of log entries
	// We need to estimate which entries are visible
	totalLines := 0
	visibleStartEntry := 0
	visibleEndEntry := len(lb.entries) - 1

	// Find which entries correspond to the viewport position
	for i, entry := range lb.entries {
		// Use true for timestamp/stream since this is just for line counting
		entryLines := len(strings.Split(formatDockerLogEntry(entry, true), "\n"))
		if totalLines+entryLines > viewportY {
			visibleStartEntry = i
			break
		}
		totalLines += entryLines
	}

	// Calculate how many entries to keep on each side
	entriesToKeep := max(
		// Rough estimate: ~3 lines per entry
		dockerViewportsToKeep*viewportHeight/3,
		// Keep at least one page
		dockerLogPageSize)

	// Calculate trim boundaries
	trimStart := max(0, visibleStartEntry-entriesToKeep)
	trimEnd := min(len(lb.entries), visibleEndEntry+entriesToKeep)

	// Don't trim if we're not actually removing much
	if trimStart < 100 && trimEnd > len(lb.entries)-100 {
		return 0 // Not worth trimming
	}

	// Calculate lines being removed from the top
	linesTrimmed := 0
	for i := range trimStart {
		// Use true for timestamp/stream since this is just for line counting
		linesTrimmed += len(strings.Split(formatDockerLogEntry(lb.entries[i], true), "\n"))
	}

	// Trim the entries
	oldEntries := lb.entries
	lb.entries = make([]dockerLogEntry, trimEnd-trimStart)
	copy(lb.entries, oldEntries[trimStart:trimEnd])

	// Update timestamps if we trimmed from edges
	if trimStart > 0 && len(lb.entries) > 0 {
		lb.beforeTimestamp = lb.entries[0].timestamp
		lb.hasMoreBefore = true // We know there are more entries we trimmed
	}
	if trimEnd < len(oldEntries) && len(lb.entries) > 0 {
		lb.afterTimestamp = lb.entries[len(lb.entries)-1].timestamp
		lb.hasMoreAfter = true // We know there are more entries we trimmed
	}

	return linesTrimmed
}

// CheckPrefetchNeeds checks if prefetching should be triggered based on viewport position
// Returns commands for prefetching in both directions if needed
func (lb *dockerLogBuffer) CheckPrefetchNeeds(viewportY, viewportHeight, totalHeight int) []tea.Cmd {
	var cmds []tea.Cmd

	// Calculate threshold for prefetching (viewportsAhead * viewport height)
	prefetchThreshold := dockerViewportsAhead * viewportHeight

	// Check if we should prefetch older logs (scrolling near top)
	if viewportY < prefetchThreshold && lb.hasMoreBefore && lb.beforeTimestamp != "" && !lb.prefetching {
		lb.prefetching = true
		cmds = append(cmds, fetchDockerLogs(lb.dockerClient, lb.containerID, lb.beforeTimestamp, true, true))
	}

	// Check if we should prefetch newer logs (scrolling near bottom)
	distanceFromBottom := totalHeight - (viewportY + viewportHeight)
	if distanceFromBottom < prefetchThreshold && lb.hasMoreAfter && lb.afterTimestamp != "" && !lb.prefetchingAfter {
		lb.prefetchingAfter = true
		cmds = append(cmds, fetchDockerLogs(lb.dockerClient, lb.containerID, lb.afterTimestamp, false, true))
	}

	return cmds
}

func fetchDockerLogs(cli *client.Client, containerID string, timestamp string, reverse bool, isPrefetch bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		options := client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Timestamps: true,
		}

		if timestamp != "" {
			if reverse {
				// Fetch logs before this timestamp (older logs)
				options.Until = timestamp
				options.Tail = fmt.Sprintf("%d", dockerLogPageSize)
			} else {
				// Fetch logs after this timestamp (newer logs)
				options.Since = timestamp
				// Don't use Tail for forward fetching to get all new logs
			}
		} else {
			// No timestamp - show most recent entries
			options.Tail = fmt.Sprintf("%d", dockerLogPageSize)
		}

		logsReader, err := cli.ContainerLogs(ctx, containerID, options)
		if err != nil {
			return dockerLogsMsg{isPrefetch: isPrefetch, err: fmt.Errorf("failed to fetch logs: %w", err)}
		}
		defer func() { _ = logsReader.Close() }()

		// Parse Docker log format
		entries, err := parseDockerLogs(logsReader)
		if err != nil {
			return dockerLogsMsg{isPrefetch: isPrefetch, err: fmt.Errorf("failed to parse logs: %w", err)}
		}

		// Filter out the timestamp entry if fetching since/until a specific time
		if timestamp != "" && len(entries) > 0 {
			// Remove entries with exact matching timestamp to avoid duplicates
			filtered := entries[:0]
			for _, entry := range entries {
				if entry.timestamp != timestamp {
					filtered = append(filtered, entry)
				}
			}
			entries = filtered
		}

		// If no entries returned, we've hit a boundary
		if len(entries) == 0 {
			return dockerLogsMsg{
				entries:        nil,
				firstTimestamp: timestamp,
				lastTimestamp:  timestamp,
				reverse:        reverse,
				hasMore:        false,
				isPrefetch:     isPrefetch,
				err:            nil,
			}
		}

		// Docker returns logs in chronological order (oldest to newest)
		// For reverse mode, we need newest to oldest, so reverse the slice
		if reverse {
			for i := 0; i < len(entries)/2; i++ {
				entries[i], entries[len(entries)-1-i] = entries[len(entries)-1-i], entries[i]
			}
		}

		// Determine if there are more entries available
		// If we got fewer entries than requested, we've hit a boundary
		hasMore := len(entries) >= dockerLogPageSize

		// Extract timestamps
		var firstTimestamp, lastTimestamp string
		if reverse {
			// In reverse mode, entries are newest→oldest after reversal
			// Last entry is the oldest (to fetch even older logs)
			firstTimestamp = entries[len(entries)-1].timestamp
			lastTimestamp = entries[0].timestamp
		} else {
			// In forward mode, entries are oldest→newest
			firstTimestamp = entries[0].timestamp
			lastTimestamp = entries[len(entries)-1].timestamp
		}

		// Normalize to oldest→newest for buffer storage
		if reverse {
			// Reverse back to oldest→newest for storage
			for i := 0; i < len(entries)/2; i++ {
				entries[i], entries[len(entries)-1-i] = entries[len(entries)-1-i], entries[i]
			}
		}

		return dockerLogsMsg{
			entries:        entries,
			firstTimestamp: firstTimestamp,
			lastTimestamp:  lastTimestamp,
			reverse:        reverse,
			hasMore:        hasMore,
			isPrefetch:     isPrefetch,
			err:            nil,
		}
	}
}

// parseDockerLogs parses Docker log format from the reader
// Docker uses stdcopy multiplexing for stdout/stderr streams
func parseDockerLogs(reader io.Reader) ([]dockerLogEntry, error) {
	var entries []dockerLogEntry

	// Create buffers for demultiplexed streams
	var stdout, stderr strings.Builder

	// Demultiplex the streams
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to demultiplex streams: %w", err)
	}

	// Process stdout entries
	if stdout.Len() > 0 {
		scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse timestamp from line if present
			// Docker format with timestamps: "2025-11-12T14:23:45.123456789Z message"
			parts := strings.SplitN(line, " ", 2)
			var timestamp, msg string
			if len(parts) == 2 {
				// Try to parse as timestamp
				if _, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
					timestamp = parts[0]
					msg = parts[1]
				} else {
					// Not a timestamp, treat whole line as message
					timestamp = time.Now().Format(time.RFC3339Nano)
					msg = line
				}
			} else {
				timestamp = time.Now().Format(time.RFC3339Nano)
				msg = line
			}

			entries = append(entries, dockerLogEntry{
				timestamp: timestamp,
				stream:    "stdout",
				message:   msg,
			})
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	// Process stderr entries
	if stderr.Len() > 0 {
		scanner := bufio.NewScanner(strings.NewReader(stderr.String()))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse timestamp from line if present
			parts := strings.SplitN(line, " ", 2)
			var timestamp, msg string
			if len(parts) == 2 {
				// Try to parse as timestamp
				if _, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
					timestamp = parts[0]
					msg = parts[1]
				} else {
					// Not a timestamp, treat whole line as message
					timestamp = time.Now().Format(time.RFC3339Nano)
					msg = line
				}
			} else {
				timestamp = time.Now().Format(time.RFC3339Nano)
				msg = line
			}

			entries = append(entries, dockerLogEntry{
				timestamp: timestamp,
				stream:    "stderr",
				message:   msg,
			})
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	// Sort entries by timestamp to maintain chronological order
	// (since stdout and stderr are processed separately)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp < entries[j].timestamp
	})

	return entries, nil
}

func handleDockerLogs() error {
	ctx := context.Background()
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer func() { _ = cli.Close() }()

	containersSummary, err := cli.ContainerList(ctx, client.ContainerListOptions{All: false})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containersSummary.Items) == 0 {
		fmt.Println("No running containers found.")
		return nil
	}

	// Calculate maximum container name length for alignment
	maxNameLength := 0
	for _, c := range containersSummary.Items {
		name := c.Names[0][1:] // Remove leading slash
		if len(name) > maxNameLength {
			maxNameLength = len(name)
		}
	}

	// Convert containers to list items
	items := make([]list.Item, len(containersSummary.Items))
	for i, c := range containersSummary.Items {
		name := c.Names[0][1:] // Remove leading slash
		// Use the simple string status for now (e.g., "Up 2 hours (healthy)")
		// The State field is a complex struct and we'd need to inspect for details
		statusDisplay := c.Status

		// Extract simple state from status string for coloring
		simpleState := "running"
		if strings.Contains(strings.ToLower(statusDisplay), "exited") {
			simpleState = "exited"
		} else if strings.Contains(strings.ToLower(statusDisplay), "created") {
			simpleState = "created"
		} else if strings.Contains(strings.ToLower(statusDisplay), "paused") {
			simpleState = "paused"
		}

		items[i] = containerItem{
			name:      name,
			id:        c.ID,
			status:    simpleState,
			state:     statusDisplay,
			maxLength: maxNameLength,
		}
	}

	// Sort by name
	sort.Slice(items, func(i, j int) bool {
		return items[i].(containerItem).name < items[j].(containerItem).name
	})

	// Create a list with styling for inline display
	listDelegate := list.NewDefaultDelegate()
	listDelegate.ShowDescription = false

	// Initialize with 0, 0 like the example - will be sized in WindowSizeMsg
	listModel := list.New(items, listDelegate, 0, 0)
	listModel.Title = "Docker Containers"
	listModel.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).UnsetBackground()
	listModel.SetFilteringEnabled(false)
	listModel.SetShowHelp(false) // We'll use our own help

	// Enable pagination to show dots
	listModel.SetShowPagination(true)
	listModel.SetShowStatusBar(true)

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Initialize help
	h := help.New()
	h.ShowAll = false

	// Initial model
	initialModel := dockerLogsModel{
		list:                listModel,
		spinner:             s,
		help:                h,
		keys:                dockerKeys,
		containerItems:      items,
		activeView:          "list",
		viewportInitialized: false,
		loading:             false,
		err:                 nil,
		showTimestampStream: true, // Show timestamp/stream by default
		followMode:          false,
		dockerClient:        cli,
	}

	// Run the program with the initial model.
	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running docker logs UI: %w", err)
	}

	return nil
}
