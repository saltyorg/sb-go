package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/buildinfo"
)

func TestNewRootCommandReturnsIndependentTrees(t *testing.T) {
	first := NewRootCommand(Dependencies{})
	second := NewRootCommand(Dependencies{})

	firstList, _, err := first.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	secondList, _, err := second.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstList.Flags().Set("include-mod", "true"); err != nil {
		t.Fatal(err)
	}
	if got := secondList.Flags().Lookup("include-mod").Value.String(); got != "false" {
		t.Fatalf("second root inherited first root flag state: include-mod=%s", got)
	}

	firstInstall, _, err := first.Find([]string{"install"})
	if err != nil {
		t.Fatal(err)
	}
	secondInstall, _, err := second.Find([]string{"install"})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstInstall.Flags().Set("verbose", "4"); err != nil {
		t.Fatal(err)
	}
	if got := secondInstall.Flags().Lookup("verbose").Value.String(); got != "0" {
		t.Fatalf("second root inherited Ansible verbosity: verbose=%s", got)
	}
}

func TestRootUsesInjectedBuildInfoAndWriters(t *testing.T) {
	root := NewRootCommand(Dependencies{BuildInfo: buildinfo.Info{Version: "1.2.3", GitCommit: "deadbeef"}})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"1.2.3", "deadbeef"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("version output %q does not contain injected value %q", output.String(), expected)
		}
	}
}

func TestInstallVerbosityRemainsAnsibleCount(t *testing.T) {
	install := newInstallCommand()
	if err := install.ParseFlags([]string{"-vvvv"}); err != nil {
		t.Fatal(err)
	}
	verbosity, err := install.Flags().GetCount("verbose")
	if err != nil {
		t.Fatal(err)
	}
	if verbosity != 4 {
		t.Fatalf("verbose count = %d, want 4", verbosity)
	}
}
