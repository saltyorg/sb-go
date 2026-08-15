package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/terminal"
)

func TestDisabledBuildSkipsSelfUpdateCheck(t *testing.T) {
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{
		Verbose: true,
		Output:  &output,
	})
	updated, err := doSelfUpdate(context.Background(), runner, buildinfo.Info{Version: "1.0.0", DisableSelfUpdate: true}, true, false, "", false)
	if err != nil {
		t.Fatalf("disabled self-update returned an error: %v", err)
	}
	if updated {
		t.Fatal("disabled self-update reported that it updated the binary")
	}
	if got := output.String(); !strings.Contains(got, "Self-update is disabled in this build") {
		t.Fatalf("disabled self-update message missing: %q", got)
	}
}

func TestDisabledSelfUpdateVerboseFlagEmitsDebugOutput(t *testing.T) {
	var output bytes.Buffer
	command := newSelfUpdateCommand(buildinfo.Info{Version: "1.0.0", DisableSelfUpdate: true})
	command.SetErr(&output)
	command.SetArgs([]string{"--verbose"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Debug: Self-update is disabled") {
		t.Fatalf("verbose debug output missing: %q", got)
	}
}
