package spinners

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/signals"
	"github.com/saltyorg/sb-go/internal/styles"
	"github.com/saltyorg/sb-go/internal/tty"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// GlobalSpinnerStyle holds the default style for the spinner itself.
var GlobalSpinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorMagenta))
var VerboseMode bool

const liveTaskOutputLines = 8
const terminalCapabilitySettleDelay = 250 * time.Millisecond

type TaskFunc func() error
type TaskOutputFunc func(stdout, stderr io.Writer) error

// SpinnerOptions defines the options for creating a spinner.
type SpinnerOptions struct {
	TaskName        string
	Color           string
	StopColor       string
	StopFailColor   string
	StopMessage     string
	StopFailMessage string
	// IndentLevel overrides automatic nesting when greater than zero. This is
	// useful for concurrent siblings whose start order does not imply parentage.
	IndentLevel int
	// CollapseChildren removes successful child results from the live render
	// and collapses them into the task's final result. Failed children are kept
	// so the final error retains useful context.
	CollapseChildren bool
	// RetainChildren keeps completed results in the live renderer so a task can
	// display its full hierarchy. The terminal may clip trees taller than the
	// available viewport.
	RetainChildren bool
}

// A single Bubble Tea program must own the terminal at a time. Tasks frequently
// call other functions which have their own spinners, so starting a program per
// task makes the programs fight over stdin and the renderer. activeSession lets
// those nested calls become children of the program which already owns it.
var sessions struct {
	sync.Mutex
	active *spinnerSession
}

type spinnerSession struct {
	program *tea.Program
	nextID  atomic.Uint64
}

// SetVerboseMode sets the verbose mode for all spinners.
// If verbose is false but there's no TTY available, verbose mode is automatically enabled.
func SetVerboseMode(verbose bool) {
	VerboseMode = verbose || !tty.IsInteractive()
}

// RunTaskWithSpinnerContext provides a spinner with default options or text output in verbose mode.
// It accepts a context for cancellation support, allowing tasks to be interrupted gracefully.
func RunTaskWithSpinnerContext(ctx context.Context, message string, taskFunc TaskFunc) error {
	return RunTaskWithSpinnerCustomContext(ctx, SpinnerOptions{TaskName: message}, taskFunc)
}

// RunTaskWithSpinnerOutputContext streams task output into the managed
// renderer. Output disappears on success and is retained when the task fails.
func RunTaskWithSpinnerOutputContext(ctx context.Context, message string, taskFunc TaskOutputFunc) error {
	opts := withDefaults(SpinnerOptions{TaskName: message})
	if VerboseMode {
		fmt.Fprintln(os.Stderr, opts.TaskName+"...")
		err := taskFunc(os.Stdout, os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, opts.StopMessage)
		}
		return err
	}

	sessions.Lock()
	active := sessions.active
	sessions.Unlock()
	if active != nil {
		return active.runChildWithOutput(ctx, opts, taskFunc)
	}

	return RunTaskWithSpinnerCustomContext(ctx, opts, func() error {
		sessions.Lock()
		active := sessions.active
		sessions.Unlock()
		if active == nil {
			return taskFunc(io.Discard, io.Discard)
		}
		return active.runWithOutput(0, taskFunc)
	})
}

// RunTaskWithSpinnerStreamingContext streams commands which use executor with
// the supplied context into the managed viewport without making them
// interactive or attaching a pseudo-terminal.
func RunTaskWithSpinnerStreamingContext(ctx context.Context, message string, taskFunc func(context.Context) error) error {
	return RunTaskWithSpinnerOutputContext(ctx, message, func(stdout, stderr io.Writer) error {
		return taskFunc(executor.WithManagedOutput(ctx, stdout, stderr))
	})
}

// RunTaskWithSpinnerCustomContext provides a spinner with custom options.
// It accepts a context for cancellation support, allowing tasks to be interrupted gracefully.
func RunTaskWithSpinnerCustomContext(ctx context.Context, opts SpinnerOptions, taskFunc TaskFunc) error {
	if opts.TaskName == "" {
		return fmt.Errorf("taskName is required")
	}
	opts = withDefaults(opts)

	if VerboseMode {
		fmt.Fprintln(os.Stderr, opts.TaskName+"...")
		err := taskFunc()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, opts.StopMessage)
		}
		return err
	}

	sessions.Lock()
	active := sessions.active
	sessions.Unlock()
	if active != nil {
		return active.runChild(ctx, opts, taskFunc)
	}

	return runRootTask(ctx, opts, taskFunc)
}

func withDefaults(opts SpinnerOptions) SpinnerOptions {
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
	return opts
}

func runRootTask(ctx context.Context, opts SpinnerOptions, taskFunc TaskFunc) error {
	result := make(chan error, 1)
	model := newSpinnerModel(opts, func() error {
		err := taskFunc()
		result <- err
		return err
	})
	program := tea.NewProgram(model, tea.WithOutput(&synchronizedOutputWriter{writer: os.Stderr}))
	session := &spinnerSession{program: program}

	// Only one root renderer may own the terminal. Holding this lock through
	// registration also closes the race where two top-level tasks start at once.
	sessions.Lock()
	if sessions.active != nil {
		active := sessions.active
		sessions.Unlock()
		return active.runChild(ctx, opts, taskFunc)
	}
	sessions.active = session
	sessions.Unlock()

	renderDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			program.Send(quitMsg{})
		case <-renderDone:
		}
	}()

	_, runErr := program.Run()
	close(renderDone)

	sessions.Lock()
	if sessions.active == session {
		sessions.active = nil
	}
	sessions.Unlock()

	if runErr != nil {
		return fmt.Errorf("failed to run spinner: %w", runErr)
	}
	select {
	case taskErr := <-result:
		return taskErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// synchronizedOutputWriter asks compatible terminals to apply each renderer
// update atomically. Without it, users can briefly see the cleared viewport
// between Bubble Tea's erase and redraw operations.
type synchronizedOutputWriter struct {
	writer io.Writer
	mu     sync.Mutex
}

func (w *synchronizedOutputWriter) Fd() uintptr {
	if file, ok := w.writer.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

func (w *synchronizedOutputWriter) Read(output []byte) (int, error) {
	if reader, ok := w.writer.(io.Reader); ok {
		return reader.Read(output)
	}
	return 0, io.EOF
}

func (w *synchronizedOutputWriter) Close() error {
	// The underlying stream is process-owned stderr and must remain available
	// after an individual spinner program exits.
	return nil
}

func (w *synchronizedOutputWriter) Write(output []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Bubble Tea enables this itself when terminal capability detection
	// succeeds. Avoid nesting the mode in that case.
	if bytes.Contains(output, []byte(ansi.SetModeSynchronizedOutput)) {
		return w.writer.Write(output)
	}

	frame := make([]byte, 0, len(ansi.SetModeSynchronizedOutput)+len(output)+len(ansi.ResetModeSynchronizedOutput))
	frame = append(frame, ansi.SetModeSynchronizedOutput...)
	frame = append(frame, output...)
	frame = append(frame, ansi.ResetModeSynchronizedOutput...)
	written, err := w.writer.Write(frame)
	if err != nil {
		return 0, err
	}
	if written != len(frame) {
		return 0, io.ErrShortWrite
	}
	return len(output), nil
}

func (s *spinnerSession) runChild(ctx context.Context, opts SpinnerOptions, taskFunc TaskFunc) error {
	id := s.nextID.Add(1)
	s.program.Send(pushTaskMsg{id: id, opts: opts})

	// A child task is deliberately run by its caller, not as a Tea command.
	// This keeps nested calls synchronous while the root renderer keeps ticking.
	errCh := make(chan error, 1)
	go func() {
		errCh <- taskFunc()
	}()

	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = ctx.Err()
	}
	s.program.Send(popTaskMsg{id: id, opts: opts, err: err})
	return err
}

func (s *spinnerSession) runChildWithOutput(ctx context.Context, opts SpinnerOptions, taskFunc TaskOutputFunc) error {
	id := s.nextID.Add(1)
	s.program.Send(pushTaskMsg{id: id, opts: opts})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.runWithOutput(id, taskFunc)
	}()

	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = ctx.Err()
	}
	s.program.Send(popTaskMsg{id: id, opts: opts, err: err})
	return err
}

func (s *spinnerSession) runWithOutput(id uint64, taskFunc TaskOutputFunc) error {
	stdout := taskOutputWriter{session: s, id: id}
	stderr := taskOutputWriter{session: s, id: id}
	return taskFunc(stdout, stderr)
}

type taskOutputWriter struct {
	session *spinnerSession
	id      uint64
}

func (w taskOutputWriter) Write(p []byte) (int, error) {
	w.session.program.Send(taskOutputMsg{id: w.id, output: string(p)})
	return len(p), nil
}

// RunInfoSpinner prints an informational message without disturbing an active spinner.
func RunInfoSpinner(message string) error {
	return printMessage(message, styles.ColorLightBlue)
}

// RunWarningSpinner prints a warning message without disturbing an active spinner.
func RunWarningSpinner(message string) error {
	return printMessage(message, styles.ColorYellow)
}

func printMessage(message, color string) error {
	if VerboseMode {
		fmt.Fprintln(os.Stderr, message)
		return nil
	}
	line := getStyle(color).Render("● " + message)
	sessions.Lock()
	active := sessions.active
	sessions.Unlock()
	if active != nil {
		// Program.Println clears and restores the current render, unlike writing
		// directly to stdout/stderr while a spinner is active.
		active.program.Println("  " + line)
		return nil
	}
	fmt.Fprintln(os.Stderr, line)
	return nil
}

// --- Bubble Tea Model ---

type spinnerModel struct {
	spinner         spinner.Model
	tasks           []spinnerTask
	completed       []completedTask
	retained        []completedTask
	pending         []completedTask
	taskFunc        TaskFunc
	taskErr         error
	finished        bool
	terminalSettled bool
	interrupt       bool
	interruptReason string
}

type errMsg struct{ err error }
type successMsg struct{}
type terminalSettledMsg struct{}
type quitMsg struct{}
type spinnerTask struct {
	id     uint64
	opts   SpinnerOptions
	depth  int
	output taskOutputBuffer
}
type completedTask struct {
	id       uint64
	opts     SpinnerOptions
	err      error
	depth    int
	parentID uint64
	output   string
}
type pushTaskMsg struct {
	id   uint64
	opts SpinnerOptions
}
type popTaskMsg struct {
	id   uint64
	opts SpinnerOptions
	err  error
}
type taskOutputMsg struct {
	id     uint64
	output string
}

// taskOutputBuffer models the small part of terminal line handling used by
// command progress output. Carriage returns replace the current line instead
// of growing the viewport, while newlines commit it.
type taskOutputBuffer struct {
	lines   []string
	current []rune
	cursor  int
	parser  *ansi.Parser
}

func (b *taskOutputBuffer) WriteString(output string) {
	if b.parser == nil {
		b.parser = ansi.NewParser()
	}
	b.parser.SetHandler(ansi.Handler{
		Print: b.writeRune,
		Execute: func(char byte) {
			b.writeRune(rune(char))
		},
		HandleCsi: func(command ansi.Cmd, params ansi.Params) {
			if command.Final() != 'K' {
				return
			}
			mode, _, _ := params.Param(0, 0)
			switch mode {
			case 0:
				b.current = b.current[:min(b.cursor, len(b.current))]
			case 2:
				b.current = b.current[:0]
				b.cursor = 0
			}
		},
	})
	for i := range len(output) {
		b.parser.Advance(output[i])
	}
}

func (b *taskOutputBuffer) writeRune(char rune) {
	switch char {
	case '\r':
		b.cursor = 0
	case '\n':
		b.lines = append(b.lines, string(b.current))
		b.current = b.current[:0]
		b.cursor = 0
	case '\b':
		if b.cursor > 0 {
			b.cursor--
		}
	default:
		if b.cursor < len(b.current) {
			b.current[b.cursor] = char
		} else {
			b.current = append(b.current, char)
		}
		b.cursor++
	}
}

func (b *taskOutputBuffer) String() string {
	if len(b.lines) == 0 {
		return string(b.current)
	}
	if len(b.current) == 0 {
		return strings.Join(b.lines, "\n")
	}
	return strings.Join(b.lines, "\n") + "\n" + string(b.current)
}

func newSpinnerModel(opts SpinnerOptions, taskFunc TaskFunc) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = GlobalSpinnerStyle
	return spinnerModel{
		spinner:  s,
		tasks:    []spinnerTask{{opts: opts}},
		taskFunc: taskFunc,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.Tick(terminalCapabilitySettleDelay, func(time.Time) tea.Msg {
		return terminalSettledMsg{}
	}), func() tea.Msg {
		if err := m.taskFunc(); err != nil {
			return errMsg{err}
		}
		return successMsg{}
	})
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			signals.GetGlobalManager().Shutdown(130)
			m.interrupt = true
			m.interruptReason = "interrupted"
			return m, tea.Quit
		}
	case errMsg:
		m.taskErr = msg.err
		m.finished = true
		if !m.terminalSettled {
			return m, nil
		}
		return m, tea.Quit
	case successMsg:
		m.finished = true
		if !m.terminalSettled {
			return m, nil
		}
		return m, tea.Quit
	case terminalSettledMsg:
		m.terminalSettled = true
		if m.finished {
			return m, tea.Quit
		}
		return m, nil
	case quitMsg:
		m.interrupt = true
		m.interruptReason = "interrupted"
		return m, tea.Quit
	case pushTaskMsg:
		depth := len(m.tasks)
		if msg.opts.IndentLevel > 0 {
			depth = msg.opts.IndentLevel
		}
		m.tasks = append(m.tasks, spinnerTask{id: msg.id, opts: msg.opts, depth: depth})
		return m, nil
	case taskOutputMsg:
		for i := range m.tasks {
			if m.tasks[i].id == msg.id {
				m.tasks[i].output.WriteString(msg.output)
				break
			}
		}
		return m, nil
	case popTaskMsg:
		depth := 1
		var parentID uint64
		parentOpts := m.tasks[0].opts
		var output string
		for i := len(m.tasks) - 1; i > 0; i-- {
			if m.tasks[i].id == msg.id {
				depth = m.tasks[i].depth
				output = m.tasks[i].output.String()
				for j := i - 1; j >= 0; j-- {
					if m.tasks[j].depth < depth {
						parentID = m.tasks[j].id
						parentOpts = m.tasks[j].opts
						break
					}
				}
				m.tasks = append(m.tasks[:i], m.tasks[i+1:]...)
				break
			}
		}
		if msg.opts.CollapseChildren {
			// When a collapsing child finishes, discard its successful staged
			// children and retain failed ones for diagnostic context before the
			// child itself is staged beneath its parent.
			remaining := m.pending[:0]
			for _, task := range m.pending {
				if task.parentID != msg.id {
					remaining = append(remaining, task)
					continue
				}
				if task.err != nil {
					if m.tasks[0].opts.RetainChildren {
						m.retained = append(m.retained, task)
					} else {
						m.completed = append(m.completed, task)
					}
				}
			}
			m.pending = remaining
		}
		if parentOpts.CollapseChildren {
			// A collapsing parent keeps successful children visible until the
			// parent itself finishes. This preserves the live progression
			// without retaining implementation details afterward.
			if msg.err == nil {
				m.pending = append(m.pending, completedTask{
					id:       msg.id,
					opts:     msg.opts,
					err:      msg.err,
					depth:    depth,
					parentID: parentID,
					output:   output,
				})
				return m, nil
			}
			task := completedTask{
				id:       msg.id,
				opts:     msg.opts,
				err:      msg.err,
				depth:    depth,
				parentID: parentID,
				output:   output,
			}
			if m.tasks[0].opts.RetainChildren {
				m.retained = append(m.retained, task)
			} else {
				m.completed = append(m.completed, task)
			}
			return m, nil
		}
		if m.tasks[0].opts.RetainChildren {
			m.retained = append(m.retained, completedTask{
				id:       msg.id,
				opts:     msg.opts,
				err:      msg.err,
				depth:    depth,
				parentID: parentID,
				output:   output,
			})
			return m, nil
		}
		if depth > 1 {
			m.pending = append(m.pending, completedTask{
				opts:     msg.opts,
				err:      msg.err,
				depth:    depth,
				parentID: parentID,
				output:   output,
			})
			return m, nil
		}
		message, color := msg.opts.StopMessage, msg.opts.StopColor
		if msg.err != nil {
			message, color = msg.opts.StopFailMessage+": Failed", msg.opts.StopFailColor
		}
		lines := []string{strings.Repeat("  ", depth) + getStyle(color).Render("● "+message)}
		remaining := m.pending[:0]
		for _, task := range m.pending {
			if task.parentID != msg.id {
				remaining = append(remaining, task)
				continue
			}
			childMessage, childColor := task.opts.StopMessage, task.opts.StopColor
			if task.err != nil {
				childMessage, childColor = task.opts.StopFailMessage+": Failed", task.opts.StopFailColor
			}
			lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(childColor).Render("● "+childMessage))
			if task.err != nil {
				lines = appendTaskOutput(lines, task.depth+1, task.output)
			}
		}
		m.pending = remaining
		return m, tea.Println(strings.Join(lines, "\n"))
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() tea.View {
	root := m.tasks[0].opts
	if m.interrupt {
		return tea.NewView(getStyle(root.StopFailColor).Render("● " + m.interruptReason + "\n"))
	}
	if m.finished {
		if root.RetainChildren {
			return tea.NewView(m.retainedOutput(true) + "\n")
		}
		if m.taskErr != nil {
			lines := []string{getStyle(root.StopFailColor).Render(fmt.Sprintf("● %s: Failed", root.StopFailMessage))}
			lines = appendTaskOutput(lines, 1, m.tasks[0].output.String())
			for _, task := range m.completed {
				if task.err != nil {
					lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(task.opts.StopFailColor).Render("● "+task.opts.StopFailMessage+": Failed"))
					lines = appendTaskOutput(lines, task.depth+1, task.output)
				}
			}
			return tea.NewView(strings.Join(lines, "\n") + "\n")
		}
		return tea.NewView(getStyle(root.StopColor).Render(fmt.Sprintf("● %s\n", root.StopMessage)))
	}

	var lines []string
	if root.RetainChildren {
		lines = append(lines, m.retainedOutput(false))
	}
	renderedPending := make([]bool, len(m.pending))
	appendPendingChildren := func(parentID uint64) {
		for i, task := range m.pending {
			if task.parentID != parentID {
				continue
			}
			message, color := task.opts.StopMessage, task.opts.StopColor
			if task.err != nil {
				message, color = task.opts.StopFailMessage+": Failed", task.opts.StopFailColor
			}
			lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(color).Render("● "+message))
			renderedPending[i] = true
		}
	}
	for i, task := range m.tasks {
		if i == 0 {
			if root.RetainChildren {
				continue
			}
			if len(m.tasks) == 1 {
				lines = append(lines, m.spinner.View()+" "+getStyle(task.opts.Color).Render(task.opts.TaskName))
			} else {
				lines = append(lines, getStyle(task.opts.Color).Render("● "+task.opts.TaskName))
			}
			lines = appendLiveTaskOutput(lines, 1, task.output.String())
			appendPendingChildren(task.id)
			continue
		}
		prefix := strings.Repeat("  ", task.depth)
		hasActiveChild := false
		for j := i + 1; j < len(m.tasks); j++ {
			if m.tasks[j].depth <= task.depth {
				break
			}
			hasActiveChild = true
		}
		if !hasActiveChild {
			lines = append(lines, prefix+m.spinner.View()+" "+getStyle(task.opts.Color).Render(task.opts.TaskName))
		} else {
			lines = append(lines, prefix+getStyle(task.opts.Color).Render("● "+task.opts.TaskName))
		}
		lines = appendLiveTaskOutput(lines, task.depth+1, task.output.String())
		appendPendingChildren(task.id)
	}
	for _, task := range m.completed {
		message, color := task.opts.StopMessage, task.opts.StopColor
		if task.err != nil {
			message, color = task.opts.StopFailMessage+": Failed", task.opts.StopFailColor
		}
		lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(color).Render("● "+message))
	}
	for i, task := range m.pending {
		if renderedPending[i] {
			continue
		}
		message, color := task.opts.StopMessage, task.opts.StopColor
		if task.err != nil {
			message, color = task.opts.StopFailMessage+": Failed", task.opts.StopFailColor
		}
		lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(color).Render("● "+message))
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

func (m spinnerModel) retainedOutput(finished bool) string {
	root := m.tasks[0].opts
	message, color := root.TaskName, root.Color
	if finished {
		message, color = root.StopMessage, root.StopColor
		if m.interrupt {
			message, color = m.interruptReason, root.StopFailColor
		} else if m.taskErr != nil {
			message, color = root.StopFailMessage+": Failed", root.StopFailColor
		}
	}

	lines := []string{getStyle(color).Render("● " + message)}
	var appendChildren func(uint64)
	appendChildren = func(parentID uint64) {
		for _, task := range m.retained {
			if task.parentID != parentID {
				continue
			}
			taskMessage, taskColor := task.opts.StopMessage, task.opts.StopColor
			if task.err != nil {
				taskMessage, taskColor = task.opts.StopFailMessage+": Failed", task.opts.StopFailColor
			}
			lines = append(lines, strings.Repeat("  ", task.depth)+getStyle(taskColor).Render("● "+taskMessage))
			if task.err != nil {
				lines = appendTaskOutput(lines, task.depth+1, task.output)
			}
			appendChildren(task.id)
		}
	}
	appendChildren(0)
	return strings.Join(lines, "\n")
}

func appendTaskOutput(lines []string, depth int, output string) []string {
	return appendTaskOutputLimit(lines, depth, output, 0)
}

func appendLiveTaskOutput(lines []string, depth int, output string) []string {
	return appendTaskOutputLimit(lines, depth, output, liveTaskOutputLines)
}

func appendTaskOutputLimit(lines []string, depth int, output string, limit int) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return lines
	}
	outputLines := strings.Split(output, "\n")
	if limit > 0 && len(outputLines) > limit {
		outputLines = outputLines[len(outputLines)-limit:]
	}
	prefix := strings.Repeat("  ", depth)
	for _, line := range outputLines {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, prefix+line)
		}
	}
	return lines
}

func getStyle(colorName string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorName))
}
