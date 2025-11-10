package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/styles"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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

const (
	logPageSize        = 500   // Number of log entries per page
	prefetchPagesAhead = 10    // Number of pages to stay ahead when prefetching
	maxBufferEntries   = 20000 // Maximum entries to keep in memory
	viewportsAhead     = 5     // Prefetch when within 5 viewports of edge
	viewportsToKeep    = 10    // Keep 10 viewports on each side when trimming
)

type serviceItem struct {
	name string
}

func (i serviceItem) Title() string       { return i.name }
func (i serviceItem) Description() string { return "" }
func (i serviceItem) FilterValue() string { return i.name }

// Key bindings for help
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Enter, k.Back, k.Quit},
	}
}

// ShortHelpForList returns help bindings for list view (no "back" option)
func (k keyMap) ShortHelpForList() []key.Binding {
	return []key.Binding{k.Enter, k.Quit}
}

// ShortHelpForLogs returns help bindings for logs view
func (k keyMap) ShortHelpForLogs() []key.Binding {
	return []key.Binding{k.Back, k.Quit}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "scroll up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "scroll down"),
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
}

type model struct {
	list                list.Model
	viewport            viewport.Model
	spinner             spinner.Model
	help                help.Model
	keys                keyMap
	serviceItems        []list.Item
	selectedService     string
	logBuf              *logBuffer // Manages log entries and prefetching
	width               int
	height              int
	activeView          string
	viewportInitialized bool
	loading             bool
	err                 error
	viewportYPosition   int // Store viewport scroll position
	quitting            bool
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		helpHeight := lipgloss.Height(m.help.View(m.keys))

		if m.activeView == "list" {
			// Inline list view - use full width, compact height
			m.list.SetWidth(msg.Width)
			m.list.SetHeight(15) // Compact height for inline display
		} else {
			// Logs view - viewport uses full screen in alt screen mode
			if m.viewportInitialized {
				// Store current position before resize
				m.viewportYPosition = m.viewport.YOffset

				m.viewport.Width = msg.Width
				m.viewport.Height = msg.Height - helpHeight

				// Restore scroll position after resize
				m.viewport.YOffset = min(m.viewportYPosition, max(0, m.viewport.TotalLineCount()-m.viewport.Height))
			}
		}

		m.help.Width = msg.Width

		return m, nil

	case tea.KeyMsg:
		// Don't process navigation keys if loading
		if m.loading && msg.String() != "q" && msg.String() != "ctrl+c" {
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			// If we're in logs view (alt screen), exit alt screen before quitting
			if m.activeView == "logs" {
				return m, tea.Sequence(tea.ExitAltScreen, tea.Quit)
			}
			return m, tea.Quit

		case "enter":
			if m.activeView == "list" {
				if i, ok := m.list.SelectedItem().(serviceItem); ok {
					newService := i.name
					m.activeView = "logs"

					// Initialize the viewport if necessary
					if !m.viewportInitialized {
						helpHeight := lipgloss.Height(m.help.View(m.keys))
						// Use full terminal width and height for fullscreen viewport
						m.viewport = viewport.New(m.width, m.height-helpHeight)
						m.viewport.Style = lipgloss.NewStyle().Padding(1, 2)
						m.viewportInitialized = true
					}

					// Only fetch new logs if a different service was selected
					if newService != m.selectedService {
						// Clean up old buffer before switching
						if m.logBuf != nil {
							m.logBuf.Cleanup()
						}

						m.selectedService = newService
						m.loading = true
						m.err = nil
						m.viewportYPosition = 0
						// Create new log buffer with target size of 10 pages
						m.logBuf = newLogBuffer(m.selectedService, prefetchPagesAhead*logPageSize)
						// Enter alt screen and fetch logs
						return m, tea.Batch(tea.EnterAltScreen, fetchLogs(m.selectedService, false, "", false))
					} else {
						// Make sure we re-apply the current log content with boundaries
						if m.logBuf != nil {
							m.viewport.SetContent(m.logBuf.GetContent())
						}
						// Enter alt screen to view existing logs
						return m, tea.EnterAltScreen
					}
				}
			}

		case "esc":
			if m.activeView == "logs" && !m.loading {
				m.activeView = "list"
				m.err = nil // Clear any errors when going back
				// Exit alt screen and return to inline list view
				return m, tea.ExitAltScreen
			}

		case "pgup", "u":
			// Fetch more older logs if we're at the top of viewport and have more to load
			if m.activeView == "logs" && !m.loading && m.logBuf != nil {
				atTop := m.viewport.YOffset <= 0
				if atTop && m.logBuf.beforeCursor != "" && m.logBuf.hasMoreBefore {
					m.loading = true
					m.err = nil
					return m, fetchLogs(m.selectedService, true, m.logBuf.beforeCursor, false)
				}
				// Otherwise, let the viewport handle scrolling
			}

		case "pgdown", "d":
			// Only fetch more logs if we're at the bottom of viewport and have more to load
			if m.activeView == "logs" && !m.loading && m.logBuf != nil {
				atBottom := m.viewport.YOffset >= m.viewport.TotalLineCount()-m.viewport.Height
				if atBottom && m.logBuf.afterCursor != "" && m.logBuf.hasMoreAfter {
					m.loading = true
					m.err = nil
					return m, fetchLogs(m.selectedService, false, m.logBuf.afterCursor, false)
				}
				// Otherwise, let the viewport handle scrolling
			}

		case "left":
			// Scroll viewport left for long lines
			if m.activeView == "logs" && m.viewportInitialized {
				m.viewport.ScrollLeft(10)
			}

		case "right":
			// Scroll viewport right for long lines
			if m.activeView == "logs" && m.viewportInitialized {
				m.viewport.ScrollRight(10)
			}
		}

	case logsMsg:
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
					savedYOffset := m.viewport.YOffset

					// Use firstCursor (oldest entry) to continue fetching even older logs
					prefetchCmd := m.logBuf.PrependOlder(msg.entries, msg.firstCursor, msg.hasMore)

					// Calculate how many lines were added
					entriesAdded := len(m.logBuf.entries) - oldLen
					linesAdded := 0
					for i := range entriesAdded {
						linesAdded += len(strings.Split(formatLogEntry(m.logBuf.entries[i]), "\n"))
					}
					// Add boundary markers if present (only if this update caused hasMoreBefore to become false)
					if !m.logBuf.hasMoreBefore && msg.hasMore {
						// Boundary marker was just added
						linesAdded += 2 // "--- start of logs ---" + blank line
					}

					// Update viewport content
					m.viewport.SetContent(m.logBuf.GetContent())

					// ALWAYS adjust viewport position when prepending to prevent scroll jumping
					// This keeps the user's view stable regardless of prefetch or user action
					m.viewport.YOffset = min(savedYOffset+linesAdded, m.viewport.TotalLineCount()-m.viewport.Height)
					m.viewportYPosition = m.viewport.YOffset

					// Check if logBuffer wants to prefetch more
					if prefetchCmd != nil {
						cmds = append(cmds, prefetchCmd)
					}
				} else {
					if len(m.logBuf.entries) == 0 {
						// Initial load
						prefetchCmd := m.logBuf.AppendInitial(msg.entries, msg.firstCursor, msg.lastCursor)

						// Update viewport and position at bottom
						m.viewport.SetContent(m.logBuf.GetContent())
						m.viewport.GotoBottom()
						m.viewportYPosition = m.viewport.YOffset

						// Start prefetching if logBuffer wants to
						if prefetchCmd != nil {
							cmds = append(cmds, prefetchCmd)
						}
					} else {
						// Append newer logs
						prefetchCmd := m.logBuf.AppendNewer(msg.entries, msg.lastCursor, msg.hasMore)

						// Update viewport content
						m.viewport.SetContent(m.logBuf.GetContent())

						if !msg.isPrefetch {
							// User-initiated: go to bottom
							m.viewport.GotoBottom()
							m.viewportYPosition = m.viewport.YOffset
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
	} else if m.activeView == "logs" && !m.loading {
		// Handle viewport navigation when showing logs
		oldYOffset := m.viewport.YOffset
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Track position changes
		m.viewportYPosition = m.viewport.YOffset

		// Check if viewport position changed and we need to prefetch or trim
		if m.logBuf != nil && oldYOffset != m.viewport.YOffset {
			// Check if we need to prefetch more logs based on viewport position
			prefetchCmds := m.logBuf.CheckPrefetchNeeds(
				m.viewport.YOffset,
				m.viewport.Height,
				m.viewport.TotalLineCount(),
			)
			if len(prefetchCmds) > 0 {
				cmds = append(cmds, prefetchCmds...)
			}

			// Trim buffer if it's too large
			linesTrimmed := m.logBuf.TrimBuffer(m.viewport.YOffset, m.viewport.Height)
			if linesTrimmed > 0 {
				// Update viewport content after trimming
				m.viewport.SetContent(m.logBuf.GetContent())
				// Adjust viewport position to account for trimmed lines
				m.viewport.YOffset = max(0, m.viewport.YOffset-linesTrimmed)
				m.viewportYPosition = m.viewport.YOffset
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	// If quitting, return empty string to clean up viewport
	if m.quitting {
		return ""
	}

	// Get context-aware help based on active view
	var helpView string
	if m.activeView == "list" {
		helpView = m.help.ShortHelpView(m.keys.ShortHelpForList())
	} else {
		helpView = m.help.ShortHelpView(m.keys.ShortHelpForLogs())
	}

	if m.activeView == "list" {
		// Inline list view - render list with help at bottom
		return lipgloss.JoinVertical(lipgloss.Left, m.list.View(), helpView)
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
	} else if m.loading {
		// Show loading spinner
		loadingMsg := fmt.Sprintf("%s Loading logs for %s...",
			m.spinner.View(),
			styles.InfoStyle.Render(m.selectedService))
		logsContent = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - lipgloss.Height(helpView)).
			Padding(2).
			Render(loadingMsg)
	} else if m.viewportInitialized && m.selectedService != "" {
		// Show viewport with logs
		logsContent = m.viewport.View()
	} else {
		// Show prompt to select a service
		promptMsg := styles.DimStyle.Render("Select a service to view logs")
		logsContent = lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - lipgloss.Height(helpView)).
			Padding(2).
			Render(promptMsg)
	}

	return lipgloss.JoinVertical(lipgloss.Left, logsContent, helpView)
}

// formatLogEntriesWithBoundaries formats log entries with boundary indicators inline
func formatLogEntriesWithBoundaries(entries []logEntry, hasMoreBefore, hasMoreAfter bool) string {
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
		lines = append(lines, formatLogEntry(entry))
	}

	// Add end indicator at the end if we've hit the end boundary
	if !hasMoreAfter {
		lines = append(lines, "")
		lines = append(lines, styles.DimStyle.Render("--- end of logs ---"))
	}

	return strings.Join(lines, "\n")
}

// formatLogEntry formats a single log entry for display
func formatLogEntry(entry logEntry) string {
	// Format: timestamp hostname unit: message
	// Similar to journalctl short-iso format
	if entry.hostname != "" && entry.unit != "" {
		return fmt.Sprintf("%s %s %s: %s", entry.timestamp, entry.hostname, entry.unit, entry.message)
	} else if entry.unit != "" {
		return fmt.Sprintf("%s %s: %s", entry.timestamp, entry.unit, entry.message)
	} else {
		return fmt.Sprintf("%s %s", entry.timestamp, entry.message)
	}
}

type logsMsg struct {
	entries     []logEntry // Parsed log entries
	firstCursor string     // First cursor in the result (for bidirectional nav)
	lastCursor  string     // Last cursor in the result
	reverse     bool
	hasMore     bool // Whether there are more entries in this direction
	isPrefetch  bool // Whether this is a background prefetch request
	err         error
}

type logEntry struct {
	timestamp string
	hostname  string
	unit      string
	message   string
	cursor    string
}

// logBuffer manages log entries and handles prefetching
type logBuffer struct {
	entries          []logEntry
	beforeCursor     string
	afterCursor      string
	hasMoreBefore    bool
	hasMoreAfter     bool
	serviceName      string
	prefetching      bool
	prefetchingAfter bool
	targetSize       int // Target number of entries to keep loaded
}

func newLogBuffer(serviceName string, targetSize int) *logBuffer {
	return &logBuffer{
		entries:       []logEntry{},
		serviceName:   serviceName,
		targetSize:    targetSize,
		hasMoreBefore: true,
		hasMoreAfter:  false,
	}
}

// GetContent returns formatted content for display with boundary markers
func (lb *logBuffer) GetContent() string {
	return formatLogEntriesWithBoundaries(lb.entries, lb.hasMoreBefore, lb.hasMoreAfter)
}

// ShouldPrefetch returns true if we need to fetch more older logs
func (lb *logBuffer) ShouldPrefetch() bool {
	return len(lb.entries) < lb.targetSize && lb.hasMoreBefore && lb.beforeCursor != "" && !lb.prefetching
}

// StartPrefetch marks prefetch as in progress and returns the fetch command
func (lb *logBuffer) StartPrefetch() tea.Cmd {
	if !lb.ShouldPrefetch() {
		return nil
	}
	lb.prefetching = true
	return fetchLogs(lb.serviceName, true, lb.beforeCursor, true)
}

// AppendInitial sets initial logs (most recent)
func (lb *logBuffer) AppendInitial(entries []logEntry, firstCursor, lastCursor string) tea.Cmd {
	lb.entries = entries
	lb.beforeCursor = firstCursor
	lb.afterCursor = lastCursor
	lb.hasMoreBefore = true
	lb.hasMoreAfter = false
	return lb.StartPrefetch()
}

// PrependOlder adds older logs to the beginning
// oldestCursor should be the cursor of the oldest entry in the batch (to fetch even older logs)
func (lb *logBuffer) PrependOlder(entries []logEntry, oldestCursor string, hasMore bool) tea.Cmd {
	lb.entries = append(entries, lb.entries...)
	lb.beforeCursor = oldestCursor
	lb.hasMoreBefore = hasMore
	lb.prefetching = false
	return lb.StartPrefetch()
}

// AppendNewer adds newer logs to the end
func (lb *logBuffer) AppendNewer(entries []logEntry, lastCursor string, hasMore bool) tea.Cmd {
	lb.entries = append(lb.entries, entries...)
	lb.afterCursor = lastCursor
	lb.hasMoreAfter = hasMore
	lb.prefetchingAfter = false
	// For forward prefetching, we could continue prefetching newer logs
	// but typically we want to stay near "now", so we don't auto-prefetch forward
	return nil
}

// Cleanup clears all log entries and resets state for memory cleanup
func (lb *logBuffer) Cleanup() {
	lb.entries = nil
	lb.beforeCursor = ""
	lb.afterCursor = ""
	lb.hasMoreBefore = false
	lb.hasMoreAfter = false
	lb.prefetching = false
	lb.prefetchingAfter = false
}

// TrimBuffer removes entries far from viewport to limit memory usage
// Returns the number of lines trimmed from the top (for viewport offset adjustment)
func (lb *logBuffer) TrimBuffer(viewportY, viewportHeight int) int {
	if len(lb.entries) <= maxBufferEntries {
		return 0 // No need to trim
	}

	// Calculate viewport position in terms of log entries
	// We need to estimate which entries are visible
	totalLines := 0
	visibleStartEntry := 0
	visibleEndEntry := len(lb.entries) - 1

	// Find which entries correspond to the viewport position
	for i, entry := range lb.entries {
		entryLines := len(strings.Split(formatLogEntry(entry), "\n"))
		if totalLines+entryLines > viewportY {
			visibleStartEntry = i
			break
		}
		totalLines += entryLines
	}

	// Calculate how many entries to keep on each side
	entriesToKeep := max(
		// Rough estimate: ~3 lines per entry
		viewportsToKeep*viewportHeight/3,
		// Keep at least one page
		logPageSize)

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
		linesTrimmed += len(strings.Split(formatLogEntry(lb.entries[i]), "\n"))
	}

	// Trim the entries
	oldEntries := lb.entries
	lb.entries = make([]logEntry, trimEnd-trimStart)
	copy(lb.entries, oldEntries[trimStart:trimEnd])

	// Update cursors if we trimmed from edges
	if trimStart > 0 {
		lb.beforeCursor = lb.entries[0].cursor
		lb.hasMoreBefore = true // We know there are more entries we trimmed
	}
	if trimEnd < len(oldEntries) {
		lb.afterCursor = lb.entries[len(lb.entries)-1].cursor
		lb.hasMoreAfter = true // We know there are more entries we trimmed
	}

	return linesTrimmed
}

// CheckPrefetchNeeds checks if prefetching should be triggered based on viewport position
// Returns commands for prefetching in both directions if needed
func (lb *logBuffer) CheckPrefetchNeeds(viewportY, viewportHeight, totalHeight int) []tea.Cmd {
	var cmds []tea.Cmd

	// Calculate threshold for prefetching (viewportsAhead * viewport height)
	prefetchThreshold := viewportsAhead * viewportHeight

	// Check if we should prefetch older logs (scrolling near top)
	if viewportY < prefetchThreshold && lb.hasMoreBefore && lb.beforeCursor != "" && !lb.prefetching {
		lb.prefetching = true
		cmds = append(cmds, fetchLogs(lb.serviceName, true, lb.beforeCursor, true))
	}

	// Check if we should prefetch newer logs (scrolling near bottom)
	distanceFromBottom := totalHeight - (viewportY + viewportHeight)
	if distanceFromBottom < prefetchThreshold && lb.hasMoreAfter && lb.afterCursor != "" && !lb.prefetchingAfter {
		lb.prefetchingAfter = true
		cmds = append(cmds, fetchLogs(lb.serviceName, false, lb.afterCursor, true))
	}

	return cmds
}

func fetchLogs(service string, reverse bool, cursor string, isPrefetch bool) tea.Cmd {
	return func() tea.Msg {
		// Build journalctl command with JSON output for proper parsing
		// Add .service suffix to ensure exact unit match
		serviceUnit := service
		if !strings.HasSuffix(serviceUnit, ".service") {
			serviceUnit = serviceUnit + ".service"
		}

		args := []string{
			"journalctl",
			"-u", serviceUnit,
			"-o", "json", // Use JSON for structured parsing
		}

		if cursor != "" {
			// Use --cursor with the given cursor position
			args = append(args, "--cursor", cursor)
			if reverse {
				// Get entries before this cursor (older logs)
				// --reverse makes it go backward from the cursor
				args = append(args, "--reverse", "-n", fmt.Sprintf("%d", logPageSize))
			} else {
				// Get entries after this cursor (newer logs)
				// Forward from the cursor (default behavior)
				args = append(args, "-n", fmt.Sprintf("%d", logPageSize))
			}
		} else {
			// No cursor - show most recent entries
			if reverse {
				args = append(args, "--reverse", "-n", fmt.Sprintf("%d", logPageSize))
			} else {
				args = append(args, "-n", fmt.Sprintf("%d", logPageSize))
			}
		}

		// Use context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := executor.Run(ctx, args[0],
			executor.WithArgs(args[1:]...),
			executor.WithOutputMode(executor.OutputModeCombined),
		)

		if err != nil {
			return logsMsg{isPrefetch: isPrefetch, err: fmt.Errorf("failed to fetch logs: %w", err)}
		}

		output := result.Combined

		// Parse JSON entries
		entries, err := parseJSONLogs(output)
		if err != nil {
			return logsMsg{isPrefetch: isPrefetch, err: fmt.Errorf("failed to parse logs: %w", err)}
		}

		// When using --cursor, journalctl ALWAYS includes the cursor entry as the first result
		// We need to skip it in both forward and reverse modes to avoid duplicates
		if cursor != "" && len(entries) > 0 {
			if entries[0].cursor == cursor {
				entries = entries[1:]
			}
		}

		// If no entries returned (after skipping cursor), we've hit a boundary
		if len(entries) == 0 {
			return logsMsg{
				entries:     nil,
				firstCursor: cursor,
				lastCursor:  cursor,
				reverse:     reverse,
				hasMore:     false,
				isPrefetch:  isPrefetch,
				err:         nil,
			}
		}

		// Determine if there are more entries available
		// After cursor skip, we get logPageSize-1 entries if more exist
		// If we got fewer entries, we've hit a boundary
		hasMore := len(entries) >= logPageSize-1

		// Extract cursors BEFORE normalizing entry order
		// For reverse mode: journalctl returns newest→oldest, so last entry is oldest
		// For forward mode: journalctl returns oldest→newest, so first entry is oldest
		var firstCursor, lastCursor string
		if reverse {
			// In reverse mode, last entry is the oldest (to fetch even older logs)
			firstCursor = entries[len(entries)-1].cursor
			lastCursor = entries[0].cursor
		} else {
			// In forward mode, first is oldest, last is newest
			firstCursor = entries[0].cursor
			lastCursor = entries[len(entries)-1].cursor
		}

		// Normalize entry order: our buffer always maintains oldest→newest order
		// journalctl with --reverse returns newest→oldest, so we need to reverse it back
		if reverse {
			// Reverse the slice to convert newest→oldest to oldest→newest
			for i := 0; i < len(entries)/2; i++ {
				entries[i], entries[len(entries)-1-i] = entries[len(entries)-1-i], entries[i]
			}
		}

		return logsMsg{
			entries:     entries,
			firstCursor: firstCursor,
			lastCursor:  lastCursor,
			reverse:     reverse,
			hasMore:     hasMore,
			isPrefetch:  isPrefetch,
			err:         nil,
		}
	}
}

// parseJSONLogs parses line-delimited JSON from journalctl -o json
func parseJSONLogs(output []byte) ([]logEntry, error) {
	var entries []logEntry
	lines := strings.SplitSeq(string(output), "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse JSON entry
		var rawEntry map[string]any
		if err := json.Unmarshal([]byte(line), &rawEntry); err != nil {
			// Skip malformed lines
			continue
		}

		entry := logEntry{}

		// Extract timestamp
		if ts, ok := rawEntry["__REALTIME_TIMESTAMP"].(string); ok {
			// Convert microseconds to time
			if usec, err := strconv.ParseInt(ts, 10, 64); err == nil {
				t := time.Unix(0, usec*1000)
				entry.timestamp = t.Format("2006-01-02T15:04:05-0700")
			}
		}

		// Extract fields
		if hostname, ok := rawEntry["_HOSTNAME"].(string); ok {
			entry.hostname = hostname
		}
		if unit, ok := rawEntry["_SYSTEMD_UNIT"].(string); ok {
			entry.unit = unit
		} else if unit, ok := rawEntry["SYSLOG_IDENTIFIER"].(string); ok {
			entry.unit = unit
		}
		if message, ok := rawEntry["MESSAGE"].(string); ok {
			entry.message = message
		}
		if cursor, ok := rawEntry["__CURSOR"].(string); ok {
			entry.cursor = cursor
		}

		// Only add entries that have at least a cursor
		if entry.cursor != "" {
			entries = append(entries, entry)
		}
	}

	return entries, nil
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

	// Create a list with styling for inline display
	listDelegate := list.NewDefaultDelegate()
	listDelegate.ShowDescription = false

	// Start with compact size for inline display
	listModel := list.New(items, listDelegate, 80, 15)
	listModel.Title = "Systemd Services"
	listModel.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).UnsetBackground()
	listModel.SetShowStatusBar(false)
	listModel.SetFilteringEnabled(false)
	listModel.SetShowHelp(false) // We'll use our own help

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Initialize help
	h := help.New()
	h.ShowAll = false

	// Initial model
	initialModel := model{
		list:                listModel,
		spinner:             s,
		help:                h,
		keys:                keys,
		serviceItems:        items,
		activeView:          "list",
		viewportInitialized: false,
		loading:             false,
		err:                 nil,
	}

	// Run the program with the initial model
	// Start in normal terminal mode (inline), only use alt screen when viewing logs
	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running logs UI: %w", err)
	}

	return nil
}

func getFilteredSystemdServices(filters []string) ([]string, error) {
	// Use context with timeout for systemctl command
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a map to deduplicate services across multiple filter queries
	serviceMap := make(map[string]bool)

	// For each filter pattern, run a targeted systemctl query
	for _, filter := range filters {
		// Use list-units with glob pattern for faster, targeted queries
		// This filters at the systemd level instead of fetching all services
		pattern := filter + "*"
		result, err := executor.Run(ctx, "systemctl",
			executor.WithArgs("list-units", pattern, "--type=service", "--all", "--no-pager"),
			executor.WithOutputMode(executor.OutputModeCombined),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list systemd services for pattern %s: %w", pattern, err)
		}

		output := result.Combined

		// Parse list-units output format: "UNIT LOAD ACTIVE SUB DESCRIPTION"
		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Skip empty lines, header, footer
			if trimmed == "" || strings.HasPrefix(trimmed, "UNIT") ||
				strings.HasPrefix(trimmed, "Legend:") || strings.HasPrefix(trimmed, "LOAD") ||
				strings.Contains(line, "loaded units listed") ||
				strings.HasPrefix(trimmed, "To show all") {
				continue
			}

			// Extract service name (first column after trimming/splitting)
			// Lines may start with ● for failed services
			fields := strings.Fields(line)
			if len(fields) > 0 {
				serviceName := strings.TrimPrefix(fields[0], "●")
				serviceName = strings.TrimSpace(serviceName)
				if strings.HasSuffix(serviceName, ".service") {
					serviceName = strings.TrimSuffix(serviceName, ".service")
					serviceMap[serviceName] = true
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scanning systemd services output failed: %w", err)
		}
	}

	// Convert map to sorted slice
	filteredServices := make([]string, 0, len(serviceMap))
	for service := range serviceMap {
		filteredServices = append(filteredServices, service)
	}

	// Sort alphabetically
	sort.Strings(filteredServices)

	return filteredServices, nil
}
