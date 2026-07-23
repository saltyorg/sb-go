package spinners

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/sb-go/internal/styles"

	tea "charm.land/bubbletea/v2"
)

func TestSynchronizedOutputWriterWrapsRendererFrame(t *testing.T) {
	var output bytes.Buffer
	writer := synchronizedOutputWriter{writer: &output}

	if _, err := writer.Write([]byte("\x1b[Jframe")); err != nil {
		t.Fatalf("write renderer frame: %v", err)
	}
	want := "\x1b[?2026h\x1b[Jframe\x1b[?2026l"
	if got := output.String(); got != want {
		t.Fatalf("unexpected synchronized frame: got %q, want %q", got, want)
	}
}

func TestWithDefaultsPreservesCustomMessages(t *testing.T) {
	opts := withDefaults(SpinnerOptions{
		TaskName:        "working",
		StopMessage:     "done",
		StopFailMessage: "not done",
	})

	if opts.StopMessage != "done" || opts.StopFailMessage != "not done" {
		t.Fatalf("custom messages were not preserved: %#v", opts)
	}
	if opts.Color != styles.ColorYellow ||
		opts.StopColor != styles.ColorMediumGreen ||
		opts.StopFailColor != styles.ColorDarkRed {
		t.Fatalf("default colors were not applied: %#v", opts)
	}
}

func TestSpinnerModelWaitsForTerminalCapabilitiesBeforeFastSuccess(t *testing.T) {
	model := newSpinnerModel(withDefaults(SpinnerOptions{TaskName: "fast"}), func() error { return nil })

	updated, cmd := model.Update(successMsg{})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("fast task quit before terminal capability responses could settle")
	}
	if !model.finished {
		t.Fatal("fast task result was not retained while waiting")
	}

	updated, cmd = model.Update(terminalSettledMsg{})
	model = updated.(spinnerModel)
	if cmd == nil {
		t.Fatal("settled, completed task did not quit")
	}
	if !model.terminalSettled {
		t.Fatal("terminal was not marked settled")
	}
}

func TestSpinnerModelQuitsImmediatelyWhenTaskFinishesAfterTerminalSettles(t *testing.T) {
	model := newSpinnerModel(withDefaults(SpinnerOptions{TaskName: "slow"}), func() error { return nil })

	updated, cmd := model.Update(terminalSettledMsg{})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("terminal settling quit a task that was still running")
	}

	_, cmd = model.Update(successMsg{})
	if cmd == nil {
		t.Fatal("task completion did not quit after terminal capabilities settled")
	}
}

func TestSpinnerModelRetainsFastFailureWhileTerminalSettles(t *testing.T) {
	model := newSpinnerModel(withDefaults(SpinnerOptions{TaskName: "fast failure"}), func() error { return nil })
	taskErr := errors.New("failed quickly")

	updated, cmd := model.Update(errMsg{err: taskErr})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("fast failed task quit before terminal capability responses could settle")
	}
	if !errors.Is(model.taskErr, taskErr) {
		t.Fatalf("fast task error was not retained: %v", model.taskErr)
	}

	_, cmd = model.Update(terminalSettledMsg{})
	if cmd == nil {
		t.Fatal("settled, failed task did not quit")
	}
}

func TestFastSpinnerProgramHonorsTerminalCapabilitySettleDelay(t *testing.T) {
	model := newSpinnerModel(withDefaults(SpinnerOptions{TaskName: "fast"}), func() error { return nil })
	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithWindowSize(80, 24),
	)

	started := time.Now()
	if _, err := program.Run(); err != nil {
		t.Fatalf("run fast spinner: %v", err)
	}
	elapsed := time.Since(started)
	if minimum := terminalCapabilitySettleDelay - 25*time.Millisecond; elapsed < minimum {
		t.Fatalf("fast spinner exited before terminal capability responses could settle: %v < %v", elapsed, minimum)
	}
}

func TestSpinnerModelRendersAndRemovesChildren(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root"})
	child := withDefaults(SpinnerOptions{TaskName: "child"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 42, opts: child})
	model = updated.(spinnerModel)
	view := model.View().Content
	if !strings.Contains(view, "root") || !strings.Contains(view, "child") {
		t.Fatalf("child hierarchy not rendered: %q", view)
	}

	updated, cmd := model.Update(popTaskMsg{id: 42, opts: child})
	model = updated.(spinnerModel)
	if cmd == nil {
		t.Fatal("child completion did not produce a persistent line")
	}
	if strings.Contains(model.View().Content, "child") {
		t.Fatalf("completed child still rendered as active: %q", model.View().Content)
	}
}

func TestSpinnerModelRemovesConcurrentChildByID(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root"})
	first := withDefaults(SpinnerOptions{TaskName: "first"})
	second := withDefaults(SpinnerOptions{TaskName: "second"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: first})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: second})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 1, opts: first, err: errors.New("failed")})
	model = updated.(spinnerModel)

	view := model.View().Content
	if strings.Contains(view, "first") || !strings.Contains(view, "second") {
		t.Fatalf("wrong child removed: %q", view)
	}
}

func TestSpinnerModelCollapsesSuccessfulChildren(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", StopMessage: "root done", CollapseChildren: true})
	child := withDefaults(SpinnerOptions{TaskName: "child", StopMessage: "child done"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: child})
	model = updated.(spinnerModel)
	updated, cmd := model.Update(popTaskMsg{id: 1, opts: child})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("collapsible child produced a persistent print command")
	}
	if !strings.Contains(model.View().Content, "child done") {
		t.Fatalf("completed child disappeared before the root task completed: %q", model.View().Content)
	}

	updated, _ = model.Update(successMsg{})
	model = updated.(spinnerModel)
	view := model.View().Content
	if !strings.Contains(view, "root done") || strings.Contains(view, "child done") {
		t.Fatalf("successful child was not collapsed: %q", view)
	}
}

func TestSpinnerModelKeepsFailedChildContext(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", CollapseChildren: true})
	child := withDefaults(SpinnerOptions{TaskName: "child", StopFailMessage: "child failed"})
	model := newSpinnerModel(root, func() error { return nil })
	childErr := errors.New("nope")

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: child})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 1, opts: child, err: childErr})
	model = updated.(spinnerModel)
	updated, _ = model.Update(errMsg{err: childErr})
	model = updated.(spinnerModel)

	if view := model.View().Content; !strings.Contains(view, "child failed") {
		t.Fatalf("failed child context was collapsed: %q", view)
	}
}

func TestSpinnerModelCollapsesHierarchyOnlyAfterEachParentCompletes(t *testing.T) {
	root := withDefaults(SpinnerOptions{
		TaskName:         "Restarting containers",
		StopMessage:      "Containers restarted",
		CollapseChildren: true,
	})
	stop := withDefaults(SpinnerOptions{
		TaskName:         "Stopping containers",
		StopMessage:      "Stopped containers",
		CollapseChildren: true,
	})
	request := withDefaults(SpinnerOptions{
		TaskName:    "Requesting stop job",
		StopMessage: "Requested stop job",
	})
	wait := withDefaults(SpinnerOptions{TaskName: "Waiting for stop job"})
	start := withDefaults(SpinnerOptions{TaskName: "Starting containers"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: stop})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: request})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 2, opts: request})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 3, opts: wait})
	model = updated.(spinnerModel)
	view := model.View().Content
	if !strings.Contains(view, "Requested stop job") || !strings.Contains(view, "Waiting for stop job") {
		t.Fatalf("live stop hierarchy was not preserved: %q", view)
	}

	updated, _ = model.Update(popTaskMsg{id: 3, opts: wait})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 1, opts: stop})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 4, opts: start})
	model = updated.(spinnerModel)
	view = model.View().Content
	if !strings.Contains(view, "Stopped containers") || !strings.Contains(view, "Starting containers") {
		t.Fatalf("completed stop task did not remain during restart: %q", view)
	}
	if strings.Contains(view, "Requested stop job") || strings.Contains(view, "Waiting for stop job") {
		t.Fatalf("stop implementation details remained after stop completed: %q", view)
	}

	updated, _ = model.Update(popTaskMsg{id: 4, opts: start})
	model = updated.(spinnerModel)
	updated, _ = model.Update(successMsg{})
	model = updated.(spinnerModel)
	view = model.View().Content
	if !strings.Contains(view, "Containers restarted") ||
		strings.Contains(view, "Stopped containers") ||
		strings.Contains(view, "Starting containers") {
		t.Fatalf("completed root hierarchy was not collapsed: %q", view)
	}
}

func TestSpinnerModelUsesExplicitIndentForConcurrentSibling(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", CollapseChildren: true})
	file := withDefaults(SpinnerOptions{TaskName: "accounts.yml"})
	api := withDefaults(SpinnerOptions{TaskName: "Cloudflare", IndentLevel: 2})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: file})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: api})
	model = updated.(spinnerModel)
	if view := model.View().Content; !strings.Contains(view, "\n    ") {
		t.Fatalf("API task was not rendered at the explicit nesting level: %q", view)
	}
	updated, cmd := model.Update(popTaskMsg{id: 2, opts: api})
	_ = updated.(spinnerModel)

	if cmd != nil {
		t.Fatal("collapsible API completion produced a persistent result line")
	}
}

func TestSpinnerModelRetainsGrandchildrenUnderParent(t *testing.T) {
	root := withDefaults(SpinnerOptions{
		TaskName:       "root",
		StopMessage:    "Root validated",
		RetainChildren: true,
	})
	file := withDefaults(SpinnerOptions{TaskName: "accounts.yml", StopMessage: "Validated accounts.yml"})
	cloudflare := withDefaults(SpinnerOptions{
		TaskName:    "Cloudflare",
		StopMessage: "Cloudflare validated",
		IndentLevel: 2,
	})
	dockerHub := withDefaults(SpinnerOptions{
		TaskName:    "Docker Hub",
		StopMessage: "Docker Hub validated",
		IndentLevel: 2,
	})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: file})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: cloudflare})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 3, opts: dockerHub})
	model = updated.(spinnerModel)
	updated, cmd := model.Update(popTaskMsg{id: 3, opts: dockerHub})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("retained Docker Hub result produced terminal output")
	}
	updated, cmd = model.Update(popTaskMsg{id: 2, opts: cloudflare})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("retained Cloudflare result produced terminal output")
	}
	if len(model.retained) != 2 || model.retained[0].parentID != 1 || model.retained[1].parentID != 1 {
		t.Fatalf("API checks were not retained under accounts.yml: %#v", model.retained)
	}

	updated, cmd = model.Update(popTaskMsg{id: 1, opts: file})
	model = updated.(spinnerModel)
	if cmd != nil {
		t.Fatal("retained accounts.yml result produced terminal output")
	}
	updated, _ = model.Update(successMsg{})
	model = updated.(spinnerModel)

	printed := model.retainedOutput(true)
	rootAt := strings.Index(printed, "Root validated")
	accountsAt := strings.Index(printed, "Validated accounts.yml")
	dockerHubAt := strings.Index(printed, "Docker Hub validated")
	cloudflareAt := strings.Index(printed, "Cloudflare validated")
	if rootAt < 0 || accountsAt < 0 || dockerHubAt < 0 || cloudflareAt < 0 ||
		rootAt > accountsAt || accountsAt > dockerHubAt || accountsAt > cloudflareAt {
		t.Fatalf("accounts.yml and its API checks were printed out of order: %s", printed)
	}
	if view := model.View().Content; !strings.Contains(view, "Root validated") ||
		!strings.Contains(view, "Validated accounts.yml") {
		t.Fatalf("retained final result was not rendered in the viewport: %q", view)
	}
}

func TestSpinnerModelCollapsesSuccessfulGrandchildUnderRetainedParent(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", RetainChildren: true})
	parent := withDefaults(SpinnerOptions{
		TaskName:         "Stopping containers",
		StopMessage:      "Stopped containers",
		CollapseChildren: true,
	})
	wait := withDefaults(SpinnerOptions{
		TaskName:        "Waiting for stop job",
		StopMessage:     "Stop job completed",
		StopFailMessage: "Stop job",
	})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: parent})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: wait})
	model = updated.(spinnerModel)
	if view := model.View().Content; !strings.Contains(view, "Waiting for stop job") {
		t.Fatalf("active grandchild was not displayed: %q", view)
	}

	updated, _ = model.Update(popTaskMsg{id: 2, opts: wait})
	model = updated.(spinnerModel)
	if view := model.View().Content; !strings.Contains(view, "Stop job completed") {
		t.Fatalf("completed grandchild disappeared before its parent completed: %q", view)
	}

	next := withDefaults(SpinnerOptions{TaskName: "Waiting for another job"})
	updated, _ = model.Update(pushTaskMsg{id: 3, opts: next})
	model = updated.(spinnerModel)
	view := model.View().Content
	completedAt := strings.Index(view, "Stop job completed")
	nextAt := strings.Index(view, "Waiting for another job")
	if completedAt < 0 || nextAt < 0 || completedAt > nextAt {
		t.Fatalf("completed child rendered after its later active sibling: %q", view)
	}
	updated, _ = model.Update(popTaskMsg{id: 3, opts: next})
	model = updated.(spinnerModel)

	updated, _ = model.Update(popTaskMsg{id: 1, opts: parent})
	model = updated.(spinnerModel)
	if len(model.retained) != 1 || model.retained[0].id != 1 {
		t.Fatalf("retained tasks = %#v, want only the parent", model.retained)
	}
	if view := model.View().Content; strings.Contains(view, "Stop job completed") {
		t.Fatalf("successful grandchild remained after its parent completed: %q", view)
	}
}

func TestSpinnerModelRetainsFailedGrandchildUnderRetainedParent(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", RetainChildren: true})
	parent := withDefaults(SpinnerOptions{
		TaskName:         "Stopping containers",
		StopFailMessage:  "Stop containers",
		CollapseChildren: true,
	})
	wait := withDefaults(SpinnerOptions{
		TaskName:        "Waiting for stop job",
		StopFailMessage: "Stop job",
	})
	model := newSpinnerModel(root, func() error { return nil })
	waitErr := errors.New("poll failed")

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: parent})
	model = updated.(spinnerModel)
	updated, _ = model.Update(pushTaskMsg{id: 2, opts: wait})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 2, opts: wait, err: waitErr})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 1, opts: parent, err: waitErr})
	model = updated.(spinnerModel)

	if len(model.retained) != 2 || model.retained[0].id != 2 || model.retained[1].id != 1 {
		t.Fatalf("failed hierarchy was not retained: %#v", model.retained)
	}
}

func TestSpinnerModelStreamsAndHidesSuccessfulOutput(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", RetainChildren: true})
	pip := withDefaults(SpinnerOptions{TaskName: "Installing pip", StopMessage: "Installed pip"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: pip})
	model = updated.(spinnerModel)
	updated, _ = model.Update(taskOutputMsg{id: 1, output: "Downloading package\nInstalling package\n"})
	model = updated.(spinnerModel)
	if view := model.View().Content; !strings.Contains(view, "Downloading package") {
		t.Fatalf("live task output was not rendered: %q", view)
	}

	updated, _ = model.Update(popTaskMsg{id: 1, opts: pip})
	model = updated.(spinnerModel)
	updated, _ = model.Update(successMsg{})
	model = updated.(spinnerModel)
	if view := model.View().Content; strings.Contains(view, "Downloading package") {
		t.Fatalf("successful task output remained visible: %q", view)
	}
}

func TestSpinnerModelRewritesProgressOutput(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root"})
	pip := withDefaults(SpinnerOptions{TaskName: "Installing pip"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: pip})
	model = updated.(spinnerModel)
	updated, _ = model.Update(taskOutputMsg{
		id:     1,
		output: "Downloading 10%\rDownloading 80%\rDownloading 100%",
	})
	model = updated.(spinnerModel)

	view := model.View().Content
	if !strings.Contains(view, "Downloading 100%") {
		t.Fatalf("latest progress output was not rendered: %q", view)
	}
	if strings.Contains(view, "Downloading 10%") || strings.Contains(view, "Downloading 80%") {
		t.Fatalf("replaced progress output remained visible: %q", view)
	}
}

func TestSpinnerModelKeepsAnimatingAboveLiveOutput(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "Installing pip"})
	model := newSpinnerModel(root, func() error { return nil })

	updated, _ := model.Update(taskOutputMsg{id: 0, output: "Collecting wheel\n"})
	model = updated.(spinnerModel)
	first := model.View().Content

	updated, _ = model.Update(model.spinner.Tick())
	model = updated.(spinnerModel)
	second := model.View().Content

	if first == second {
		t.Fatalf("spinner stopped animating above live output: %q", first)
	}
}

func TestSpinnerModelLimitsLiveOutputButRetainsFailureDetails(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "Installing pip"})
	model := newSpinnerModel(root, func() error { return nil })

	var output strings.Builder
	for i := range liveTaskOutputLines + 3 {
		fmt.Fprintf(&output, "line %d\n", i)
	}
	updated, _ := model.Update(taskOutputMsg{id: 0, output: output.String()})
	model = updated.(spinnerModel)

	liveView := model.View().Content
	if strings.Contains(liveView, "line 0") || !strings.Contains(liveView, "line 10") {
		t.Fatalf("live output was not limited to its newest lines: %q", liveView)
	}

	taskErr := errors.New("pip failed")
	updated, _ = model.Update(errMsg{err: taskErr})
	model = updated.(spinnerModel)
	failedView := model.View().Content
	if !strings.Contains(failedView, "line 0") || !strings.Contains(failedView, "line 10") {
		t.Fatalf("failed output did not retain the complete command log: %q", failedView)
	}
}

func TestTaskOutputBufferHandlesNewlinesAndBackspaces(t *testing.T) {
	var output taskOutputBuffer
	output.WriteString("first\r\nabc\b\bXY")

	if got, want := output.String(), "first\naXY"; got != want {
		t.Fatalf("unexpected terminal output: got %q, want %q", got, want)
	}
}

func TestTaskOutputBufferHandlesSplitANSIProgressUpdates(t *testing.T) {
	var output taskOutputBuffer
	output.WriteString("\x1b[?25lDownloading 10%\r\x1b[")
	output.WriteString("2KDone\n\x1b[?25h")

	if got, want := output.String(), "Done"; got != want {
		t.Fatalf("unexpected terminal progress output: got %q, want %q", got, want)
	}
}

func TestSpinnerModelRetainsFailedOutput(t *testing.T) {
	root := withDefaults(SpinnerOptions{TaskName: "root", CollapseChildren: true})
	pip := withDefaults(SpinnerOptions{TaskName: "Installing pip", StopFailMessage: "Install pip"})
	model := newSpinnerModel(root, func() error { return nil })
	taskErr := errors.New("pip failed")

	updated, _ := model.Update(pushTaskMsg{id: 1, opts: pip})
	model = updated.(spinnerModel)
	updated, _ = model.Update(taskOutputMsg{id: 1, output: "No matching distribution found\n"})
	model = updated.(spinnerModel)
	updated, _ = model.Update(popTaskMsg{id: 1, opts: pip, err: taskErr})
	model = updated.(spinnerModel)
	updated, _ = model.Update(errMsg{err: taskErr})
	model = updated.(spinnerModel)

	if view := model.View().Content; !strings.Contains(view, "No matching distribution found") {
		t.Fatalf("failed task output was not retained: %q", view)
	}
}
