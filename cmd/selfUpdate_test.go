package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/internal/runtime"
	"github.com/saltyorg/sb-go/internal/spinners"
)

func TestDisabledBuildSkipsSelfUpdateCheck(t *testing.T) {
	previous := runtime.DisableSelfUpdate
	runtime.DisableSelfUpdate = "true"
	t.Cleanup(func() {
		runtime.DisableSelfUpdate = previous
	})

	var output bytes.Buffer
	runner := spinners.NewRunner(spinners.RunnerOptions{
		Verbose: true,
		Output:  &output,
	})
	updated, err := doSelfUpdate(context.Background(), runner, true, false, "", false)
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
