package python

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestManagedUVCommandIsolatesAmbientState(t *testing.T) {
	callerDir := t.TempDir()
	t.Chdir(callerDir)
	recordDir := t.TempDir()
	executable := writeRuntimeRecorder(t)

	for name, value := range map[string]string{
		"SB_TEST_RECORD":       recordDir,
		"UV":                   "hostile",
		"UV_CONFIG_FILE":       "hostile.toml",
		"UV_WORKING_DIR":       "/hostile/working-dir",
		"UV_WORKING_DIRECTORY": "/hostile/working-directory",
		"PIP_CONFIG_FILE":      "/hostile/pip.conf",
		"PYTHONPATH":           "/hostile/python",
		"PYTHONHOME":           "/hostile/home",
		"VIRTUAL_ENV":          "/hostile/venv",
		"VIRTUAL_ENV_PROMPT":   "hostile",
		"CONDA_PREFIX":         "/hostile/conda",
		"_CONDA_ROOT":          "/hostile/conda-root",
		"__PYVENV_LAUNCHER__":  "/hostile/launcher",
		"TMPDIR":               "/hostile/tmpdir",
		"TMP":                  "/hostile/tmp",
		"TEMP":                 "/hostile/temp",
		"UV_SYSTEM_CERTS":      "1",
		"UV_NATIVE_TLS":        "true",
		"HTTPS_PROXY":          "https://proxy.example.invalid",
		"http_proxy":           "http://proxy.example.invalid",
		"SSL_CERT_FILE":        "cert.pem",
		"SSL_CLIENT_CERT":      "client.pem",
		"REQUESTS_CA_BUNDLE":   "requests.pem",
		"CURL_CA_BUNDLE":       "curl.pem",
		"SSL_CERT_DIR":         string(os.PathListSeparator) + "certs" + string(os.PathListSeparator) + string(os.PathListSeparator) + "/absolute/certs",
	} {
		t.Setenv(name, value)
	}

	cacheDir := filepath.Join(t.TempDir(), "cache")
	scratchParent := t.TempDir()
	runtime := newManagedRuntime(runtimeSettings{cacheDir: cacheDir, scratchParent: scratchParent})
	_, err := runtime.run(t.Context(), managedCommand{
		path:     executable,
		args:     []string{"pip", "check", "--python", "relative/python"},
		kind:     managedUV,
		pathArgs: []int{3},
		pathEnv:  map[string]string{"UV_PYTHON_INSTALL_DIR": "relative/install"},
	})
	if err != nil {
		t.Fatal(err)
	}

	args := readRuntimeArgs(t, recordDir)
	wantArgs := []string{"--no-config", "pip", "check", "--python", filepath.Join(callerDir, "relative/python")}
	if !slices.Equal(args, wantArgs) {
		t.Fatalf("args = %q, want %q", args, wantArgs)
	}
	env := readRuntimeEnv(t, recordDir)
	for _, name := range []string{
		"UV", "UV_CONFIG_FILE", "UV_WORKING_DIR", "UV_WORKING_DIRECTORY", "PIP_CONFIG_FILE",
		"PYTHONPATH", "PYTHONHOME", "VIRTUAL_ENV", "VIRTUAL_ENV_PROMPT", "CONDA_PREFIX",
		"_CONDA_ROOT", "__PYVENV_LAUNCHER__",
	} {
		if _, ok := env[name]; ok {
			t.Errorf("filtered environment contains %s=%q", name, env[name])
		}
	}
	for name, want := range map[string]string{
		"PWD":                   "/",
		"PATH":                  standardSystemPath,
		"UV_CACHE_DIR":          cacheDir,
		"UV_PYTHON_INSTALL_DIR": filepath.Join(callerDir, "relative/install"),
		"UV_SYSTEM_CERTS":       "1",
		"UV_NATIVE_TLS":         "true",
		"HTTPS_PROXY":           "https://proxy.example.invalid",
		"http_proxy":            "http://proxy.example.invalid",
		"SSL_CERT_FILE":         filepath.Join(callerDir, "cert.pem"),
		"SSL_CLIENT_CERT":       filepath.Join(callerDir, "client.pem"),
		"REQUESTS_CA_BUNDLE":    filepath.Join(callerDir, "requests.pem"),
		"CURL_CA_BUNDLE":        filepath.Join(callerDir, "curl.pem"),
		"SSL_CERT_DIR":          string(os.PathListSeparator) + filepath.Join(callerDir, "certs") + string(os.PathListSeparator) + string(os.PathListSeparator) + "/absolute/certs",
	} {
		if got := env[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := strings.TrimSpace(readRuntimeFile(t, recordDir, "cwd")); got != "/" {
		t.Errorf("cwd = %q, want /", got)
	}
	assertScratchRemoved(t, env["TMPDIR"], scratchParent)
	if env["TMP"] != env["TMPDIR"] || env["TEMP"] != env["TMPDIR"] {
		t.Errorf("temporary environment = TMPDIR %q TMP %q TEMP %q", env["TMPDIR"], env["TMP"], env["TEMP"])
	}
	if info, statErr := os.Stat(cacheDir); statErr != nil || !info.IsDir() {
		t.Fatalf("managed cache was not created at %s: %v", cacheDir, statErr)
	}
}

func TestManagedRuntimeDoesNotReadDeletedWorkingDirectoryForAbsoluteInputs(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	deletedDir := filepath.Join(t.TempDir(), "deleted")
	if err := os.Mkdir(deletedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(deletedDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deletedDir); err != nil {
		t.Fatal(err)
	}

	recordDir := t.TempDir()
	t.Setenv("SB_TEST_RECORD", recordDir)
	runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
	executable := writeRuntimeRecorder(t)
	if _, err := runtime.run(t.Context(), managedCommand{path: executable, args: []string{"--version"}, kind: managedPython}); err != nil {
		t.Fatalf("absolute-input command from deleted cwd: %v", err)
	}
	if _, err := runtime.run(t.Context(), managedCommand{
		path: executable, args: []string{"-c", "pass", "relative"}, kind: managedPython, pathArgs: []int{2},
	}); err == nil || !strings.Contains(err.Error(), "current working directory") {
		t.Fatalf("relative input error = %v, want current-working-directory context", err)
	}
	if _, err := binaryVersionWithRuntime(t.Context(), runtime, "relative-uv", "uv"); err == nil || !strings.Contains(err.Error(), "current working directory") {
		t.Fatalf("relative uv path error = %v, want current-working-directory context", err)
	}
}

func TestManagedPythonAndEntrypointHealthFlags(t *testing.T) {
	t.Run("direct Python", func(t *testing.T) {
		recordDir := t.TempDir()
		t.Setenv("SB_TEST_RECORD", recordDir)
		runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
		if _, err := runtime.run(t.Context(), managedCommand{
			path: writeRuntimeRecorder(t), args: []string{"--version"}, kind: managedPython,
		}); err != nil {
			t.Fatal(err)
		}
		if got, want := readRuntimeArgs(t, recordDir), []string{"-I", "--version"}; !slices.Equal(got, want) {
			t.Fatalf("Python args = %q, want %q", got, want)
		}
	})

	t.Run("Ansible entrypoint", func(t *testing.T) {
		venvPath := t.TempDir()
		binPath := filepath.Join(venvPath, "bin")
		if err := os.Mkdir(binPath, 0o755); err != nil {
			t.Fatal(err)
		}
		recordDir := t.TempDir()
		t.Setenv("SB_TEST_RECORD", recordDir)
		t.Setenv("ANSIBLE_CONFIG", "/hostile/ansible.cfg")
		t.Setenv("ANSIBLE_COLLECTIONS_PATH", "/hostile/collections")
		ansiblePath := filepath.Join(binPath, "ansible")
		writeRuntimeExecutableAt(t, ansiblePath, `
printf '%s\n' "$PWD" > "$SB_TEST_RECORD/cwd"
printf '%s\000' "$@" > "$SB_TEST_RECORD/args"
env -0 > "$SB_TEST_RECORD/env"
`)
		runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
		if _, err := runtime.run(t.Context(), managedCommand{
			path: ansiblePath, args: []string{"--version"}, kind: managedEntrypoint,
			workingDir: venvPath, venvBin: binPath, ansibleHealth: true,
		}); err != nil {
			t.Fatal(err)
		}
		env := readRuntimeEnv(t, recordDir)
		if got, want := env["PATH"], binPath+string(os.PathListSeparator)+standardSystemPath; got != want {
			t.Errorf("PATH = %q, want %q", got, want)
		}
		if env["PYTHONNOUSERSITE"] != "1" || env["PYTHONSAFEPATH"] != "1" {
			t.Errorf("Python safety environment = %q, %q", env["PYTHONNOUSERSITE"], env["PYTHONSAFEPATH"])
		}
		if _, ok := env["ANSIBLE_COLLECTIONS_PATH"]; ok {
			t.Error("ANSIBLE_COLLECTIONS_PATH survived Ansible health isolation")
		}
		configPath := env["ANSIBLE_CONFIG"]
		if data, readErr := os.ReadFile(configPath); readErr == nil || !os.IsNotExist(readErr) || len(data) != 0 {
			t.Fatalf("temporary Ansible config remained after command: data=%q error=%v", data, readErr)
		}
		if got := strings.TrimSpace(readRuntimeFile(t, recordDir, "cwd")); got != venvPath {
			t.Errorf("cwd = %q, want %q", got, venvPath)
		}
		assertScratchRemoved(t, env["TMPDIR"], filepath.Dir(env["TMPDIR"]))
	})
}

func TestManagedRuntimeCleansScratchOnErrorAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name    string
		script  string
		context func() (context.Context, context.CancelFunc)
	}{
		{
			name:   "error",
			script: "printf '%s' \"$TMPDIR\" > \"$SB_TEST_RECORD/scratch\"\nprintf 'failure detail\\n' >&2\nexit 23\n",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
		},
		{
			name:   "cancellation",
			script: "printf '%s' \"$TMPDIR\" > \"$SB_TEST_RECORD/scratch\"\nwhile :; do sleep 1; done\n",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(t.Context(), 100*time.Millisecond)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recordDir := t.TempDir()
			t.Setenv("SB_TEST_RECORD", recordDir)
			scratchParent := t.TempDir()
			runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: scratchParent})
			ctx, cancel := test.context()
			defer cancel()
			_, err := runtime.run(ctx, managedCommand{path: writeRuntimeExecutable(t, test.script), kind: managedPython})
			if err == nil {
				t.Fatal("command unexpectedly succeeded")
			}
			if test.name == "cancellation" && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("context error = %v, want deadline exceeded", ctx.Err())
			}
			scratch := strings.TrimSpace(readRuntimeFile(t, recordDir, "scratch"))
			assertScratchRemoved(t, scratch, scratchParent)
		})
	}
}

func TestManagedPythonIgnoresImportShadowing(t *testing.T) {
	pythonPath := "/usr/bin/python3"
	if _, err := os.Stat(pythonPath); err != nil {
		t.Skipf("system Python unavailable: %v", err)
	}
	callerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(callerDir, "encodings.py"), []byte("raise RuntimeError('shadowed')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(callerDir, "sitecustomize.py"), []byte("raise RuntimeError('shadowed')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(callerDir)
	t.Setenv("PYTHONPATH", callerDir)
	t.Setenv("PYTHONHOME", callerDir)

	runtime := newManagedRuntime(runtimeSettings{cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir()})
	if _, err := runtime.run(t.Context(), managedCommand{
		path: pythonPath,
		args: []string{"-c", "import encodings, sys; assert sys.flags.isolated == 1"},
		kind: managedPython,
	}); err != nil {
		t.Fatalf("isolated Python probe loaded ambient shadowing: %v", err)
	}
}

func TestRealUVBinaryIgnoresAmbientProjectsAndConfiguration(t *testing.T) {
	uvPath := os.Getenv("SB_TEST_UV_BINARY")
	if uvPath == "" {
		t.Skip("SB_TEST_UV_BINARY is not set")
	}
	if !filepath.IsAbs(uvPath) {
		t.Fatalf("SB_TEST_UV_BINARY = %q, want absolute path", uvPath)
	}

	ambientRoot := t.TempDir()
	callerDir := filepath.Join(ambientRoot, "project", "child")
	if err := os.MkdirAll(filepath.Join(callerDir, ".venv"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(ambientRoot, ".python-version"):   "9.9.9\n",
		filepath.Join(ambientRoot, "uv.toml"):           "invalid = [",
		filepath.Join(callerDir, ".python-version"):     "8.8.8\n",
		filepath.Join(callerDir, ".env"):                "UV_PYTHON=7.7.7\n",
		filepath.Join(callerDir, "uv.toml"):             "invalid = [",
		filepath.Join(callerDir, ".venv", "pyvenv.cfg"): "home = /hostile\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(callerDir)
	for name, value := range map[string]string{
		"UV_CONFIG_FILE":       filepath.Join(callerDir, "uv.toml"),
		"UV_WORKING_DIR":       ambientRoot,
		"UV_WORKING_DIRECTORY": ambientRoot,
		"UV_PYTHON":            "6.6.6",
		"PIP_CONFIG_FILE":      filepath.Join(callerDir, "missing-pip.conf"),
		"PYTHONPATH":           callerDir,
		"VIRTUAL_ENV":          filepath.Join(callerDir, ".venv"),
		"HOME":                 filepath.Join(callerDir, "missing-home"),
		"XDG_CONFIG_HOME":      filepath.Join(callerDir, "missing-xdg-config"),
		"XDG_CACHE_HOME":       filepath.Join(callerDir, "missing-xdg-cache"),
		"TMPDIR":               "/dev/null",
		"TMP":                  "/dev/null",
		"TEMP":                 "/dev/null",
	} {
		t.Setenv(name, value)
	}

	runtime := newManagedRuntime(runtimeSettings{
		uvPath: uvPath, cacheDir: filepath.Join(t.TempDir(), "cache"), scratchParent: t.TempDir(),
	})
	version, err := binaryVersionWithRuntime(t.Context(), runtime, uvPath, "uv")
	if err != nil {
		t.Fatalf("real uv version probe: %v", err)
	}
	if !exactUVVersionPattern.MatchString(version) {
		t.Fatalf("real uv version = %q, want exact version", version)
	}
	if _, err := listInstalledPythonsWithRuntime(t.Context(), runtime, filepath.Join(t.TempDir(), "python-install")); err != nil {
		t.Fatalf("real uv managed Python listing: %v", err)
	}
}

func TestManagedUVStorageErrorIsSurfacedAsUnprivileged(t *testing.T) {
	if os.Getenv("SB_TEST_PERMISSION_CHILD") == "1" {
		cacheDir := os.Getenv("SB_TEST_PERMISSION_CACHE")
		pythonPath := os.Getenv("SB_TEST_PERMISSION_PYTHON")
		runtime := newManagedRuntime(runtimeSettings{
			uvPath: "/bin/true", cacheDir: cacheDir, scratchParent: os.Getenv("SB_TEST_PERMISSION_SCRATCH"),
		})
		_, err := runtime.run(t.Context(), managedCommand{
			path: runtime.uvPath, args: []string{"pip", "check", "--python", pythonPath}, kind: managedUV, pathArgs: []int{3},
		})
		if err == nil || !strings.Contains(err.Error(), cacheDir) {
			t.Fatalf("storage error = %v, want managed cache path %s", err, cacheDir)
		}
		return
	}
	if os.Geteuid() != 0 {
		t.Skip("root is required to verify a real uid/gid permission boundary")
	}

	root, err := os.MkdirTemp("/tmp", "sb-go-python-permission-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	blockedParent := filepath.Join(root, "root-only")
	if err := os.Mkdir(blockedParent, 0o700); err != nil {
		t.Fatal(err)
	}
	scratchParent := filepath.Join(root, "scratch")
	if err := os.Mkdir(scratchParent, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(scratchParent, 0o777); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(blockedParent, "uv")

	testBinary := filepath.Join(root, "python.test")
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(testBinary, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
	if err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := errors.Join(source.Close(), destination.Close())
	if err := errors.Join(copyErr, closeErr); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(testBinary, "-test.run=^TestManagedUVStorageErrorIsSurfacedAsUnprivileged$")
	command.Env = append(os.Environ(),
		"SB_TEST_PERMISSION_CHILD=1",
		"SB_TEST_PERMISSION_CACHE="+cacheDir,
		"SB_TEST_PERMISSION_PYTHON="+filepath.Join(root, "python"),
		"SB_TEST_PERMISSION_SCRATCH="+scratchParent,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("unprivileged permission subprocess: %v\n%s", err, output)
	}
}

func writeRuntimeRecorder(t *testing.T) string {
	t.Helper()
	return writeRuntimeExecutable(t, `
printf '%s\n' "$PWD" > "$SB_TEST_RECORD/cwd"
printf '%s\000' "$@" > "$SB_TEST_RECORD/args"
env -0 > "$SB_TEST_RECORD/env"
`)
}

func writeRuntimeExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command")
	writeRuntimeExecutableAt(t, path, body)
	return path
}

func writeRuntimeExecutableAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readRuntimeArgs(t *testing.T, directory string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "args"))
	if err != nil {
		t.Fatal(err)
	}
	args := slices.DeleteFunc(strings.Split(string(data), "\x00"), func(value string) bool { return value == "" })
	return args
}

func readRuntimeEnv(t *testing.T, directory string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "env"))
	if err != nil {
		t.Fatal(err)
	}
	environment := make(map[string]string)
	for entry := range strings.SplitSeq(string(data), "\x00") {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[name] = value
		}
	}
	return environment
}

func readRuntimeFile(t *testing.T, directory, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertScratchRemoved(t *testing.T, scratch, parent string) {
	t.Helper()
	if filepath.Dir(scratch) != parent {
		t.Fatalf("scratch %q is outside parent %q", scratch, parent)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("scratch %q remains after command: %v", scratch, err)
	}
}
