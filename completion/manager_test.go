package completion

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	m := NewManager()
	m.Paths = Paths{
		BashDir: filepath.Join(root, "bash"), ZshDir: filepath.Join(root, "zsh"),
		BashLegacyDir: filepath.Join(root, "legacy-bash"), ZshLegacyDir: filepath.Join(root, "legacy-zsh"),
	}
	m.Probe = func(_ context.Context, shell Shell) (Detection, error) {
		return Detection{Present: true, Directory: filepath.Join(root, "fish")}, nil
	}
	return m
}

func generated(shell Shell, name string) ([]byte, error) {
	if shell == Zsh {
		return []byte("#compdef " + name + "\n# native fixture\n"), nil
	}
	return []byte("# native fixture for " + name + "\n"), nil
}

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func legacyCompletion(shell Shell, name string, standalone bool) []byte {
	if standalone {
		if shell == Bash {
			return []byte("# bash completion for " + name + " -*- shell-script -*-\n_" + name + "_install_completion() {}\n")
		}
		return []byte("#compdef " + name + "\n\n# " + name + " zsh completion script\n_" + name + "_install_tags() {}\n")
	}
	return []byte("# Custom tag completion for 'install' command with comma-separated support\n_" + name + "_custom_install_tags() {}\n")
}

func TestInstallOnlyDetectedShells(t *testing.T) {
	m := testManager(t)
	m.Probe = func(_ context.Context, shell Shell) (Detection, error) {
		switch shell {
		case Bash:
			return Detection{Present: true, Reason: "bash-completion is not installed"}, nil
		case Zsh:
			return Detection{}, nil
		default:
			return Detection{Present: true, Directory: filepath.Join(filepath.Dir(m.Paths.BashDir), "fish-config", "completions")}, nil
		}
	}
	results, err := m.Install(context.Background(), SupportedShells(), []string{"sb", "another-name", "sb"}, generated)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 || results[0].Status != Skipped || results[1].Status != Skipped {
		t.Fatalf("unexpected results: %+v", results)
	}
	for _, directory := range []string{m.Paths.BashDir, m.Paths.ZshDir, m.Paths.BashLegacyDir, m.Paths.ZshLegacyDir} {
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("absent shell/dependency created %s: %v", directory, err)
		}
	}
	for _, name := range []string{"sb", "another-name"} {
		path := filepath.Join(filepath.Dir(m.Paths.BashDir), "fish-config", "completions", name+".fish")
		data, err := os.ReadFile(path)
		if err != nil || !identifiesCompletion(data, Fish, name) {
			t.Fatalf("Fish alias %s not installed: %v", name, err)
		}
	}
}

func TestMigrationCleansGeneratedActiveAndObsoleteAliases(t *testing.T) {
	for _, shell := range []Shell{Bash, Zsh} {
		t.Run(string(shell), func(t *testing.T) {
			m := testManager(t)
			for _, name := range []string{"sb", "another-name", "retired-name"} {
				writeFixture(t, m.legacyPath(shell, name), legacyCompletion(shell, name, false))
			}
			writeFixture(t, m.legacyPath(shell, "retired-standalone"), legacyCompletion(shell, "retired-standalone", true))
			foreign := m.legacyPath(shell, "another-command")
			foreignContent := []byte("# another application's completion\n")
			writeFixture(t, foreign, foreignContent)
			foreignModern := filepath.Join(m.directory(shell, Detection{}), fileName(shell, "third-party"))
			writeFixture(t, foreignModern, foreignContent)
			results, err := m.Install(context.Background(), []Shell{shell}, []string{"sb", "another-name"}, generated)
			if err != nil {
				t.Fatal(err)
			}
			var removed []string
			for _, result := range results {
				removed = append(removed, result.Removed...)
			}
			for _, name := range []string{"sb", "another-name", "retired-name", "retired-standalone"} {
				old := m.legacyPath(shell, name)
				if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) || !slices.Contains(removed, old) {
					t.Fatalf("old %s was not removed/reported: %v %+v", old, err, results)
				}
			}
			for _, path := range []string{foreign, foreignModern} {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, foreignContent) {
					t.Fatalf("unrelated completion changed: %s %v", path, err)
				}
			}
		})
	}
}

func TestOrphanIdentificationUsesOnlyOwnershipCommentsOrOldWrappers(t *testing.T) {
	tests := []struct {
		name    string
		shell   Shell
		content []byte
		want    bool
	}{
		{"current marker", Bash, []byte("# sb-go completion bash another-name\nchanged body\n"), true},
		{"previous development marker", Bash, []byte("# sb-go managed completion v1 bash another-name invalid-checksum\nchanged body\n"), true},
		{"old hybrid", Bash, legacyCompletion(Bash, "another-name", false), true},
		{"old standalone bash", Bash, legacyCompletion(Bash, "another-name", true), true},
		{"old standalone zsh", Zsh, legacyCompletion(Zsh, "another-name", true), true},
		{"marker for different name", Bash, []byte("# sb-go completion bash different-name\n"), false},
		{"marker for different shell", Bash, []byte("# sb-go completion fish another-name\n"), false},
		{"ordinary bash header", Bash, []byte("# bash completion for another-name -*- shell-script -*-\n"), false},
		{"ordinary zsh registration", Zsh, []byte("#compdef another-name\n_another-name() {}\n"), false},
		{"custom comment alone", Bash, []byte("# Custom tag completion for 'install' command with comma-separated support\n"), false},
		{"custom function alone", Bash, []byte("_another-name_custom_install_tags() {}\n"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := identifiesCompletion(tt.content, tt.shell, "another-name"); got != tt.want {
				t.Fatalf("completion identification = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplacesCurrentNamedFilesAndRetiresLegacyPaths(t *testing.T) {
	for _, shell := range SupportedShells() {
		t.Run(string(shell), func(t *testing.T) {
			m := testManager(t)
			const name = "another-name"
			directory := m.directory(shell, Detection{Directory: filepath.Join(filepath.Dir(m.Paths.BashDir), "fish")})
			path := filepath.Join(directory, fileName(shell, name))
			writeFixture(t, path, []byte("changed old script without an ownership marker\n"))
			var oldPaths []string
			if legacy := m.legacyPath(shell, name); legacy != "" {
				oldPaths = append(oldPaths, legacy)
			}
			if shell == Bash {
				oldPaths = append(oldPaths, filepath.Join(directory, name))
			}
			for _, old := range oldPaths {
				writeFixture(t, old, []byte("changed obsolete named output\n"))
			}
			results, err := m.Install(context.Background(), []Shell{shell}, []string{name}, generated)
			if err != nil || len(results) != 1 || results[0].Status != Installed {
				t.Fatalf("named output was not replaced: %v %+v", err, results)
			}
			data, readErr := os.ReadFile(path)
			body, _ := generated(shell, name)
			if readErr != nil || !bytes.Equal(data, completionContent(shell, name, body)) {
				t.Fatalf("replacement does not contain the generated script: %v", readErr)
			}
			for _, old := range oldPaths {
				if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) || !slices.Contains(results[0].Removed, old) {
					t.Fatalf("old named output not retired and reported: %s %v %+v", old, err, results)
				}
			}
		})
	}
}

func TestFailedGenerationPreservesLegacyAndManagedFiles(t *testing.T) {
	m := testManager(t)
	body, _ := generated(Bash, "sb")
	modern := filepath.Join(m.Paths.BashDir, "sb.bash")
	original := completionContent(Bash, "sb", body)
	writeFixture(t, modern, original)
	legacy := m.legacyPath(Bash, "sb")
	writeFixture(t, legacy, legacyCompletion(Bash, "sb", false))
	writeFixture(t, m.legacyPath(Bash, "sb-old"), legacyCompletion(Bash, "sb-old", false))
	_, err := m.Install(context.Background(), []Shell{Bash}, []string{"sb"}, func(Shell, string) ([]byte, error) {
		return nil, errors.New("generation failed")
	})
	if err == nil {
		t.Fatal("expected generation error")
	}
	data, err := os.ReadFile(modern)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("old modern output changed: %v", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("active legacy recovery output removed: %v", err)
	}
	if _, err := os.Stat(m.legacyPath(Bash, "sb-old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete legacy output retained: %v", err)
	}
}

func TestFailedReplacementKeepsActiveLegacyFile(t *testing.T) {
	for _, shell := range []Shell{Bash, Zsh} {
		t.Run(string(shell), func(t *testing.T) {
			m := testManager(t)
			legacy := m.legacyPath(shell, "sb")
			original := []byte("old active completion\n")
			writeFixture(t, legacy, original)
			modern := filepath.Join(m.directory(shell, Detection{}), fileName(shell, "sb"))
			if err := os.MkdirAll(modern, 0755); err != nil {
				t.Fatal(err)
			}
			results, err := m.Install(context.Background(), []Shell{shell}, []string{"sb"}, generated)
			if err == nil || results[0].Status != Failed {
				t.Fatalf("replacement over a directory succeeded: %v %+v", err, results)
			}
			data, err := os.ReadFile(legacy)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatalf("failed replacement removed the active legacy completion: %v", err)
			}
		})
	}
}

func TestSecureInstallationAndObsoleteManagedCleanup(t *testing.T) {
	m := testManager(t)
	oldUmask := syscall.Umask(0077)
	defer syscall.Umask(oldUmask)
	results, err := m.Install(context.Background(), SupportedShells(), []string{"sb", "sb-old"}, generated)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		info, err := os.Stat(result.Path)
		if err != nil || info.Mode().Perm() != 0644 {
			t.Fatalf("completion not world-readable under restrictive umask: %s %v", result.Path, err)
		}
		parent, err := os.Stat(filepath.Dir(result.Path))
		if err != nil || parent.Mode().Perm() != 0755 {
			t.Fatalf("created directory not world-searchable: %v", err)
		}
	}
	results, err = m.Install(context.Background(), SupportedShells(), []string{"sb"}, generated)
	if err != nil {
		t.Fatal(err)
	}
	for _, shell := range SupportedShells() {
		found := false
		for _, result := range results {
			if result.Shell == shell && result.Name == "sb-old" && result.Status == Cleaned {
				found = true
				if _, err := os.Stat(result.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("obsolete output remains: %v", err)
				}
			}
		}
		if !found {
			t.Fatalf("obsolete %s alias not cleaned: %+v", shell, results)
		}
	}
}

func TestFileSymlinksAreReplacedOrUnlinkedWithoutFollowingTargets(t *testing.T) {
	m := testManager(t)
	target := filepath.Join(filepath.Dir(m.Paths.BashDir), "external-file")
	original := []byte("unrelated target\n")
	writeFixture(t, target, original)
	modern := filepath.Join(m.Paths.BashDir, "sb.bash")
	oldPaths := []string{m.legacyPath(Bash, "sb"), filepath.Join(m.Paths.BashDir, "sb")}
	for _, path := range append([]string{modern}, oldPaths...) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	results, err := m.Install(context.Background(), []Shell{Bash}, []string{"sb"}, generated)
	if err != nil || results[0].Status != Installed {
		t.Fatalf("file symlink replacement failed: %v %+v", err, results)
	}
	info, err := os.Lstat(modern)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("modern path was not replaced with a regular file: %v", err)
	}
	installed, err := os.ReadFile(modern)
	body, _ := generated(Bash, "sb")
	if err != nil || !bytes.Equal(installed, completionContent(Bash, "sb", body)) {
		t.Fatalf("file symlink was not replaced with generated completion: %v", err)
	}
	for _, path := range oldPaths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old file symlink remains: %s %v", path, err)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("replacement changed an unrelated symlink target: %v", err)
	}
}

func TestDetectionFailureDoesNotPreventOtherShells(t *testing.T) {
	m := testManager(t)
	m.Probe = func(_ context.Context, shell Shell) (Detection, error) {
		if shell == Bash {
			return Detection{Present: true}, errors.New("probe timed out")
		}
		return Detection{Present: true}, nil
	}
	results, err := m.Install(context.Background(), []Shell{Bash, Zsh}, []string{"sb"}, generated)
	if err == nil || len(results) != 2 || results[0].Status != Failed || results[1].Status != Installed {
		t.Fatalf("independent engine stopped: %v %+v", err, results)
	}
}

func TestAbsentEngineCleansOnlyObsoleteOutputs(t *testing.T) {
	m := testManager(t)
	m.Probe = func(context.Context, Shell) (Detection, error) { return Detection{}, nil }
	active := m.legacyPath(Bash, "sb")
	obsolete := m.legacyPath(Bash, "sb-old")
	writeFixture(t, active, legacyCompletion(Bash, "sb", false))
	writeFixture(t, obsolete, legacyCompletion(Bash, "sb-old", false))
	results, err := m.Install(context.Background(), []Shell{Bash}, []string{"sb"}, generated)
	if err != nil || len(results) != 2 || results[0].Status != Skipped || results[1].Status != Cleaned {
		t.Fatalf("absent engine cleanup failed: %v %+v", err, results)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active recovery output removed: %v", err)
	}
	if _, err := os.Stat(obsolete); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete output remains: %v", err)
	}
	if _, err := os.Stat(m.Paths.BashDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent engine created autoload directory: %v", err)
	}
}

func TestEditedRetiredOwnedOutputsAreRemoved(t *testing.T) {
	for _, shell := range SupportedShells() {
		t.Run(string(shell), func(t *testing.T) {
			m := testManager(t)
			directory := m.directory(shell, Detection{Directory: filepath.Join(filepath.Dir(m.Paths.BashDir), "fish")})
			path := filepath.Join(directory, fileName(shell, "retired-name"))
			body, _ := generated(shell, "retired-name")
			content := append(completionContent(shell, "retired-name", body), []byte("# later edit\n")...)
			writeFixture(t, path, content)
			oldPaths := []string{path}
			if shell != Fish {
				for _, standalone := range []bool{false, true} {
					name := "retired-hybrid"
					if standalone {
						name = "retired-standalone"
					}
					legacy := m.legacyPath(shell, name)
					oldPaths = append(oldPaths, legacy)
					content := append(legacyCompletion(shell, name, standalone), []byte("# later edit\n")...)
					writeFixture(t, legacy, content)
				}
			}
			results, err := m.Install(context.Background(), []Shell{shell}, []string{"sb"}, generated)
			if err != nil {
				t.Fatalf("retired output cleanup failed: %v %+v", err, results)
			}
			for _, old := range oldPaths {
				if _, err := os.Lstat(old); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("edited retired output remains: %s %v", old, err)
				}
			}
		})
	}
}

func TestSeparateLiveInstallationKeepsItsCompletions(t *testing.T) {
	m := testManager(t)
	bin := filepath.Join(filepath.Dir(m.Paths.BashDir), "usr", "local", "bin")
	for _, name := range []string{"sb", "sb2"} {
		path := filepath.Join(bin, name)
		writeFixture(t, path, []byte("#!/bin/sh\nexit 42\n"))
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	m.NameInUse = func(name string) (bool, error) {
		info, err := os.Stat(filepath.Join(bin, name))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0, nil
	}
	for _, shell := range []Shell{Bash, Zsh} {
		body, _ := generated(shell, "sb2")
		writeFixture(t, filepath.Join(m.directory(shell, Detection{}), fileName(shell, "sb2")), completionContent(shell, "sb2", body))
		writeFixture(t, m.legacyPath(shell, "sb2"), legacyCompletion(shell, "sb2", false))
	}
	results, err := m.Install(context.Background(), []Shell{Bash, Zsh}, []string{"sb"}, generated)
	if err != nil {
		t.Fatal(err)
	}
	preserved := 0
	for _, result := range results {
		if result.Name == "sb2" && result.Status == Skipped && strings.Contains(result.Reason, "live executable") {
			preserved++
			if _, err := os.Stat(result.Path); err != nil {
				t.Fatalf("separate installation completion removed: %v", err)
			}
		}
	}
	if preserved != 4 {
		t.Fatalf("did not preserve/report both shells and both historical/current files: %+v", results)
	}
}

func TestCancellationPreservesLegacyWithoutCreatingDirectories(t *testing.T) {
	m := testManager(t)
	active := m.legacyPath(Bash, "sb")
	obsolete := m.legacyPath(Bash, "sb-old")
	writeFixture(t, active, legacyCompletion(Bash, "sb", false))
	writeFixture(t, obsolete, legacyCompletion(Bash, "sb-old", false))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := m.Install(ctx, []Shell{Bash}, []string{"sb"}, func(shell Shell, name string) ([]byte, error) {
		cancel()
		return generated(shell, name)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not returned: %v", err)
	}
	for _, path := range []string{active, obsolete} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("canceled migration removed recovery output: %v", err)
		}
	}
	if _, err := os.Stat(m.Paths.BashDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled generator created autoload directory: %v", err)
	}
}
