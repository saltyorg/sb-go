package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFiles(t *testing.T) {
	dir := t.TempDir()
	pythonPath := filepath.Join(dir, ".python-version")
	uvPath := filepath.Join(dir, ".uv-version")
	if err := os.WriteFile(pythonPath, []byte("3.12.13\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uvPath, []byte("0.12.3\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadFiles(pythonPath, uvPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.13" || config.PythonMinor != "3.12" || config.MinimumUV != "0.12.3" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestLoadFilesRejectsNonExactVersions(t *testing.T) {
	dir := t.TempDir()
	pythonPath := filepath.Join(dir, ".python-version")
	uvPath := filepath.Join(dir, ".uv-version")
	if err := os.WriteFile(pythonPath, []byte("3.12\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uvPath, []byte("0.12.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFiles(pythonPath, uvPath); err == nil {
		t.Fatal("non-exact Python version was accepted")
	}
}

func TestLoadFromRootUsesOnlyCanonicalVersionFiles(t *testing.T) {
	canonicalRoot := t.TempDir()
	writeVersionFile(t, canonicalRoot, ".python-version", "3.12.13\n")
	writeVersionFile(t, canonicalRoot, ".uv-version", "0.12.10\n")

	ambientRoot := t.TempDir()
	ambientProject := filepath.Join(ambientRoot, "project")
	if err := os.Mkdir(ambientProject, 0o755); err != nil {
		t.Fatal(err)
	}
	writeVersionFile(t, ambientRoot, ".python-version", "9.9.9\n")
	writeVersionFile(t, ambientProject, ".python-version", "8.8.8\n")
	writeVersionFile(t, ambientProject, ".uv-version", "7.7.7\n")
	writeVersionFile(t, ambientProject, "uv.toml", "invalid = [")
	t.Chdir(ambientProject)
	t.Setenv("UV_CONFIG_FILE", filepath.Join(ambientProject, "uv.toml"))
	t.Setenv("UV_WORKING_DIR", ambientRoot)

	config, err := loadFromRoot(canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.13" || config.MinimumUV != "0.12.10" {
		t.Fatalf("config = %#v, want canonical Python and uv versions", config)
	}

	writeVersionFile(t, canonicalRoot, ".python-version", "3.12.14\n")
	config, err = loadFromRoot(canonicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if config.Python != "3.12.14" {
		t.Fatalf("Python = %q after canonical bump, want 3.12.14", config.Python)
	}
}

func TestLoadFromRootReportsCanonicalVersionFailures(t *testing.T) {
	root := t.TempDir()
	writeVersionFile(t, root, ".uv-version", "0.12.10\n")
	writeVersionFile(t, root, ".python-version", "3.12\n")
	if _, err := loadFromRoot(root); err == nil || !strings.Contains(err.Error(), filepath.Join(root, ".python-version")) {
		t.Fatalf("invalid canonical Python error = %v", err)
	}
}

func TestLoadFilesWithoutPinsFallsBackToPythonMinor(t *testing.T) {
	dir := t.TempDir()
	config, err := LoadFiles(filepath.Join(dir, ".python-version"), filepath.Join(dir, ".uv-version"))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{PythonMinor: "3.12"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
	if config.Pinned() {
		t.Fatal("a checkout without .python-version reports a pinned Python")
	}
	if err := checkMinimumUV(config, "0.0.1"); err != nil {
		t.Fatalf("missing .uv-version still enforced a minimum: %v", err)
	}
}

func TestLoadFilesWithPythonPinAndNoUVMinimum(t *testing.T) {
	dir := t.TempDir()
	writeVersionFile(t, dir, ".python-version", "3.12.41\n")
	config, err := LoadFiles(filepath.Join(dir, ".python-version"), filepath.Join(dir, ".uv-version"))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Python: "3.12.41", PythonMinor: "3.12"}
	if config != want {
		t.Fatalf("config = %#v, want %#v", config, want)
	}
	if !config.Pinned() {
		t.Fatal("a checkout with .python-version reports no pinned Python")
	}
	if err := checkMinimumUV(config, "0.0.1"); err != nil {
		t.Fatalf("missing .uv-version still enforced a minimum: %v", err)
	}
}

func TestLoadFilesRefusesMalformedPins(t *testing.T) {
	for _, test := range []struct {
		name   string
		python string
		uv     string
		bad    string
	}{
		{name: "Python minor only", python: "3.12\n", uv: "0.90.1\n", bad: ".python-version"},
		{name: "empty Python pin", python: "\n", uv: "0.90.1\n", bad: ".python-version"},
		{name: "Python pre-release", python: "3.12.41rc1\n", bad: ".python-version"},
		{name: "uv minimum without patch", python: "3.12.41\n", uv: "0.90\n", bad: ".uv-version"},
		{name: "uv minimum without Python pin", uv: "latest\n", bad: ".uv-version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.python != "" {
				writeVersionFile(t, dir, ".python-version", test.python)
			}
			if test.uv != "" {
				writeVersionFile(t, dir, ".uv-version", test.uv)
			}
			_, err := LoadFiles(filepath.Join(dir, ".python-version"), filepath.Join(dir, ".uv-version"))
			badPath := filepath.Join(dir, test.bad)
			if err == nil || !strings.Contains(err.Error(), badPath+" must contain one exact major.minor.patch version") {
				t.Fatalf("error = %v, want the malformed %s refused", err, badPath)
			}
		})
	}
}

func TestCheckMinimumUV(t *testing.T) {
	config := Config{Python: "3.12.41", PythonMinor: "3.12", MinimumUV: "0.90.1"}
	if err := checkMinimumUV(config, "0.90.1"); err != nil {
		t.Fatalf("equal uv refused: %v", err)
	}
	err := checkMinimumUV(config, "0.90.0")
	want := "saltbox requires uv 0.90.1 or newer, but this sb release provides 0.90.0; run sb self-update"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

// pythonListFixture is invented uv `python list --output-format json` output:
// numeric patch ordering, other variants, other implementations and a
// pre-release must not be picked.
const pythonListFixture = `[
  {"key": "cpython-3.13.50-linux-x86_64-gnu", "version": "3.13.50", "implementation": "cpython", "variant": "default"},
  {"key": "cpython-3.12.80rc1-linux-x86_64-gnu", "version": "3.12.80rc1", "implementation": "cpython", "variant": "default"},
  {"key": "cpython-3.12.60+freethreaded-linux-x86_64-gnu", "version": "3.12.60", "implementation": "cpython", "variant": "freethreaded"},
  {"key": "pypy-3.12.70-linux-x86_64-gnu", "version": "3.12.70", "implementation": "pypy", "variant": "default"},
  {"key": "cpython-3.12.9-linux-x86_64-gnu", "version": "3.12.9", "implementation": "cpython", "variant": "default"},
  {"key": "cpython-3.12.41-linux-x86_64-gnu", "version": "3.12.41", "implementation": "cpython", "variant": "default"},
  {"key": "cpython-3.12.7-linux-x86_64-gnu", "version": "3.12.7", "implementation": "cpython", "variant": "default"}
]`

// fakeListUV records every invocation and answers `python list` with output,
// or fails with failure on stderr when failure is set.
func fakeListUV(t *testing.T, output, failure string) (*managedRuntime, string) {
	t.Helper()
	recordDir := t.TempDir()
	t.Setenv("SB_TEST_RECORD", recordDir)
	listPath := filepath.Join(recordDir, "list.json")
	if err := os.WriteFile(listPath, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SB_TEST_LIST", listPath)
	t.Setenv("SB_TEST_FAILURE", failure)
	uvPath := writeRuntimeExecutable(t, `
printf '%s\000' "$@" >> "$SB_TEST_RECORD/args"
printf '\n' >> "$SB_TEST_RECORD/args"
if [ -n "$SB_TEST_FAILURE" ]; then
  printf '%s\n' "$SB_TEST_FAILURE" >&2
  exit 2
fi
if [ "$2 $3" = "python list" ]; then cat "$SB_TEST_LIST"; fi
`)
	return newManagedRuntime(runtimeSettings{
		uvPath:        uvPath,
		cacheDir:      filepath.Join(t.TempDir(), "cache"),
		scratchParent: t.TempDir(),
	}), recordDir
}

// assertOnlyPythonListed checks that uv was asked for its Python list once
// and for nothing else, so nothing was installed.
func assertOnlyPythonListed(t *testing.T, recordDir string) {
	t.Helper()
	calls := strings.Split(strings.TrimSuffix(readRuntimeFile(t, recordDir, "args"), "\n"), "\n")
	want := strings.Join([]string{"--no-config", "python", "list", "--only-downloads", "--all-versions", "--output-format", "json"}, "\x00") + "\x00"
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("uv calls = %q, want only %q", calls, want)
	}
}

func TestResolveToolchainWithoutPinsUsesNewestFallbackPatch(t *testing.T) {
	runtime, recordDir := fakeListUV(t, pythonListFixture, "")
	toolchain, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{PythonMinor: "3.12"}, "0.90.1")
	if err != nil {
		t.Fatal(err)
	}
	want := Toolchain{
		Python: "3.12.41",
		Hashed: false,
		Notice: "This Saltbox branch has no pinned Python; using Python 3.12.41, the newest 3.12 this sb's uv (0.90.1) provides",
	}
	if toolchain != want {
		t.Fatalf("toolchain = %#v, want %#v", toolchain, want)
	}
	assertOnlyPythonListed(t, recordDir)
}

func TestResolveToolchainWithListedPinInstallsThatPin(t *testing.T) {
	runtime, recordDir := fakeListUV(t, pythonListFixture, "")
	toolchain, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{Python: "3.12.9", PythonMinor: "3.12"}, "0.90.1")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Toolchain{Python: "3.12.9", Hashed: true}); toolchain != want {
		t.Fatalf("toolchain = %#v, want %#v", toolchain, want)
	}
	assertOnlyPythonListed(t, recordDir)
}

func TestResolveToolchainRefusesPinTheUVCannotInstall(t *testing.T) {
	for _, pin := range []string{"3.12.42", "3.12.60", "3.12.70"} {
		t.Run(pin, func(t *testing.T) {
			runtime, recordDir := fakeListUV(t, pythonListFixture, "")
			_, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{Python: pin, PythonMinor: "3.12"}, "0.90.1")
			want := "this Saltbox branch needs Python " + pin + ", which this sb's uv (0.90.1) can't install; run sb self-update"
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
			assertOnlyPythonListed(t, recordDir)
		})
	}
}

func TestResolveToolchainWithoutFallbackMinorFails(t *testing.T) {
	runtime, _ := fakeListUV(t, `[{"version": "3.13.50", "implementation": "cpython", "variant": "default"}]`, "")
	_, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{PythonMinor: "3.12"}, "0.90.1")
	if err == nil || !strings.Contains(err.Error(), "this sb's uv (0.90.1) offers no Python 3.12") {
		t.Fatalf("error = %v, want missing fallback minor", err)
	}
}

func TestResolveToolchainReportsUVListFailure(t *testing.T) {
	runtime, _ := fakeListUV(t, "", "error: invented list failure")
	_, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{Python: "3.12.41", PythonMinor: "3.12"}, "0.90.1")
	if err == nil || !strings.Contains(err.Error(), "error: invented list failure") || !strings.Contains(err.Error(), "uv 0.90.1") {
		t.Fatalf("error = %v, want uv's own message", err)
	}
}

func TestResolveToolchainRejectsUnreadableUVList(t *testing.T) {
	runtime, _ := fakeListUV(t, "not json", "")
	_, err := resolveToolchainWithRuntime(t.Context(), runtime, Config{Python: "3.12.41", PythonMinor: "3.12"}, "0.90.1")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error = %v, want parse failure", err)
	}
}

func writeVersionFile(t *testing.T, root, name, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		actual  string
		minimum string
		want    bool
	}{
		{"0.12.3", "0.12.3", true},
		{"0.12.4", "0.12.3", true},
		{"0.13.0", "0.12.99", true},
		{"0.12.2", "0.12.3", false},
	}
	for _, test := range tests {
		got, err := AtLeast(test.actual, test.minimum)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("AtLeast(%q, %q) = %v, want %v", test.actual, test.minimum, got, test.want)
		}
	}
}
