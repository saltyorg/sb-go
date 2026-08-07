package cmd

import "testing"

func TestUpdateVenvCommandIsHidden(t *testing.T) {
	if !updateVenvCmd.Hidden {
		t.Fatal("update-venv command must remain hidden")
	}
	if updateVenvCmd.Use != "update-venv" {
		t.Fatalf("updateVenvCmd.Use = %q, want %q", updateVenvCmd.Use, "update-venv")
	}
	if err := updateVenvCmd.Args(updateVenvCmd, []string{"unexpected"}); err == nil {
		t.Fatal("update-venv command unexpectedly accepted a positional argument")
	}
}

func TestUpdateVenvTaskSpecDescribesCheckResult(t *testing.T) {
	spec := updateVenvTaskSpec()
	if spec.Running != "Checking Ansible virtual environment for updates" {
		t.Fatalf("Running = %q", spec.Running)
	}
	if spec.Success != "Ansible virtual environment is ready" {
		t.Fatalf("Success = %q", spec.Success)
	}
}
