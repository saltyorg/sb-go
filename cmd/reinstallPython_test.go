package cmd

import (
	"testing"

	"github.com/saltyorg/sb-go/terminal"
)

func TestReinstallPythonRetainsTaskHierarchy(t *testing.T) {
	if got := reinstallPythonTaskSpec().ChildDisplay; got != terminal.RetainChildTasks {
		t.Fatalf("root child display = %v, want RetainChildTasks", got)
	}
	if got := reinstallPythonVenvTaskSpec().ChildDisplay; got != terminal.RetainChildTasks {
		t.Fatalf("venv child display = %v, want RetainChildTasks", got)
	}
}
