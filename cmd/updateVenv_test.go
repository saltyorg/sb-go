package cmd

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/saltyorg/sb-go/terminal"
)

func TestUpdateVenvCommandIsHidden(t *testing.T) {
	updateVenvCmd := newUpdateVenvCommand()
	if !updateVenvCmd.Hidden {
		t.Fatal("update-venv command must remain hidden")
	}
	if updateVenvCmd.Use != "update-venv" {
		t.Fatalf("updateVenvCmd.Use = %q, want %q", updateVenvCmd.Use, "update-venv")
	}
	if err := updateVenvCmd.Args(updateVenvCmd, []string{"unexpected"}); err == nil {
		t.Fatal("update-venv command unexpectedly accepted a positional argument")
	}
	if updateVenvCmd.Short != "Update the Ansible virtual environment and saltbox.fact" {
		t.Fatalf("updateVenvCmd.Short = %q", updateVenvCmd.Short)
	}
	if updateVenvCmd.Long != "Update the Ansible virtual environment from the current Saltbox checkout and refresh saltbox.fact without updating Git repositories" {
		t.Fatalf("updateVenvCmd.Long = %q", updateVenvCmd.Long)
	}
}

func TestUpdateVenvTaskSpecDescribesCheckResult(t *testing.T) {
	spec := updateVenvTaskSpec()
	if spec.Running != "Checking Ansible virtual environment and saltbox.fact for updates" {
		t.Fatalf("Running = %q", spec.Running)
	}
	if spec.Success != "Ansible virtual environment and saltbox.fact are ready" {
		t.Fatalf("Success = %q", spec.Success)
	}
}

func TestReconcileSaltboxRuntimeRunsOperationsInOrder(t *testing.T) {
	var calls []string
	operations := saltboxRuntimeOperations{
		cleanupDeadsnakes: func(_ context.Context, verbose bool) (bool, error) {
			if !verbose {
				t.Fatal("cleanup did not receive verbose=true")
			}
			calls = append(calls, "cleanup")
			return false, nil
		},
		manageAnsibleVenv: func(_ context.Context, _ *terminal.Task, force bool, saltboxUser string, verbose bool) error {
			if force {
				t.Fatal("venv reconciliation unexpectedly forced recreation")
			}
			if saltboxUser != "salty" {
				t.Fatalf("saltbox user = %q, want salty", saltboxUser)
			}
			if !verbose {
				t.Fatal("venv reconciliation did not receive verbose=true")
			}
			calls = append(calls, "venv")
			return nil
		},
		updateFact: func(_ context.Context, _ *terminal.Task, alwaysUpdate bool, verbose bool) error {
			if alwaysUpdate {
				t.Fatal("saltbox.fact update unexpectedly forced reinstallation")
			}
			if !verbose {
				t.Fatal("saltbox.fact update did not receive verbose=true")
			}
			calls = append(calls, "fact")
			return nil
		},
	}

	if err := runSaltboxRuntimeTest(t, operations); err != nil {
		t.Fatal(err)
	}
	if want := []string{"cleanup", "venv", "fact"}; !slices.Equal(calls, want) {
		t.Fatalf("operation order = %v, want %v", calls, want)
	}
}

func TestReconcileSaltboxRuntimeStopsAfterFailure(t *testing.T) {
	tests := []struct {
		name      string
		failAt    string
		wantCalls []string
	}{
		{name: "cleanup", failAt: "cleanup", wantCalls: []string{"cleanup"}},
		{name: "venv", failAt: "venv", wantCalls: []string{"cleanup", "venv"}},
		{name: "fact", failAt: "fact", wantCalls: []string{"cleanup", "venv", "fact"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.New("operation failed")
			var calls []string
			operations := saltboxRuntimeOperations{
				cleanupDeadsnakes: func(context.Context, bool) (bool, error) {
					calls = append(calls, "cleanup")
					if test.failAt == "cleanup" {
						return false, failure
					}
					return false, nil
				},
				manageAnsibleVenv: func(context.Context, *terminal.Task, bool, string, bool) error {
					calls = append(calls, "venv")
					if test.failAt == "venv" {
						return failure
					}
					return nil
				},
				updateFact: func(context.Context, *terminal.Task, bool, bool) error {
					calls = append(calls, "fact")
					if test.failAt == "fact" {
						return failure
					}
					return nil
				},
			}

			err := runSaltboxRuntimeTest(t, operations)
			if !errors.Is(err, failure) {
				t.Fatalf("error = %v, want wrapped operation failure", err)
			}
			if !slices.Equal(calls, test.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, test.wantCalls)
			}
		})
	}
}

func TestHandleUpdateVenvRunsSharedRuntimeReconciliation(t *testing.T) {
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: io.Discard})
	called := false
	err := handleUpdateVenvWith(
		t.Context(),
		runner,
		true,
		func() (string, error) { return "salty", nil },
		func(_ context.Context, _ *terminal.Task, saltboxUser string, verbose bool) error {
			called = true
			if saltboxUser != "salty" {
				t.Fatalf("saltbox user = %q, want salty", saltboxUser)
			}
			if !verbose {
				t.Fatal("runtime reconciliation did not receive verbose=true")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("update-venv did not run shared runtime reconciliation")
	}
}

func runSaltboxRuntimeTest(t *testing.T, operations saltboxRuntimeOperations) error {
	t.Helper()
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: io.Discard})
	return runner.Run(t.Context(), updateVenvTaskSpec(), func(ctx context.Context, task *terminal.Task) error {
		return reconcileSaltboxRuntimeWith(ctx, task, "salty", true, operations)
	})
}
