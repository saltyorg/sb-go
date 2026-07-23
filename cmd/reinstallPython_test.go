package cmd

import (
	"testing"

	"github.com/saltyorg/sb-go/internal/spinners"
)

func TestReinstallPythonRetainsTaskHierarchy(t *testing.T) {
	if got := reinstallPythonTaskSpec().ChildDisplay; got != spinners.RetainChildTasks {
		t.Fatalf("root child display = %v, want RetainChildTasks", got)
	}
	if got := reinstallPythonVenvTaskSpec().ChildDisplay; got != spinners.RetainChildTasks {
		t.Fatalf("venv child display = %v, want RetainChildTasks", got)
	}
}
