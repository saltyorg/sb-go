package spinners

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/saltyorg/sb-go/internal/styles"
)

func TestProgressTreeUsesExplicitParentsAndCreationOrder(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", ChildDisplay: RetainChildTasks}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "first"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressStartMsg{id: 2, parentID: 0, spec: TaskSpec{Running: "second"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressStartMsg{id: 3, parentID: 1, spec: TaskSpec{Running: "nested"}})
	model = updated.(progressModel)

	if got := model.nodes[3].parentID; got != 1 {
		t.Fatalf("nested parent = %d, want 1", got)
	}
	view := model.View().Content
	firstAt := strings.Index(view, "first")
	nestedAt := strings.Index(view, "nested")
	secondAt := strings.Index(view, "second")
	if firstAt < 0 || nestedAt < firstAt || secondAt < nestedAt {
		t.Fatalf("tree rendered out of order: %q", view)
	}
}

func TestCollapseKeepsChildrenLiveThenHidesThemOnSuccess(t *testing.T) {
	model := newProgressModel(TaskSpec{
		Running:      "restart",
		Success:      "restarted",
		ChildDisplay: CollapseChildTasks,
	}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "stop", Success: "stopped", ChildDisplay: CollapseChildTasks}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressStartMsg{id: 2, parentID: 1, spec: TaskSpec{Running: "request", Success: "requested"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 2})
	model = updated.(progressModel)
	updated, _ = model.Update(progressStartMsg{id: 3, parentID: 1, spec: TaskSpec{Running: "waiting"}})
	model = updated.(progressModel)

	if view := model.View().Content; !strings.Contains(view, "requested") || !strings.Contains(view, "waiting") {
		t.Fatalf("live descendants disappeared too early: %q", view)
	}

	updated, _ = model.Update(progressFinishMsg{id: 3})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)
	view := model.View().Content
	if !strings.Contains(view, "stopped") || strings.Contains(view, "requested") || strings.Contains(view, "waiting") {
		t.Fatalf("child boundary collapsed incorrectly: %q", view)
	}

	updated, _ = model.Update(progressSuccessMsg{})
	model = updated.(progressModel)
	output := model.finalOutput()
	if !strings.Contains(output, "restarted") || strings.Contains(output, "stopped") {
		t.Fatalf("root hierarchy was not collapsed: %q", output)
	}
}

func TestRetainKeepsCompletedHierarchy(t *testing.T) {
	model := newProgressModel(TaskSpec{
		Running:      "root",
		Success:      "root done",
		ChildDisplay: RetainChildTasks,
	}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "child", Success: "child done"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)
	updated, _ = model.Update(progressSuccessMsg{})
	model = updated.(progressModel)
	if output := model.finalOutput(); !strings.Contains(output, "root done") || !strings.Contains(output, "child done") {
		t.Fatalf("retained hierarchy missing: %q", output)
	}
}

func TestRetainIsTheDefaultChildDisplay(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root"}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "child", Success: "child done"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)
	updated, _ = model.Update(progressSuccessMsg{})
	model = updated.(progressModel)

	if output := model.finalOutput(); !strings.Contains(output, "child done") {
		t.Fatalf("default child display did not retain completed child: %q", output)
	}
	if model.nodes[1].detached {
		t.Fatal("default child display detached completed child")
	}
}

func TestCompletedMarkerUsesTaskResultColor(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", Success: "done"}, func() error { return nil })
	updated, _ := model.Update(progressSuccessMsg{})
	model = updated.(progressModel)

	want := getStyle("40").Render("● done")
	if output := model.finalOutput(); !strings.Contains(output, want) {
		t.Fatalf("completed marker and message were not styled together: %q", output)
	}
}

func TestFailureRetainsAncestorPathAndOutput(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", Failure: "root", ChildDisplay: CollapseChildTasks}, func() error { return nil })
	childErr := errors.New("failed")
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "child", Failure: "child"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressOutputMsg{id: 1, output: "diagnostic output\n"})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1, err: childErr})
	model = updated.(progressModel)
	updated, _ = model.Update(progressErrorMsg{err: childErr})
	model = updated.(progressModel)

	output := model.finalOutput()
	if !strings.Contains(output, "root: Failed") ||
		!strings.Contains(output, "child: Failed") ||
		!strings.Contains(output, "diagnostic output") {
		t.Fatalf("failure context missing: %q", output)
	}
}

func TestFailurePrefersFinalDiagnosticOutput(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", Failure: "root"}, func() error { return nil })
	childErr := errors.New("failed")
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "install", Failure: "install"}})
	model = updated.(progressModel)
	updated, _ = model.Update(progressOutputMsg{id: 1, output: "Reading package lists...\n"})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{
		id:            1,
		err:           childErr,
		failureOutput: "E: Unable to locate package invalid\n",
	})
	model = updated.(progressModel)
	updated, _ = model.Update(progressErrorMsg{err: childErr})
	model = updated.(progressModel)

	output := model.finalOutput()
	if !strings.Contains(output, "E: Unable to locate package invalid") {
		t.Fatalf("final diagnostic output missing: %q", output)
	}
	if strings.Contains(output, "Reading package lists") {
		t.Fatalf("routine progress remained in failed task output: %q", output)
	}
}

func TestTaskNoticeRemainsAttachedToCompletedTask(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", Success: "root done"}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{
		id:       1,
		parentID: 0,
		spec:     TaskSpec{Running: "fact", Success: "fact ready"},
	})
	model = updated.(progressModel)
	updated, _ = model.Update(progressNoticeMsg{
		id:      1,
		message: "saltbox.fact updated successfully: 1.0.7 → v1.0.8",
		color:   styles.ColorLightBlue,
	})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)
	updated, _ = model.Update(progressSuccessMsg{})
	model = updated.(progressModel)

	output := model.finalOutput()
	taskAt := strings.Index(output, "fact ready")
	noticeAt := strings.Index(output, "saltbox.fact updated successfully")
	if taskAt < 0 || noticeAt < taskAt {
		t.Fatalf("task notice was not retained beneath its task: %q", output)
	}
	noticeLine := ""
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "saltbox.fact updated successfully") {
			noticeLine = line
			break
		}
	}
	if !strings.HasPrefix(noticeLine, "    ") {
		t.Fatalf("task notice did not use task-relative indentation: %q", output)
	}
}

func TestFinishedViewClearsTransientFrameBeforeFinalOutput(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", Success: "root done"}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{
		id:       1,
		parentID: 0,
		spec:     TaskSpec{Running: "child", Success: "child done"},
	})
	model = updated.(progressModel)
	updated, _ = model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)
	updated, _ = model.Update(progressSuccessMsg{})
	model = updated.(progressModel)

	if view := model.View().Content; view != "" {
		t.Fatalf("finished view retained transient content: %q", view)
	}
	if output := model.finalOutput(); !strings.Contains(output, "root done") || !strings.Contains(output, "child done") {
		t.Fatalf("final output lost retained tree: %q", output)
	}
}

func TestPrintChildrenDetachesSuccessfulChild(t *testing.T) {
	model := newProgressModel(TaskSpec{Running: "root", ChildDisplay: PrintChildTasks}, func() error { return nil })
	updated, _ := model.Update(progressStartMsg{id: 1, parentID: 0, spec: TaskSpec{Running: "child", Success: "child done"}})
	model = updated.(progressModel)
	updated, cmd := model.Update(progressFinishMsg{id: 1})
	model = updated.(progressModel)

	if cmd == nil {
		t.Fatal("print policy did not produce a persistent output command")
	}
	if view := model.View().Content; strings.Contains(view, "child done") {
		t.Fatalf("printed child remained in live hierarchy: %q", view)
	}
}

func TestRunnerPlainModeIsIndependent(t *testing.T) {
	var first, second bytes.Buffer
	firstRunner := NewRunner(RunnerOptions{Verbose: true, Output: &first})
	secondRunner := NewRunner(RunnerOptions{Verbose: true, Output: &second})

	if err := firstRunner.Run(context.Background(), TaskSpec{Running: "one"}, func(context.Context, *Task) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := secondRunner.Run(context.Background(), TaskSpec{Running: "two"}, func(context.Context, *Task) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(first.String(), "two") || strings.Contains(second.String(), "one") {
		t.Fatalf("runner output leaked: first=%q second=%q", first.String(), second.String())
	}
}

func TestRunnerRejectsInvalidTaskSpec(t *testing.T) {
	runner := NewRunner(RunnerOptions{Verbose: true, Output: io.Discard})
	err := runner.Run(context.Background(), TaskSpec{
		Running:      "invalid",
		ChildDisplay: ChildDisplay(99),
	}, func(context.Context, *Task) error {
		return nil
	})
	if err == nil {
		t.Fatal("invalid child display mode was accepted")
	}
}

func TestPlainRunnerUsesFlatOutput(t *testing.T) {
	var output bytes.Buffer
	runner := NewRunner(RunnerOptions{Verbose: true, Output: &output})
	err := runner.Run(context.Background(), TaskSpec{Running: "root"}, func(ctx context.Context, root *Task) error {
		return root.Run(ctx, TaskSpec{Running: "parent"}, func(ctx context.Context, parent *Task) error {
			parent.Info("notice")
			return parent.Run(ctx, TaskSpec{Running: "child"}, func(context.Context, *Task) error {
				return nil
			})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "root...\nparent...\nnotice\nchild...\nchild\nparent\nroot\n"
	if got := output.String(); got != want {
		t.Fatalf("plain output = %q, want %q", got, want)
	}
}

func TestRunnerSupportsConcurrentExplicitSiblings(t *testing.T) {
	runner := NewRunner(RunnerOptions{Verbose: true, Output: io.Discard})
	err := runner.Run(context.Background(), TaskSpec{Running: "root"}, func(ctx context.Context, root *Task) error {
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, name := range []string{"first", "second"} {
			wg.Go(func() {
				errs <- root.Run(ctx, TaskSpec{Running: name}, func(context.Context, *Task) error {
					return nil
				})
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunnerPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := NewRunner(RunnerOptions{Verbose: true, Output: io.Discard})
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, TaskSpec{Running: "root"}, func(ctx context.Context, _ *Task) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("runner error = %v, want context cancellation", err)
	}
}

func TestTaskOutputBufferRewritesCarriageReturnProgress(t *testing.T) {
	var output taskOutputBuffer
	output.WriteString("Downloading 10%\rDownloading 80%\rDownloading 100%")
	if got := output.String(); got != "Downloading 100%" {
		t.Fatalf("output = %q", got)
	}
}
