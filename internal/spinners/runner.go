package spinners

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/saltyorg/sb-go/internal/executor"
	"github.com/saltyorg/sb-go/internal/styles"
	"github.com/saltyorg/sb-go/internal/tty"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ChildDisplay controls how successful direct children are shown after their
// parent completes.
type ChildDisplay uint8

const (
	// RetainChildTasks is the safe default: tasks remain in the live tree
	// unless their caller explicitly chooses to collapse or print them.
	RetainChildTasks ChildDisplay = iota
	CollapseChildTasks
	PrintChildTasks
)

// TaskSpec describes one task and how its direct children behave when it
// completes. Failed descendants are always retained.
type TaskSpec struct {
	Running      string
	Success      string
	Failure      string
	ChildDisplay ChildDisplay
}

// RunnerOptions configures one independent progress renderer.
type RunnerOptions struct {
	Verbose bool
	// NoProgress suppresses task lifecycle output while preserving notices and
	// output written by the work itself.
	NoProgress bool
	Output     io.Writer
}

// Runner owns one progress session. It contains no process-global state.
type Runner struct {
	verbose    bool
	noProgress bool
	output     io.Writer
	mu         sync.Mutex
}

// NewRunner creates an independent progress runner.
func NewRunner(opts RunnerOptions) *Runner {
	output := opts.Output
	if output == nil {
		output = os.Stderr
	}
	return &Runner{
		verbose:    opts.Verbose || opts.NoProgress || !tty.IsInteractive(),
		noProgress: opts.NoProgress,
		output:     output,
	}
}

// Verbose reports whether this runner uses plain text output.
func (r *Runner) Verbose() bool {
	return r.verbose
}

// Info prints an informational message outside a task scope.
func (r *Runner) Info(message string) {
	r.printMessage(message, styles.ColorLightBlue)
}

// Warning prints a warning message outside a task scope.
func (r *Runner) Warning(message string) {
	r.printMessage(message, styles.ColorYellow)
}

// Task is an explicit scope for creating children under one parent.
type Task struct {
	run   *taskRun
	id    uint64
	depth int
}

// Verbose reports whether this task's runner uses plain text output.
func (t *Task) Verbose() bool {
	return t.run.runner.verbose
}

type taskRun struct {
	runner  *Runner
	nextID  atomic.Uint64
	program *tea.Program
}

// Run executes a root task and owns the renderer until it completes.
func (r *Runner) Run(
	ctx context.Context,
	spec TaskSpec,
	fn func(context.Context, *Task) error,
) error {
	spec = normalizeTaskSpec(spec)
	if err := validateTaskSpec(spec); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("root task function is required")
	}
	run := &taskRun{runner: r}
	root := &Task{run: run, id: 0}
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if r.noProgress {
		return fn(taskCtx, root)
	}

	if r.verbose {
		r.printPlain(0, spec.Running+"...")
		err := fn(taskCtx, root)
		if err != nil {
			r.printPlain(0, spec.Failure+": Failed")
			return err
		}
		r.printPlain(0, spec.Success)
		return nil
	}

	result := make(chan error, 1)
	model := newProgressModel(spec, func() error {
		err := fn(taskCtx, root)
		result <- err
		return err
	}, cancel)
	program := tea.NewProgram(model, tea.WithOutput(&synchronizedOutputWriter{writer: r.output}))
	run.program = program

	renderDone := make(chan struct{})
	go func() {
		select {
		case <-taskCtx.Done():
			program.Send(progressCancelMsg{})
		case <-renderDone:
		}
	}()

	_, runErr := program.Run()
	close(renderDone)
	if runErr != nil {
		return fmt.Errorf("run progress renderer: %w", runErr)
	}
	select {
	case taskErr := <-result:
		return taskErr
	case <-taskCtx.Done():
		return taskCtx.Err()
	}
}

// RunOutput executes a root task with task-scoped stdout and stderr writers.
func (r *Runner) RunOutput(
	ctx context.Context,
	spec TaskSpec,
	fn func(context.Context, io.Writer, io.Writer) error,
) error {
	if fn == nil {
		return fmt.Errorf("root output task function is required")
	}
	return r.Run(ctx, spec, func(ctx context.Context, root *Task) error {
		if r.verbose {
			return fn(ctx, r.output, r.output)
		}
		writer := &progressOutputWriter{program: root.run.program, id: root.id}
		return fn(ctx, writer, writer)
	})
}

// Run executes a child task under the receiver.
func (t *Task) Run(
	ctx context.Context,
	spec TaskSpec,
	fn func(context.Context, *Task) error,
) error {
	return t.runTask(ctx, spec, nil, fn)
}

// RunOutput executes a child task with task-scoped stdout and stderr writers.
func (t *Task) RunOutput(
	ctx context.Context,
	spec TaskSpec,
	fn func(context.Context, io.Writer, io.Writer) error,
) error {
	return t.runTask(ctx, spec, fn, nil)
}

// RunStreaming executes work that writes through executor's managed output.
func (t *Task) RunStreaming(
	ctx context.Context,
	spec TaskSpec,
	fn func(context.Context) error,
) error {
	return t.RunOutput(ctx, spec, func(taskCtx context.Context, stdout, stderr io.Writer) error {
		return fn(executor.WithManagedOutput(taskCtx, stdout, stderr))
	})
}

func (t *Task) runTask(
	ctx context.Context,
	spec TaskSpec,
	outputFn func(context.Context, io.Writer, io.Writer) error,
	fn func(context.Context, *Task) error,
) error {
	spec = normalizeTaskSpec(spec)
	if err := validateTaskSpec(spec); err != nil {
		return err
	}
	if outputFn == nil && fn == nil {
		return fmt.Errorf("child task function is required")
	}
	id := t.run.nextID.Add(1)
	child := &Task{run: t.run, id: id, depth: t.depth + 1}

	if t.run.runner.noProgress {
		if outputFn != nil {
			return outputFn(ctx, t.run.runner.output, t.run.runner.output)
		}
		return fn(ctx, child)
	}

	if t.run.runner.verbose {
		depth := child.depth
		t.run.runner.printPlain(depth, spec.Running+"...")
		var err error
		if outputFn != nil {
			err = outputFn(ctx, t.run.runner.output, t.run.runner.output)
		} else {
			err = fn(ctx, child)
		}
		if err != nil {
			t.run.runner.printPlain(depth, spec.Failure+": Failed")
		} else {
			t.run.runner.printPlain(depth, spec.Success)
		}
		return err
	}

	t.run.program.Send(progressStartMsg{id: id, parentID: t.id, spec: spec})
	var err error
	var failureOutput string
	if outputFn != nil {
		stdout := &outputCapture{}
		stderr := &outputCapture{}
		err = outputFn(ctx,
			&progressOutputWriter{program: t.run.program, id: id, capture: stdout},
			&progressOutputWriter{program: t.run.program, id: id, capture: stderr},
		)
		if err != nil {
			// Prefer stderr so routine stdout progress does not obscure the
			// diagnostic. Pseudo-terminal commands combine both streams on
			// stdout, so fall back to it when stderr is empty.
			failureOutput = stderr.String()
			if strings.TrimSpace(failureOutput) == "" {
				failureOutput = stdout.String()
			}
		}
	} else {
		err = fn(ctx, child)
	}
	t.run.program.Send(progressFinishMsg{id: id, err: err, failureOutput: failureOutput})
	return err
}

// Info prints an informational message without disturbing the live renderer.
func (t *Task) Info(message string) {
	t.message(message, styles.ColorLightBlue)
}

// Warning prints a warning message without disturbing the live renderer.
func (t *Task) Warning(message string) {
	t.message(message, styles.ColorYellow)
}

func (t *Task) message(message, color string) {
	if t.run.runner.verbose {
		t.run.runner.printPlain(t.depth+1, message)
		return
	}
	t.run.program.Send(progressNoticeMsg{id: t.id, message: message, color: color})
}

func (r *Runner) printPlain(depth int, message string) {
	// Multiple concurrent tasks may report plain output.
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.output, strings.Repeat("  ", depth)+message)
}

func (r *Runner) printMessage(message, color string) {
	if r.verbose {
		r.printPlain(0, message)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.output, getStyle(color).Render("● "+message))
}

func normalizeTaskSpec(spec TaskSpec) TaskSpec {
	if spec.Success == "" {
		spec.Success = spec.Running
	}
	if spec.Failure == "" {
		spec.Failure = spec.Running
	}
	return spec
}

func validateTaskSpec(spec TaskSpec) error {
	if strings.TrimSpace(spec.Running) == "" {
		return fmt.Errorf("task running message is required")
	}
	if spec.ChildDisplay > PrintChildTasks {
		return fmt.Errorf("invalid child display mode %d", spec.ChildDisplay)
	}
	return nil
}

type progressTaskState uint8

const (
	progressRunning progressTaskState = iota
	progressSucceeded
	progressFailed
)

type progressNode struct {
	id       uint64
	parentID uint64
	order    uint64
	spec     TaskSpec
	state    progressTaskState
	err      error
	output   taskOutputBuffer
	notices  []progressNotice
	children []uint64
	detached bool
}

type progressModel struct {
	spinner         spinner.Model
	nodes           map[uint64]*progressNode
	rootID          uint64
	nextOrder       uint64
	taskFunc        func() error
	cancel          context.CancelFunc
	taskErr         error
	finished        bool
	terminalSettled bool
	cancelled       bool
}

type progressStartMsg struct {
	id       uint64
	parentID uint64
	spec     TaskSpec
}

type progressFinishMsg struct {
	id            uint64
	err           error
	failureOutput string
}

type progressOutputMsg struct {
	id     uint64
	output string
}

type progressNoticeMsg struct {
	id      uint64
	message string
	color   string
}

type progressNotice struct {
	message string
	color   string
}

type progressSuccessMsg struct{}
type progressErrorMsg struct{ err error }
type progressSettledMsg struct{}
type progressCancelMsg struct{}

func newProgressModel(root TaskSpec, taskFunc func() error, cancels ...context.CancelFunc) progressModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.ColorMagenta))
	cancel := func() {}
	if len(cancels) > 0 && cancels[0] != nil {
		cancel = cancels[0]
	}
	return progressModel{
		spinner: s,
		nodes: map[uint64]*progressNode{
			0: {
				id:    0,
				spec:  normalizeTaskSpec(root),
				state: progressRunning,
			},
		},
		taskFunc: taskFunc,
		cancel:   cancel,
	}
}

func (m progressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.Tick(terminalCapabilitySettleDelay, func(time.Time) tea.Msg {
		return progressSettledMsg{}
	}), func() tea.Msg {
		if err := m.taskFunc(); err != nil {
			return progressErrorMsg{err: err}
		}
		return progressSuccessMsg{}
	})
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
			m.cancelled = true
			return m, tea.Quit
		}
	case progressCancelMsg:
		m.cancelled = true
		return m, tea.Quit
	case progressStartMsg:
		parent, ok := m.nodes[msg.parentID]
		if !ok {
			return m, nil
		}
		m.nextOrder++
		m.nodes[msg.id] = &progressNode{
			id:       msg.id,
			parentID: msg.parentID,
			order:    m.nextOrder,
			spec:     normalizeTaskSpec(msg.spec),
			state:    progressRunning,
		}
		parent.children = append(parent.children, msg.id)
	case progressFinishMsg:
		if node, ok := m.nodes[msg.id]; ok {
			node.err = msg.err
			if msg.err != nil {
				node.state = progressFailed
				if strings.TrimSpace(msg.failureOutput) != "" {
					node.output = taskOutputBuffer{}
					node.output.WriteString(msg.failureOutput)
				}
			} else {
				node.state = progressSucceeded
			}
			if parent := m.nodes[node.parentID]; parent != nil &&
				parent.spec.ChildDisplay == PrintChildTasks &&
				node.state == progressSucceeded {
				lines := m.renderNode(node.id, m.nodeDepth(node.id), true)
				node.detached = true
				return m, tea.Println(strings.Join(lines, "\n"))
			}
		}
	case progressOutputMsg:
		if node, ok := m.nodes[msg.id]; ok {
			node.output.WriteString(msg.output)
		}
	case progressNoticeMsg:
		if node, ok := m.nodes[msg.id]; ok {
			node.notices = append(node.notices, progressNotice{
				message: msg.message,
				color:   msg.color,
			})
		}
	case progressSuccessMsg:
		m.finished = true
		m.nodes[m.rootID].state = progressSucceeded
		if m.terminalSettled {
			return m, tea.Quit
		}
	case progressErrorMsg:
		m.finished = true
		m.taskErr = msg.err
		root := m.nodes[m.rootID]
		root.state = progressFailed
		root.err = msg.err
		if m.terminalSettled {
			return m, tea.Quit
		}
	case progressSettledMsg:
		m.terminalSettled = true
		if m.finished {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m progressModel) View() tea.View {
	if m.cancelled {
		return tea.NewView(getStyle(styles.ColorDarkRed).Render("● interrupted") + "\n")
	}
	lines := m.renderNode(m.rootID, 0, true)
	return tea.NewView(strings.Join(lines, "\n") + "\n")
}

func (m progressModel) renderNode(id uint64, depth int, forceVisible bool) []string {
	node, ok := m.nodes[id]
	if !ok {
		return nil
	}
	if !forceVisible && !m.nodeVisible(node) {
		return nil
	}

	prefix := strings.Repeat("  ", depth)
	message := node.spec.Running
	color := styles.ColorYellow
	marker := "●"
	switch node.state {
	case progressRunning:
		marker = m.spinner.View()
	case progressSucceeded:
		message = node.spec.Success
		color = styles.ColorMediumGreen
	case progressFailed:
		message = node.spec.Failure + ": Failed"
		color = styles.ColorDarkRed
	}
	line := prefix + marker + " " + getStyle(color).Render(message)
	if node.state != progressRunning {
		line = prefix + getStyle(color).Render(marker+" "+message)
	}
	lines := []string{line}
	if node.state == progressRunning || node.state == progressFailed {
		lines = appendLiveTaskOutput(lines, depth+1, node.output.String())
	}
	for _, notice := range node.notices {
		lines = append(lines,
			strings.Repeat("  ", depth+1)+getStyle(notice.color).Render("● "+notice.message),
		)
	}

	for _, childID := range node.children {
		child := m.nodes[childID]
		if child == nil || !m.childVisible(node, child) {
			continue
		}
		lines = append(lines, m.renderNode(childID, depth+1, true)...)
	}
	return lines
}

func (m progressModel) nodeVisible(node *progressNode) bool {
	parent := m.nodes[node.parentID]
	return parent == nil || m.childVisible(parent, node)
}

func (m progressModel) childVisible(parent, child *progressNode) bool {
	if child.detached {
		return false
	}
	if child.state == progressFailed || m.hasFailedDescendant(child) {
		return true
	}
	if parent.state == progressRunning {
		return true
	}
	switch parent.spec.ChildDisplay {
	case RetainChildTasks:
		return true
	case CollapseChildTasks:
		return false
	case PrintChildTasks:
		return true
	default:
		return true
	}
}

func (m progressModel) nodeDepth(id uint64) int {
	depth := 0
	for id != m.rootID {
		node := m.nodes[id]
		if node == nil {
			break
		}
		depth++
		id = node.parentID
	}
	return depth
}

func (m progressModel) hasFailedDescendant(node *progressNode) bool {
	for _, childID := range node.children {
		child := m.nodes[childID]
		if child == nil {
			continue
		}
		if child.state == progressFailed || m.hasFailedDescendant(child) {
			return true
		}
	}
	return false
}

type progressOutputWriter struct {
	program *tea.Program
	id      uint64
	capture *outputCapture
}

func (w *progressOutputWriter) Write(output []byte) (int, error) {
	if w.capture != nil {
		_, _ = w.capture.Write(output)
	}
	w.program.Send(progressOutputMsg{id: w.id, output: string(output)})
	return len(output), nil
}

type outputCapture struct {
	mu     sync.Mutex
	output strings.Builder
}

func (c *outputCapture) Write(output []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.Write(output)
}

func (c *outputCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}
