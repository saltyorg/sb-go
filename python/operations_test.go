package python

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestManagedUVOperationsUseTheRuntimeBoundary(t *testing.T) {
	managedRoot := t.TempDir()
	pythonInstallDir := filepath.Join(managedRoot, "python")
	foundPythonPath := filepath.Join(pythonInstallDir, "cpython", "bin", "python3.12")
	pythonPath := filepath.Join(pythonInstallDir, "bin", "python3.12")
	venvPath := filepath.Join(managedRoot, "venv")
	venvPythonPath := filepath.Join(venvPath, "bin", "python3")
	requirementsPath := filepath.Join(managedRoot, "requirements.txt")

	tests := []struct {
		name     string
		wantArgs []string
		run      func(*managedRuntime) error
	}{
		{
			name: "Python install",
			wantArgs: []string{
				"--no-config", "python", "install", "--managed-python", "--no-bin", "--install-dir", pythonInstallDir,
				"--reinstall", "--no-cache", "3.12.13",
			},
			run: func(runtime *managedRuntime) error {
				return installPythonAtWithRuntime(t.Context(), runtime, "3.12.13", pythonInstallDir, true, true, false)
			},
		},
		{
			name:     "Python find",
			wantArgs: []string{"--no-config", "python", "find", "--managed-python", "--no-project", "--no-python-downloads", "3.12.13"},
			run: func(runtime *managedRuntime) error {
				got, err := findPythonAtWithRuntime(t.Context(), runtime, "3.12.13", pythonInstallDir)
				if err == nil && got != foundPythonPath {
					t.Errorf("found Python = %q", got)
				}
				return err
			},
		},
		{
			name:     "venv create",
			wantArgs: []string{"--no-config", "venv", "--python", pythonPath, "--no-project", "--no-python-downloads", venvPath},
			run: func(runtime *managedRuntime) error {
				return createVenvWithPythonRuntime(t.Context(), runtime, venvPath, pythonPath, false)
			},
		},
		{
			name:     "requirements sync",
			wantArgs: []string{"--no-config", "pip", "sync", "--no-progress", "--python", venvPythonPath, "--require-hashes", "--no-cache", requirementsPath},
			run: func(runtime *managedRuntime) error {
				var stdout bytes.Buffer
				var stderr bytes.Buffer
				err := syncRequirementsWithRuntime(t.Context(), runtime, venvPythonPath, requirementsPath, SyncRequirementsOptions{
					NoCache: true,
					Verbose: true,
					Stdout:  &stdout,
					Stderr:  &stderr,
				})
				if err == nil && (stdout.String() != "sync stdout\n" || stderr.String() != "sync stderr\n") {
					t.Errorf("sync output = stdout %q stderr %q", stdout.String(), stderr.String())
				}
				return err
			},
		},
		{
			name:     "package check",
			wantArgs: []string{"--no-config", "pip", "check", "--python", venvPythonPath},
			run: func(runtime *managedRuntime) error {
				return checkPackagesWithRuntime(t.Context(), runtime, venvPythonPath)
			},
		},
		{
			name:     "Python list",
			wantArgs: []string{"--no-config", "python", "list", "--only-installed"},
			run: func(runtime *managedRuntime) error {
				got, err := listInstalledPythonsWithRuntime(t.Context(), runtime, pythonInstallDir)
				if err == nil && !slices.Equal(got, []string{"cpython-3.12.13-linux-x86_64-gnu", "cpython-3.13.7-linux-x86_64-gnu"}) {
					t.Errorf("listed Pythons = %q", got)
				}
				return err
			},
		},
		{
			name:     "Python uninstall",
			wantArgs: []string{"--no-config", "python", "uninstall", "--install-dir", pythonInstallDir, "3.12.13"},
			run: func(runtime *managedRuntime) error {
				return uninstallPythonWithRuntime(t.Context(), runtime, "3.12.13", pythonInstallDir, false)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recordDir := t.TempDir()
			t.Setenv("SB_TEST_RECORD", recordDir)
			t.Setenv("SB_TEST_FOUND_PYTHON", foundPythonPath)
			uvPath := writeRuntimeExecutable(t, `
printf '%s\000' "$@" > "$SB_TEST_RECORD/args"
case "$2 $3" in
  'python find') printf '%s\n' "$SB_TEST_FOUND_PYTHON" ;;
  'python list') printf '%s\n' 'cpython-3.12.13-linux-x86_64-gnu' 'cpython-3.13.7-linux-x86_64-gnu' ;;
esac
if [ "$2 $3" = "pip sync" ]; then
  printf 'sync stdout\n'
  printf 'sync stderr\n' >&2
fi
`)
			runtime := newManagedRuntime(runtimeSettings{
				uvPath:        uvPath,
				cacheDir:      filepath.Join(t.TempDir(), "cache"),
				scratchParent: t.TempDir(),
			})
			if err := test.run(runtime); err != nil {
				t.Fatal(err)
			}
			if got := readRuntimeArgs(t, recordDir); !slices.Equal(got, test.wantArgs) {
				t.Fatalf("args = %q, want %q", got, test.wantArgs)
			}
		})
	}
}

func TestManagedVersionAndPythonHealthProbesUseIsolationFlags(t *testing.T) {
	t.Run("uv and uvx versions", func(t *testing.T) {
		for _, name := range []string{"uv", "uvx"} {
			t.Run(name, func(t *testing.T) {
				recordDir := t.TempDir()
				t.Setenv("SB_TEST_RECORD", recordDir)
				path := writeRuntimeExecutable(t, "printf '%s\\000' \"$@\" > \"$SB_TEST_RECORD/args\"\nprintf '"+name+" 0.12.10\\n'\n")
				runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "missing-cache"), scratchParent: t.TempDir()})
				got, err := binaryVersionWithRuntime(t.Context(), runtime, path, name)
				if err != nil {
					t.Fatal(err)
				}
				if got != "0.12.10" {
					t.Fatalf("version = %q, want 0.12.10", got)
				}
				if gotArgs, want := readRuntimeArgs(t, recordDir), []string{"--no-config", "--version"}; !slices.Equal(gotArgs, want) {
					t.Fatalf("args = %q, want %q", gotArgs, want)
				}
				if _, statErr := os.Stat(runtime.cacheDir); !os.IsNotExist(statErr) {
					t.Fatalf("version probe created cache %s: %v", runtime.cacheDir, statErr)
				}
			})
		}
	})

	t.Run("direct Python", func(t *testing.T) {
		recordDir := t.TempDir()
		t.Setenv("SB_TEST_RECORD", recordDir)
		path := writeRuntimeExecutable(t, `
printf '%s\000' "$@" >> "$SB_TEST_RECORD/args"
printf '\n' >> "$SB_TEST_RECORD/args"
if [ "$2" = "--version" ]; then printf 'Python 3.12.13\n'; fi
`)
		runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
		if err := validatePythonWithRuntime(t.Context(), runtime, path, "3.12.13"); err != nil {
			t.Fatal(err)
		}
		data := readRuntimeFile(t, recordDir, "args")
		if !strings.Contains(data, "-I\x00--version\x00") || !strings.Contains(data, "-I\x00-c\x00import encodings, sys; sys.exit(0)\x00") {
			t.Fatalf("Python health args did not isolate both probes: %q", data)
		}
	})
}

func TestManagedNonVerboseUVErrorRetainsStderr(t *testing.T) {
	uvPath := writeRuntimeExecutable(t, "printf 'download corrupt\\n' >&2\nexit 19\n")
	runtime := newManagedRuntime(runtimeSettings{uvPath: uvPath, cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
	err := installPythonAtWithRuntime(t.Context(), runtime, "3.12.13", filepath.Join(t.TempDir(), "python"), false, false, false)
	if err == nil || !strings.Contains(err.Error(), "download corrupt") {
		t.Fatalf("error = %v, want captured stderr", err)
	}
}

func TestFindPythonRejectsNonAbsoluteManagedResult(t *testing.T) {
	uvPath := writeRuntimeExecutable(t, "printf 'relative/python\\n'\n")
	runtime := newManagedRuntime(runtimeSettings{uvPath: uvPath, cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
	_, err := findPythonAtWithRuntime(t.Context(), runtime, "3.12.13", filepath.Join(t.TempDir(), "python"))
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("error = %v, want non-absolute managed Python rejection", err)
	}
}
