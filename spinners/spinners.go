package spinners

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GlobalSpinnerStyle holds the default style for the spinner itself.
var GlobalSpinnerStyle = getStyle("magenta")

type TaskFunc func() error

// SpinnerOptions defines the options for creating a spinner.
type SpinnerOptions struct {
	TaskName        string
	Color           string
	StopColor       string
	StopFailColor   string
	StopMessage     string
	StopFailMessage string
}

// RunTaskWithSpinner provides a spinner with default options.
func RunTaskWithSpinner(message string, taskFunc TaskFunc) error {
	opts := SpinnerOptions{
		TaskName:        message,
		Color:           "yellow",
		StopColor:       "green",
		StopFailColor:   "red",
		StopMessage:     message,
		StopFailMessage: message,
	}
	return runTaskWithSpinner(opts, taskFunc)
}

// RunTaskWithSpinnerCustom provides a spinner with custom options.
func RunTaskWithSpinnerCustom(opts SpinnerOptions, taskFunc TaskFunc) error {
	if opts.TaskName == "" {
		return fmt.Errorf("taskName is required")
	}

	if opts.Color == "" {
		opts.Color = "white"
	}
	if opts.StopColor == "" {
		opts.StopColor = "green"
	}
	if opts.StopFailColor == "" {
		opts.StopFailColor = "red"
	}
	if opts.StopMessage == "" {
		opts.StopMessage = opts.TaskName
	}
	if opts.StopFailMessage == "" {
		opts.StopFailMessage = opts.TaskName
	}
	return runTaskWithSpinner(opts, taskFunc)
}

// Internal function to handle the actual spinner logic.
func runTaskWithSpinner(opts SpinnerOptions, taskFunc TaskFunc) error {
	p := tea.NewProgram(newSpinnerModel(opts, taskFunc))

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		p.Send(quitMsg{})
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run spinner: %w", err)
	}
	return nil
}

// RunInfoSpinner (for backwards compatibility)
func RunInfoSpinner(message string) error {
	opts := SpinnerOptions{
		TaskName:        message,
		Color:           "blue",
		StopColor:       "blue",
		StopFailColor:   "blue",
		StopMessage:     message,
		StopFailMessage: message,
	}
	return runTaskWithSpinner(opts, func() error { return nil })
}

// RunWarningSpinner (for backwards compatibility)
func RunWarningSpinner(message string) error {
	opts := SpinnerOptions{
		TaskName:        message,
		Color:           "yellow",
		StopColor:       "yellow",
		StopFailColor:   "yellow",
		StopMessage:     message,
		StopFailMessage: message,
	}
	return runTaskWithSpinner(opts, func() error { return nil })
}

// --- Bubble Tea Model ---

type spinnerModel struct {
	spinner         spinner.Model
	opts            SpinnerOptions
	taskFunc        TaskFunc
	taskErr         error
	finished        bool
	interrupt       bool
	interruptReason string
}

type errMsg struct{ err error }
type successMsg struct{}
type quitMsg struct{}

func newSpinnerModel(opts SpinnerOptions, taskFunc TaskFunc) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = GlobalSpinnerStyle

	return spinnerModel{
		spinner:  s,
		opts:     opts,
		taskFunc: taskFunc,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := m.taskFunc()
		if err != nil {
			return errMsg{err}
		}
		return successMsg{}
	})
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.interrupt = true
			m.interruptReason = "interrupted"
			return m, tea.Quit
		}
	case errMsg:
		m.taskErr = msg.err
		m.finished = true
		return m, tea.Quit
	case successMsg:
		m.finished = true
		return m, tea.Quit
	case quitMsg:
		m.interrupt = true
		m.interruptReason = "interrupted"
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m spinnerModel) View() string {
	if m.interrupt {
		return getStyle(m.opts.StopFailColor).Render("● " + m.interruptReason + "\n")
	}
	if m.finished {
		if m.taskErr != nil {
			return getStyle(m.opts.StopFailColor).Render(fmt.Sprintf("● %s: %v\n", m.opts.StopFailMessage, m.taskErr))
		}
		return getStyle(m.opts.StopColor).Render(fmt.Sprintf("● %s\n", m.opts.StopMessage))
	}

	// Apply style from opts.Color to the task name.
	styledTaskName := getStyle(m.opts.Color).Render(m.opts.TaskName)
	return fmt.Sprintf("%s %s", m.spinner.View(), styledTaskName)
}

// Helper function to map color names to styles (still needed for stop/fail colors).
func getStyle(colorName string) lipgloss.Style {
	style := lipgloss.NewStyle()

	switch colorName {
	case "black":
		style = style.Foreground(lipgloss.Color("0"))
	case "red":
		style = style.Foreground(lipgloss.Color("1"))
	case "green":
		style = style.Foreground(lipgloss.Color("2"))
	case "yellow":
		style = style.Foreground(lipgloss.Color("3"))
	case "blue":
		style = style.Foreground(lipgloss.Color("4"))
	case "magenta":
		style = style.Foreground(lipgloss.Color("5"))
	case "cyan":
		style = style.Foreground(lipgloss.Color("6"))
	case "white":
		style = style.Foreground(lipgloss.Color("7"))
	case "brightblack", "gray", "grey":
		style = style.Foreground(lipgloss.Color("8"))
	case "brightred":
		style = style.Foreground(lipgloss.Color("9"))
	case "brightgreen":
		style = style.Foreground(lipgloss.Color("10"))
	case "brightyellow":
		style = style.Foreground(lipgloss.Color("11"))
	case "brightblue":
		style = style.Foreground(lipgloss.Color("12"))
	case "brightmagenta":
		style = style.Foreground(lipgloss.Color("13"))
	case "brightcyan":
		style = style.Foreground(lipgloss.Color("14"))
	case "brightwhite":
		style = style.Foreground(lipgloss.Color("15"))
	default:
		style = style.Foreground(lipgloss.Color("7"))
	}

	return style
}
