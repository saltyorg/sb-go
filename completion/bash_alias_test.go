package completion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBashAliasesLoadNativeBundle(t *testing.T) {
	for _, tc := range []struct {
		name     string
		names    []string
		installs [][]string
		separate bool
	}{
		{"ordinary aliases", []string{"sb", "sb2"}, [][]string{{"sb", "sb2"}, {"sb", "sb2"}}, false},
		{"suffix alias", []string{"sb", "sb.bash", "sb2"}, [][]string{{"sb", "sb.bash", "sb2"}, {"sb", "sb.bash", "sb2"}}, false},
		{"separate regular builds", []string{"sb", "sb.bash"}, [][]string{{"sb.bash"}, {"sb"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			root := filepath.Dir(m.Paths.BashDir)
			m.Paths.BashDir = filepath.Join(root, "share", "bash-completion", "completions")
			bin := filepath.Join(root, "bin")
			binary := []byte("#!/bin/sh\nprintf '%s %s\\n' \"${0##*/}\" \"$*\" >> \"$CALL_LOG\"\nprintf 'install\\n:4\\n'\n")
			writeFixture(t, filepath.Join(bin, "sb"), binary)
			if err := os.Chmod(filepath.Join(bin, "sb"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range tc.names[1:] {
				path := filepath.Join(bin, name)
				if tc.separate {
					writeFixture(t, path, binary)
					if err := os.Chmod(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink("sb", path); err != nil {
					t.Fatal(err)
				}
			}
			m.NameInUse = func(name string) (bool, error) {
				_, err := os.Stat(filepath.Join(bin, name))
				if errors.Is(err, os.ErrNotExist) {
					return false, nil
				}
				return err == nil, err
			}
			native := func(_ Shell, name string) ([]byte, error) {
				var script bytes.Buffer
				err := (&cobra.Command{Use: name}).GenBashCompletionV2(&script, true)
				return script.Bytes(), err
			}
			if tc.separate {
				if _, err := m.Install(context.Background(), []Shell{Bash}, []string{"sb"}, native); err != nil {
					t.Fatal(err)
				}
			}
			calls := make(map[string]int)
			generate := func(shell Shell, name string) ([]byte, error) {
				calls[name]++
				return native(shell, name)
			}
			for attempt, currentNames := range tc.installs {
				results, err := m.Install(context.Background(), []Shell{Bash}, currentNames, generate)
				if err != nil || len(results) != len(tc.names) {
					t.Fatalf("installation %d failed: %v %+v", attempt, err, results)
				}
				for _, result := range results {
					if result.Status != Installed {
						t.Fatalf("alias installation failed: %+v", result)
					}
					if _, err := os.Stat(result.Path); err != nil {
						t.Fatalf("installed completion was removed: %s %v", result.Path, err)
					}
				}
				for _, first := range tc.names {
					t.Run(fmt.Sprintf("install %d/load %s first", attempt, first), func(t *testing.T) {
						assertNativeBashAliasLoading(t, root, bin, first, tc.names)
					})
				}
			}
			for _, name := range tc.names {
				if calls[name] != 2 {
					t.Fatalf("rendered %s %d times; expected once per installation", name, calls[name])
				}
			}
		})
	}
}

func TestBashAliasCollisionPreservesUnrelatedLiveCompletion(t *testing.T) {
	m := testManager(t)
	bin := filepath.Join(filepath.Dir(m.Paths.BashDir), "bin")
	writeFixture(t, filepath.Join(bin, "sb"), []byte("#!/bin/sh\nexit 0\n"))
	if err := os.Chmod(filepath.Join(bin, "sb"), 0755); err != nil {
		t.Fatal(err)
	}
	m.NameInUse = func(name string) (bool, error) {
		_, err := os.Stat(filepath.Join(bin, name))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return err == nil, err
	}
	before := map[string][]byte{
		filepath.Join(m.Paths.BashDir, "sb.bash"):      []byte("# another application's completion\n"),
		filepath.Join(m.Paths.BashDir, "sb.bash.bash"): completionContent(Bash, "sb.bash", []byte("# previous output\n")),
		m.legacyPath(Bash, "sb.bash"):                  legacyCompletion(Bash, "sb.bash", false),
	}
	for path, data := range before {
		writeFixture(t, path, data)
	}
	results, err := m.Install(context.Background(), SupportedShells(), []string{"sb.bash"}, func(shell Shell, name string) ([]byte, error) {
		if shell == Bash {
			t.Fatal("collision detection must precede Bash generation")
		}
		return generated(shell, name)
	})
	if err == nil || !strings.Contains(err.Error(), "filename collision") {
		t.Fatalf("unrelated filename collision was not reported: %v %+v", err, results)
	}
	for _, result := range results {
		if result.Shell == Bash && result.Status != Failed || result.Shell != Bash && result.Status != Installed {
			t.Fatalf("Bash collision blocked an independent shell: %+v", results)
		}
	}
	for path, original := range before {
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, original) {
			t.Fatalf("collision changed an existing output: %s %v", path, err)
		}
	}
}

func assertNativeBashAliasLoading(t *testing.T, root, bin, first string, names []string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Bash is not installed")
	}
	framework := os.Getenv("SB_COMPLETION_BASH_FRAMEWORK")
	if framework == "" {
		framework = "/usr/share/bash-completion/bash_completion"
	}
	if _, err := os.Stat(framework); err != nil {
		t.Skipf("bash-completion framework is unavailable: %v", err)
	}
	log := filepath.Join(t.TempDir(), "calls")
	var commands, wantCalls strings.Builder
	fmt.Fprintf(&commands, "source \"$1\"\n_completion_loader '%s'\n", first)
	for _, name := range names {
		fmt.Fprintf(&commands, "complete -p '%s' >/dev/null || exit 42\nCOMP_WORDS=('%s' ins)\nCOMP_CWORD=1\nCOMP_LINE='%s ins'\nCOMP_POINT=${#COMP_LINE}\n__start_%s\n[[ ${COMPREPLY[*]} == install ]] || exit 43\n", name, name, name, name)
		fmt.Fprintf(&wantCalls, "%s __complete ins\n", name)
	}
	process := exec.CommandContext(t.Context(), bash, "--noprofile", "--norc", "-c", commands.String(), "completion-test", framework)
	process.Env = []string{
		"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root, "LC_ALL=C",
		"XDG_DATA_DIRS=" + filepath.Join(root, "share"),
		"BASH_COMPLETION_COMPAT_DIR=" + filepath.Join(root, "empty"), "CALL_LOG=" + log,
	}
	output, err := process.CombinedOutput()
	if err != nil {
		t.Fatalf("native alias loading or completion failed: %v\n%s", err, output)
	}
	calls, err := os.ReadFile(log)
	if err != nil || string(calls) != wantCalls.String() {
		t.Fatalf("completion invoked the wrong aliases: got %q, want %q, error %v", calls, wantCalls.String(), err)
	}
}

func TestBashBundleGenerationFailureKeepsAllActiveOutputs(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "render error", true: "empty script"}[empty], func(t *testing.T) {
			m := testManager(t)
			names := []string{"sb", "sb2"}
			if _, err := m.Install(context.Background(), []Shell{Bash}, names, generated); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			for _, name := range names {
				legacy := m.legacyPath(Bash, name)
				writeFixture(t, legacy, legacyCompletion(Bash, name, false))
				for _, path := range []string{legacy, filepath.Join(m.Paths.BashDir, fileName(Bash, name))} {
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					before[path] = data
				}
			}
			results, err := m.Install(context.Background(), SupportedShells(), names, func(shell Shell, name string) ([]byte, error) {
				if shell == Bash && name == "sb2" {
					if empty {
						return nil, nil
					}
					return nil, errors.New("render failed")
				}
				return []byte("# replacement\n"), nil
			})
			if err == nil {
				t.Fatal("incomplete Bash bundle was accepted")
			}
			installed := make(map[Shell]int)
			for _, result := range results {
				if result.Status == Installed {
					installed[result.Shell]++
				}
			}
			if installed[Bash] != 0 || installed[Zsh] != 2 || installed[Fish] != 2 {
				t.Fatalf("bundle failure changed Bash or blocked independent shells: %+v", results)
			}
			for path, original := range before {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, original) {
					t.Fatalf("failed bundle generation changed an active output: %s %v", path, err)
				}
			}
		})
	}
}
