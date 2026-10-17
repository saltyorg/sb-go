package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/host"
)

func TestEditorCommandUsesEditor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yml")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "vim.basic")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	c, err := editorCommand(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Path != editor {
		t.Fatalf("editor = %q, want %q", c.Path, editor)
	}
	if len(c.Args) != 2 || c.Args[1] != path {
		t.Fatalf("editor args = %q, want [%s %s]", c.Args, editor, path)
	}
	output, err := c.Output()
	if err != nil || string(output) != path {
		t.Fatalf("editor output = %q, %v, want %q", output, err, path)
	}
}

func TestEditorSurvivesRootRelaunch(t *testing.T) {
	dir := t.TempDir()
	// Model sudo's environment filtering without changing users or sudo policy.
	sudo := `#!/bin/sh
if [ "$1" = "--preserve-env=EDITOR" ]; then
    shift
else
    unset EDITOR
fi
exec "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nano"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.yml"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	editor := filepath.Join(dir, "vim.basic")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	t.Setenv("SB_TEST_EDITOR_RELAUNCH", dir)
	originalArgs := os.Args
	os.Args = []string{os.Args[0], "-test.run=^TestEditorRelaunchHelper$"}
	t.Cleanup(func() { os.Args = originalArgs })
	if code, err := host.RelaunchAsRoot(); err != nil || code != 0 {
		t.Fatalf("relaunch = %d, %v", code, err)
	}
	selected, err := os.ReadFile(filepath.Join(dir, "selected-editor"))
	if err != nil {
		t.Fatal(err)
	}
	want := editor + "\n" + filepath.Join(dir, "settings.yml")
	if string(selected) != want {
		t.Fatalf("editor after sudo = %q, want %q", selected, want)
	}
}

func TestEditorRelaunchHelper(t *testing.T) {
	dir := os.Getenv("SB_TEST_EDITOR_RELAUNCH")
	if dir == "" {
		return
	}
	c, err := editorCommand(filepath.Join(dir, "settings.yml"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := c.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selected-editor"), []byte(strings.Join([]string{c.Path, string(output)}, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
}
