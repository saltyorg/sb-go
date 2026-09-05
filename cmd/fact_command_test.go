package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestFactEditorCommandContract(t *testing.T) {
	command := newFactCommand()
	if command.Flags().HasFlags() {
		t.Error("fact editor has local flags")
	}
	if err := command.ValidateArgs(nil); err != nil {
		t.Errorf("zero arguments rejected: %v", err)
	}
	for _, args := range [][]string{{"plex"}, {"plex", "default"}} {
		if err := command.ValidateArgs(args); err == nil {
			t.Errorf("legacy arguments accepted: %v", args)
		}
	}
	for _, flag := range []string{"method", "delete-type", "key"} {
		if command.Flags().Lookup(flag) != nil {
			t.Errorf("legacy flag --%s still exists", flag)
		}
	}
}

func TestFactEditorRequiresTerminal(t *testing.T) {
	command := newFactCommand()
	command.SetIn(strings.NewReader(""))
	command.SetOut(new(bytes.Buffer))
	command.SetErr(new(bytes.Buffer))
	command.SetArgs(nil)
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-TTY error = %v", err)
	}
}

func TestFactEditorHelp(t *testing.T) {
	command := newFactCommand()
	var output bytes.Buffer
	command.SetIn(strings.NewReader(""))
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"--method", "--delete-type", "--key", "[role]", "[instance]"} {
		if strings.Contains(output.String(), legacy) {
			t.Errorf("help contains legacy syntax %q", legacy)
		}
	}
}
