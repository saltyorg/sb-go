package spinners

import (
	"context"
	"fmt"

	"github.com/saltyorg/sb-go/internal/styles"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GlobalSpinnerStyle holds the default style for the spinner itself.
var GlobalSpinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorMagenta))
var VerboseMode bool

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

// SetVerboseMode sets the verbose mode for all spinners
func SetVerboseMode(verbose bool) {
	VerboseMode = verbose
}

// RunTaskWithSpinnerContext provides a spinner with default options or text output in verbose mode.
// It accepts a context for cancellation support, allowing tasks to be interrupted gracefully.
func RunTaskWithSpinnerContext(ctx context.Context, message string, taskFunc TaskFunc) error {
	if VerboseMode {
		// In verbose mode, just print the message and execute the task directly
		fmt.Println(message + "...")
		err := taskFunc()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Println(message + " completed")
		}
		return err
	}

	// Otherwise, use the spinner UI
	opts := SpinnerOptions{
		TaskName:        message,
		Color:           styles.ColorYellow,
		StopColor:       styles.ColorMediumGreen,
		StopFailColor:   styles.ColorDarkRed,
		StopMessage:     message,
		StopFailMessage: message,
	}
	return runTaskWithSpinnerContext(ctx, opts, taskFunc)
}

// RunTaskWithSpinnerCustomContext provides a spinner with custom options.
// It accepts a context for cancellation support, allowing tasks to be interrupted gracefully.
func RunTaskWithSpinnerCustomContext(ctx context.Context, opts SpinnerOptions, taskFunc TaskFunc) error {
	if opts.TaskName == "" {
		return fmt.Errorf("taskName is required")
	}

	if opts.Color == "" {
		opts.Color = styles.ColorYellow
	}
	if opts.StopColor == "" {
		opts.StopColor = styles.ColorMediumGreen
	}
	if opts.StopFailColor == "" {
		opts.StopFailColor = styles.ColorDarkRed
	}
	if opts.StopMessage == "" {
		opts.StopMessage = opts.TaskName
	}
	if opts.StopFailMessage == "" {
		opts.StopFailMessage = opts.TaskName
	}
	return runTaskWithSpinnerContext(ctx, opts, taskFunc)
}

// runTaskWithSpinnerContext is an internal function to handle the actual spinner logic with context support.
func runTaskWithSpinnerContext(ctx context.Context, opts SpinnerOptions, taskFunc TaskFunc) error {
	// Create a channel to receive the task error
	errCh := make(chan error, 1)

	// Modify the task function to send the error to the channel
	wrappedTaskFunc := func() error {
		err := taskFunc()
		errCh <- err // Send the error to the channel
		return err
	}

	// Create and run the program with the wrapped task
	p := tea.NewProgram(newSpinnerModel(opts, wrappedTaskFunc))

	// Monitor context cancellation and send quit message to the program
	go func() {
		<-ctx.Done()
		p.Send(quitMsg{})
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run spinner: %w", err)
	}

	// Get the task error from the channel
	taskErr := <-errCh

	// If there's an error, print the details on a separate line
	if taskErr != nil {
		fmt.Printf("  Error details: %v\n", taskErr)
	}

	return taskErr
}

// RunInfoSpinner prints an informational message.
func RunInfoSpinner(message string) error {
	if VerboseMode {
		fmt.Println(message)
		return nil
	}
	style := getStyle(styles.ColorLightBlue)
	fmt.Println(style.Render(fmt.Sprintf("● %s", message)))
	return nil
}

// RunWarningSpinner prints a warning message.
func RunWarningSpinner(message string) error {
	if VerboseMode {
		fmt.Println(message)
		return nil
	}
	style := getStyle(styles.ColorYellow)
	fmt.Println(style.Render(fmt.Sprintf("● %s", message)))
	return nil
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
			return getStyle(m.opts.StopFailColor).Render(fmt.Sprintf("● %s: Failed\n", m.opts.StopFailMessage))
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
	return style.Foreground(lipgloss.Color(colorName))
}
