package main

import (
	"context"
	"fmt"
	"github.com/saltyorg/sb-go/python"
	"github.com/saltyorg/sb-go/terminal"
	"os"
	"time"
)

// This harness exercises the production reconciler only in the disposable
// container created by test-python-isolation-container.sh.
func main() {
	if _, err := os.Stat("/.dockerenv"); err != nil {
		fmt.Fprintln(os.Stderr, "Python lifecycle acceptance must run in its disposable container")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	options := python.Options{Verbose: true}
	if len(os.Args) > 1 {
		options.ForceVenv = os.Args[1] == "force-venv"
		options.ForcePython = os.Args[1] == "force-python"
	}
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true})
	err := runner.Run(ctx, terminal.TaskSpec{Running: "Python isolation acceptance"}, func(ctx context.Context, task *terminal.Task) error {
		return python.Reconcile(ctx, task, options)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
