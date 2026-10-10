//go:build linux

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/saltyorg/sb-go/completion"
	"github.com/spf13/cobra"
)

const completionShellHelperEnv = "SB_GO_COMPLETION_SHELL_HELPER"

var completionShellTags = []string{"mod-plex", "plex", "radarr", "sandbox-plex", "sonarr"}

// Run the actual Cobra command tree without application startup, sudo, or Ansible.
// The helper accepts only completion requests and replaces the tag loader.
func TestCompletionShellHelper(t *testing.T) {
	if os.Getenv(completionShellHelperEnv) != "1" {
		t.Skip("completion subprocess helper")
	}
	i := slices.Index(os.Args, "--")
	if i < 0 || i+1 >= len(os.Args) || (os.Args[i+1] != "__complete" && os.Args[i+1] != "__completeNoDesc") {
		t.Fatal("fixture received a non-completion request")
	}
	root := completionShellRoot("sb-completion-fixture")
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(os.Args[i+1:])
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Cobra's protocol must not be followed by the test runner's PASS message.
	os.Exit(0)
}

func completionShellRoot(name string) *cobra.Command {
	root := NewRootCommand(Dependencies{})
	root.Use = name
	for _, command := range root.Commands() {
		if command.Name() == "install" {
			root.RemoveCommand(command)
			break
		}
	}
	root.AddCommand(newInstallCommandWithTagLoader(func() ([]string, error) {
		return slices.Clone(completionShellTags), nil
	}))
	return root
}

type completionShellFixture struct {
	dir string
	env []string
}

func newCompletionShellFixture(t *testing.T, shell string) completionShellFixture {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	for _, path := range []string{bin, filepath.Join(dir, "home"), filepath.Join(dir, "config"), filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "work")} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launcher := "#!/bin/sh\nexec " + completionShellQuote(testBinary) + " -test.run='^TestCompletionShellHelper$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "sb-completion-fixture"), []byte(launcher), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sb-completion-fixture", filepath.Join(bin, "sb-completion-alias")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sb-completion-fixture", "sb-completion-alias"} {
		root := completionShellRoot(name)
		var relative string
		switch shell {
		case "bash":
			relative = filepath.Join("share", "bash-completion", "completions", name+".bash")
		case "zsh":
			relative = filepath.Join("zsh", "_"+name)
		case "fish":
			relative = filepath.Join("fish", name+".fish")
		default:
			t.Fatalf("unknown fixture shell %q", shell)
		}
		script, err := generateCompletion(root, completion.Shell(shell), name)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, script, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return completionShellFixture{dir: dir, env: []string{
		"HOME=" + filepath.Join(dir, "home"),
		"ZDOTDIR=" + filepath.Join(dir, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "config"),
		"XDG_DATA_HOME=" + filepath.Join(dir, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
		"XDG_DATA_DIRS=" + filepath.Join(dir, "share"),
		"BASH_COMPLETION_COMPAT_DIR=" + filepath.Join(dir, "empty-compat"),
		"PATH=" + bin + ":/usr/bin:/bin",
		"TERM=xterm-256color", "LC_ALL=C.UTF-8", completionShellHelperEnv + "=1",
	}}
}

func completionShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

type completionShellCase struct {
	name  string
	input string
	words []string
	want  []string
}

func completionShellCases(command string) []completionShellCase {
	return []completionShellCase{
		{"command", command + " ins", []string{command, "ins"}, []string{"install"}},
		{"flag", command + " install --no-c", []string{command, "install", "--no-c"}, []string{"--no-cache"}},
		{"flag after noun", command + " install plex --no-c", []string{command, "install", "plex", "--no-c"}, []string{"--no-cache"}},
		{"tag", command + " install so", []string{command, "install", "so"}, []string{"sonarr"}},
		{"empty current word", command + " install ", []string{command, "install", ""}, slices.Clone(completionShellTags)},
		{"exclude previous argument", command + " install plex p", []string{command, "install", "plex", "p"}, nil},
		{"comma prefix", command + " install plex,so", []string{command, "install", "plex,so"}, []string{"plex,sonarr"}},
		{"multiple commas", command + " install plex,radarr,so", []string{command, "install", "plex,radarr,so"}, []string{"plex,radarr,sonarr"}},
		{"empty comma suffix", command + " install plex,", []string{command, "install", "plex,"}, []string{"plex,mod-plex", "plex,radarr", "plex,sandbox-plex", "plex,sonarr"}},
		{"comma followed by separate word", command + " install plex, so", []string{command, "install", "plex,", "so"}, []string{"sonarr"}},
		{"quoted comma space", command + " install \"plex, so", []string{command, "install", "\"plex, so"}, []string{"plex, sonarr"}},
		{"single quoted comma space", command + " install 'plex, so", []string{command, "install", "'plex, so"}, []string{"plex, sonarr"}},
		{"closed quoted comma", command + " install \"plex, so\"", []string{command, "install", "\"plex, so\""}, []string{"plex, sonarr"}},
		{"quoted previous noun", command + " install \"plex\" p", []string{command, "install", "\"plex\"", "p"}, nil},
		{"extra-vars value", command + " install --extra-vars p", []string{command, "install", "--extra-vars", "p"}, nil},
		{"skip-tags value", command + " install --skip-tags p", []string{command, "install", "--skip-tags", "p"}, nil},
		{"quoted extra-vars before noun", command + " install --extra-vars \"x=y z\" so", []string{command, "install", "--extra-vars", "\"x=y z\"", "so"}, []string{"sonarr"}},
	}
}

func TestNativeBashCompletion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	const framework = "/usr/share/bash-completion/bash_completion"
	if _, err := os.Stat(framework); err != nil {
		t.Skip("system bash-completion framework is not installed")
	}
	fixture := newCompletionShellFixture(t, "bash")
	for _, name := range []string{"sb-completion-fixture", "sb-completion-alias"} {
		for _, tc := range completionShellCases(name) {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				want := slices.Clone(tc.want)
				if tc.name == "quoted comma space" || tc.name == "single quoted comma space" {
					// Cobra 1.10.2 uses eval and cannot complete an unfinished
					// quote. Preserve this known adapter limit explicitly.
					want = nil
				} else if len(want) == 1 {
					// Cobra quotes a single candidate for insertion by Readline.
					want[0] = strings.ReplaceAll(want[0], ",", "\\,")
					want[0] = strings.ReplaceAll(want[0], " ", "\\ ")
				}
				var script strings.Builder
				fmt.Fprintf(&script, "source %s\ncomplete -p %s >/dev/null 2>&1 && exit 71\n_completion_loader %s\ncomplete -p %s >/dev/null || exit 72\n", completionShellQuote(framework), completionShellQuote(name), completionShellQuote(name), completionShellQuote(name))
				for range 2 {
					script.WriteString("COMP_WORDS=(")
					for _, word := range tc.words {
						script.WriteString(completionShellQuote(word) + " ")
					}
					fmt.Fprintf(&script, ")\nCOMP_CWORD=%d\nCOMP_LINE=%s\nCOMP_POINT=${#COMP_LINE}\nCOMPREPLY=()\n__start_%s\nprintf 'BEGIN\\n'\nfor value in \"${COMPREPLY[@]}\"; do printf '%%s\\n' \"$value\"; done\nprintf 'END\\n'\n", len(tc.words)-1, completionShellQuote(tc.input), name)
				}
				output := fixture.run(t, bash, "--noprofile", "--norc", "-c", script.String())
				assertCompletionShellResults(t, output, want, 2)
			})
		}
	}
}

func TestNativeFishCompletion(t *testing.T) {
	fish := completionFishRuntime(t)
	fixture := newCompletionShellFixture(t, "fish")
	for _, name := range []string{"sb-completion-fixture", "sb-completion-alias"} {
		for _, tc := range completionShellCases(name) {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				want := slices.Clone(tc.want)
				switch tc.name {
				case "quoted comma space", "single quoted comma space":
					// Cobra passes the current opening quote through as literal
					// data. The callback suppresses that unsafe candidate.
					want = nil
				case "closed quoted comma":
					// Fish does not complete inside a quote once the cursor has
					// moved past its closing character.
					want = nil
				}
				// --no-config suppresses user startup. Set the autoload search path
				// explicitly, since Fish leaves it uninitialized in this mode.
				var script strings.Builder
				fmt.Fprintf(&script, "set -g fish_complete_path %s\n", completionShellQuote(filepath.Join(fixture.dir, "fish")))
				for range 2 {
					fmt.Fprintf(&script, "printf 'BEGIN\\n'; complete -C %s; printf 'END\\n'\n", completionShellQuote(tc.input))
				}
				output := fixture.run(t, fish, "--no-config", "-c", script.String())
				assertCompletionShellResults(t, output, want, 2)
			})
		}
	}
}

func completionFishRuntime(t *testing.T) string {
	t.Helper()
	if fish := os.Getenv("SB_COMPLETION_FISH"); fish != "" {
		return fish
	}
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("Fish is not installed; SB_COMPLETION_FISH can select an isolated runtime")
	}
	return fish
}

func TestNativeFishQuotedEditorCompletion(t *testing.T) {
	fish := completionFishRuntime(t)
	fixture := newCompletionShellFixture(t, "fish")
	for _, name := range []string{"sb-completion-fixture", "sb-completion-alias"} {
		t.Run(name, func(t *testing.T) {
			log := filepath.Join(fixture.dir, name+"-fish.log")
			initPath := filepath.Join(fixture.dir, name+"-fish-init.fish")
			init := fmt.Sprintf(`set -g fish_complete_path %s
set -g sb_completion_capture 0
function fish_prompt
  printf 'SB_READY_%%s> ' $sb_completion_capture
end
function capture_completion
  commandline >> %s
  commandline --replace ''
  commandline --cursor 0
  set -g sb_completion_capture (math $sb_completion_capture + 1)
  commandline -f repaint
end
bind \t complete
bind \cx capture_completion
`, completionShellQuote(filepath.Join(fixture.dir, "fish")), completionShellQuote(log))
			if err := os.WriteFile(initPath, []byte(init), 0644); err != nil {
				t.Fatal(err)
			}
			shell := startCompletionShellPTY(t, fixture, fish, "--no-config", "-i")
			shell.send("source "+completionShellQuote(initPath)+"\r", "SB_READY_0> ")
			sequence := 0
			for _, quote := range []string{"\"", "'"} {
				for range 2 {
					input := name + " install " + quote + "plex, so"
					sequence++
					shell.send(input+"\t\x18", fmt.Sprintf("SB_READY_%d> ", sequence))
					data, err := os.ReadFile(log)
					if err != nil {
						t.Fatal(err)
					}
					lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
					// Preserve an unfinished quote. A candidate retaining its
					// opening quote would insert a literal quote into the tag.
					if got := lines[len(lines)-1]; got != input {
						t.Errorf("Fish Tab on %q produced %q, want %q", input, got, input)
					}
				}
			}
			shell.send("exit\r", "")
			shell.finish()
		})
	}
}

func (fixture completionShellFixture) run(t *testing.T, binary string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = filepath.Join(fixture.dir, "work")
	command.Env = fixture.env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", filepath.Base(binary), err, output)
	}
	return string(output)
}

func assertCompletionShellResults(t *testing.T, output string, want []string, rounds int) {
	t.Helper()
	var results [][]string
	var current []string
	for line := range strings.SplitSeq(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		switch line {
		case "BEGIN":
			current = nil
		case "END":
			results = append(results, current)
		default:
			if line != "" {
				candidate, _, _ := strings.Cut(line, "\t")
				current = append(current, candidate)
			}
		}
	}
	if len(results) != rounds {
		t.Fatalf("completion produced %d rounds, want %d: %q", len(results), rounds, output)
	}
	want = slices.Clone(want)
	slices.Sort(want)
	for round, got := range results {
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("completion round %d = %q, want %q\n%s", round+1, got, want, output)
		}
	}
}

func TestNativeZshCompletion(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("Zsh is not installed")
	}
	fixture := newCompletionShellFixture(t, "zsh")
	for _, name := range []string{"sb-completion-fixture", "sb-completion-alias"} {
		t.Run(name, func(t *testing.T) {
			log := filepath.Join(fixture.dir, name+".log")
			initPath := filepath.Join(fixture.dir, name+".zsh")
			init := fmt.Sprintf(`fpath=(%s $fpath)
autoload -Uz compinit
compinit -D
integer sb_completion_capture=0
PROMPT='SB_READY_0> '
bindkey -e
capture_completion() {
  print -r -- "$BUFFER" >> %s
  BUFFER=''
  CURSOR=0
  (( ++sb_completion_capture ))
  PROMPT="SB_READY_${sb_completion_capture}> "
  zle reset-prompt
}
zle -N capture_completion
bindkey '^Xc' capture_completion
`, completionShellQuote(filepath.Join(fixture.dir, "zsh")), completionShellQuote(log))
			if err := os.WriteFile(initPath, []byte(init), 0644); err != nil {
				t.Fatal(err)
			}
			shell := startCompletionShellPTY(t, fixture, zsh, "-dfi")
			shell.send("source "+completionShellQuote(initPath)+"\r", "SB_READY_0> ")
			cases := []struct{ input, want string }{
				{name + " ins", name + " install "},
				{name + " install ple", name + " install plex "},
				{name + " install ple", name + " install plex "},
				{name + " install plex --no-c", name + " install plex --no-cache "},
				{name + " install plex,radarr,so", name + " install plex,radarr,sonarr "},
				{name + " install mod-plex plex radarr sandbox-plex ", name + " install mod-plex plex radarr sandbox-plex sonarr "},
				{name + " install mod-plex plex sandbox-plex radarr,", name + " install mod-plex plex sandbox-plex radarr,sonarr "},
				{name + " install plex p", name + " install plex p"},
				{name + " install --extra-vars p", name + " install --extra-vars p"},
				{name + " install --skip-tags p", name + " install --skip-tags p"},
				{name + " install plex, so", name + " install plex, sonarr "},
				// Native Cobra cannot complete unfinished quotes. Tab must
				// preserve the input instead of deleting or rewriting it.
				{name + " install \"plex, so", name + " install \"plex, so"},
				{name + " install 'plex, so", name + " install 'plex, so"},
				// Zsh's native adapter appends its completion space inside an
				// already closed quote. install trims tag whitespace.
				{name + " install \"plex, so\"", name + " install \"plex, sonarr \""},
				{name + " install \"plex\" p", name + " install \"plex\" p"},
				{name + " install --extra-vars \"x=y z\" so", name + " install --extra-vars \"x=y z\" sonarr "},
			}
			for sequence, tc := range cases {
				shell.send(tc.input+"\t\x18c", fmt.Sprintf("SB_READY_%d> ", sequence+1))
				data, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
				if got := lines[len(lines)-1]; got != tc.want {
					t.Errorf("Tab on %q produced %q, want %q", tc.input, got, tc.want)
				}
			}
			shell.send("exit\r", "")
			shell.finish()
		})
	}
}

type completionShellPTY struct {
	t      *testing.T
	master *os.File
	chunks chan string
	done   chan error
	output string
	name   string
}

func startCompletionShellPTY(t *testing.T, fixture completionShellFixture, binary string, args ...string) *completionShellPTY {
	t.Helper()
	// Race-instrumented completion subprocesses can take substantially longer
	// to start. The protocol uses explicit prompts rather than timing delays.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = filepath.Join(fixture.dir, "work")
	command.Env = fixture.env
	master, err := pty.StartWithSize(command, &pty.Winsize{Rows: 30, Cols: 160})
	if err != nil {
		t.Fatal(err)
	}
	shell := &completionShellPTY{t: t, master: master, chunks: make(chan string, 128), done: make(chan error, 1), name: filepath.Base(binary)}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = master.Close()
	})
	go func() { shell.done <- command.Wait() }()
	go func() {
		defer close(shell.chunks)
		buf := make([]byte, 8192)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				select {
				case shell.chunks <- string(buf[:n]):
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return shell
}

func (shell *completionShellPTY) send(keys, marker string) {
	shell.t.Helper()
	start := len(shell.output)
	if _, err := io.WriteString(shell.master, keys); err != nil {
		shell.t.Fatal(err)
	}
	if marker == "" {
		return
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for !strings.Contains(shell.output[start:], marker) {
		select {
		case chunk, ok := <-shell.chunks:
			if !ok {
				shell.t.Fatalf("%s PTY closed waiting for %q: %q", shell.name, marker, shell.output[start:])
			}
			shell.output += chunk
		case <-timer.C:
			shell.t.Fatalf("%s PTY timed out waiting for %q: %q", shell.name, marker, shell.output[start:])
		}
	}
}

func (shell *completionShellPTY) finish() {
	shell.t.Helper()
	select {
	case err := <-shell.done:
		if err != nil {
			shell.t.Fatalf("%s PTY failed: %v\n%s", shell.name, err, shell.output)
		}
	case <-time.After(10 * time.Second):
		shell.t.Fatalf("%s PTY did not exit", shell.name)
	}
}
