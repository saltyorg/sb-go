package python

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/executor"
)

const (
	standardSystemPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	managedUVCacheDir  = "/var/cache/sb-go/uv"
)

type managedCommandKind uint8

const (
	managedUV managedCommandKind = iota
	managedPython
	managedEntrypoint
)

type commandExecutor interface {
	Execute(*executor.Config) (*executor.Result, error)
}

type runtimeSettings struct {
	uvPath        string
	cacheDir      string
	scratchParent string
	executor      commandExecutor
}

type managedRuntime struct {
	uvPath        string
	cacheDir      string
	scratchParent string
	executor      commandExecutor
}

type managedCommand struct {
	path          string
	args          []string
	kind          managedCommandKind
	workingDir    string
	venvBin       string
	pathArgs      []int
	pathEnv       map[string]string
	ansibleHealth bool
	outputMode    executor.OutputMode
	outputModeSet bool
	stdout        io.Writer
	stderr        io.Writer
}

func newManagedRuntime(settings runtimeSettings) *managedRuntime {
	if settings.uvPath == "" {
		settings.uvPath = UVBinaryPath
	}
	if settings.cacheDir == "" {
		settings.cacheDir = managedUVCacheDir
	}
	if settings.scratchParent == "" {
		settings.scratchParent = "/tmp"
	}
	if settings.executor == nil {
		settings.executor = executor.NewExecutor()
	}
	return &managedRuntime{
		uvPath:        settings.uvPath,
		cacheDir:      settings.cacheDir,
		scratchParent: settings.scratchParent,
		executor:      settings.executor,
	}
}

func (runtime *managedRuntime) runVerbose(ctx context.Context, command managedCommand, verbose bool) error {
	command.outputModeSet = true
	if verbose {
		command.outputMode = executor.OutputModeStream
	} else {
		command.outputMode = executor.OutputModeDiscard
	}
	result, err := runtime.run(ctx, command)
	if err == nil {
		return nil
	}
	if result != nil && len(result.Stderr) > 0 {
		return fmt.Errorf("command failed: %w\nStderr:\n%s", err, string(result.Stderr))
	}
	return fmt.Errorf("command failed: %w", err)
}

func defaultManagedRuntime() *managedRuntime {
	return newManagedRuntime(runtimeSettings{})
}

func (runtime *managedRuntime) run(ctx context.Context, command managedCommand) (*executor.Result, error) {
	callerDir := ""
	callerDirLoaded := false
	loadCallerDir := func() (string, error) {
		if callerDirLoaded {
			return callerDir, nil
		}
		workingDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("read current working directory for relative managed Python path: %w", err)
		}
		callerDir = workingDir
		callerDirLoaded = true
		return callerDir, nil
	}
	resolvePath := func(path, description string) (string, error) {
		if path == "" {
			return "", fmt.Errorf("%s path is empty", description)
		}
		if filepath.IsAbs(path) {
			return filepath.Clean(path), nil
		}
		workingDir, err := loadCallerDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(workingDir, path), nil
	}

	commandPath, err := resolvePath(command.path, "managed command")
	if err != nil {
		return nil, err
	}
	args := append([]string(nil), command.args...)
	for _, index := range command.pathArgs {
		if index < 0 || index >= len(args) {
			return nil, fmt.Errorf("managed command path argument index %d is out of range", index)
		}
		args[index], err = resolvePath(args[index], fmt.Sprintf("managed command argument %d", index))
		if err != nil {
			return nil, err
		}
	}
	pathEnv := make(map[string]string, len(command.pathEnv))
	for name, path := range command.pathEnv {
		pathEnv[name], err = resolvePath(path, name)
		if err != nil {
			return nil, err
		}
	}
	cacheDir := runtime.cacheDir
	if command.kind == managedUV {
		cacheDir, err = resolvePath(cacheDir, "managed uv cache")
		if err != nil {
			return nil, err
		}
		if !isVersionProbe(command.args) {
			if err := os.MkdirAll(cacheDir, 0o755); err != nil {
				return nil, fmt.Errorf("create managed uv cache %s: %w", cacheDir, err)
			}
		}
	}

	workingDir := command.workingDir
	if command.kind != managedEntrypoint {
		workingDir = "/"
	} else {
		workingDir, err = resolvePath(workingDir, "managed entrypoint working directory")
		if err != nil {
			return nil, err
		}
	}
	venvBin := command.venvBin
	if venvBin != "" {
		venvBin, err = resolvePath(venvBin, "managed venv bin")
		if err != nil {
			return nil, err
		}
	}

	environment, err := managedEnvironment(os.Environ(), loadCallerDir, command.ansibleHealth)
	if err != nil {
		return nil, err
	}
	scratchDir, err := runtime.newScratch()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratchDir) }()

	environment = setEnvironment(environment, "PWD", workingDir)
	environment = setEnvironment(environment, "PATH", standardSystemPath)
	environment = setEnvironment(environment, "TMPDIR", scratchDir)
	environment = setEnvironment(environment, "TMP", scratchDir)
	environment = setEnvironment(environment, "TEMP", scratchDir)
	if command.kind == managedUV {
		environment = setEnvironment(environment, "UV_CACHE_DIR", cacheDir)
		args = append([]string{"--no-config"}, args...)
	}
	if command.kind == managedPython {
		args = append([]string{"-I"}, args...)
	}
	if command.kind == managedEntrypoint {
		environment = setEnvironment(environment, "PYTHONNOUSERSITE", "1")
		environment = setEnvironment(environment, "PYTHONSAFEPATH", "1")
		if venvBin != "" {
			environment = setEnvironment(environment, "PATH", venvBin+string(os.PathListSeparator)+standardSystemPath)
		}
	}
	if command.ansibleHealth {
		configPath := filepath.Join(scratchDir, "ansible.cfg")
		if err := os.WriteFile(configPath, nil, 0o600); err != nil {
			return nil, fmt.Errorf("create isolated Ansible config %s: %w", configPath, err)
		}
		environment = setEnvironment(environment, "ANSIBLE_CONFIG", configPath)
	}
	for name, path := range pathEnv {
		environment = setEnvironment(environment, name, path)
	}

	config := &executor.Config{
		Context:    ctx,
		Command:    commandPath,
		Args:       args,
		WorkingDir: workingDir,
		Env:        environment,
		OutputMode: executor.OutputModeCombined,
		Stdout:     command.stdout,
		Stderr:     command.stderr,
	}
	if command.outputModeSet {
		config.OutputMode = command.outputMode
	}
	return runtime.executor.Execute(config)
}

func (runtime *managedRuntime) newScratch() (string, error) {
	scratchDir, err := os.MkdirTemp(runtime.scratchParent, "sb-go-python-*")
	if err != nil {
		return "", fmt.Errorf("create managed Python scratch directory in %s: %w", runtime.scratchParent, err)
	}
	return scratchDir, nil
}

func isVersionProbe(args []string) bool {
	return len(args) == 1 && args[0] == "--version"
}

func managedEnvironment(parent []string, callerDir func() (string, error), ansibleHealth bool) ([]string, error) {
	environment := make([]string, 0, len(parent))
	for _, entry := range parent {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || filteredManagedEnvironmentName(name) || (ansibleHealth && strings.HasPrefix(name, "ANSIBLE_")) {
			continue
		}
		if isCertificatePathEnvironment(name) {
			resolved, err := resolveEnvironmentPath(value, callerDir)
			if err != nil {
				return nil, fmt.Errorf("resolve %s relative to current working directory: %w", name, err)
			}
			value = resolved
		}
		if name == "SSL_CERT_DIR" {
			resolved, err := resolveEnvironmentPathList(value, callerDir)
			if err != nil {
				return nil, fmt.Errorf("resolve SSL_CERT_DIR relative to current working directory: %w", err)
			}
			value = resolved
		}
		environment = setEnvironment(environment, name, value)
	}
	return environment, nil
}

func filteredManagedEnvironmentName(name string) bool {
	if name == "UV_SYSTEM_CERTS" || name == "UV_NATIVE_TLS" {
		return false
	}
	return name == "UV" ||
		strings.HasPrefix(name, "UV_") ||
		strings.HasPrefix(name, "PIP_") ||
		strings.HasPrefix(name, "PYTHON") ||
		strings.HasPrefix(name, "VIRTUAL_ENV") ||
		strings.HasPrefix(name, "CONDA_") ||
		strings.HasPrefix(name, "_CONDA_") ||
		name == "__PYVENV_LAUNCHER__"
}

func isCertificatePathEnvironment(name string) bool {
	switch name {
	case "SSL_CERT_FILE", "SSL_CLIENT_CERT", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE":
		return true
	default:
		return false
	}
}

func resolveEnvironmentPath(path string, callerDir func() (string, error)) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return path, nil
	}
	workingDir, err := callerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(workingDir, path), nil
}

func resolveEnvironmentPathList(value string, callerDir func() (string, error)) (string, error) {
	paths := filepath.SplitList(value)
	for index, path := range paths {
		resolved, err := resolveEnvironmentPath(path, callerDir)
		if err != nil {
			return "", err
		}
		paths[index] = resolved
	}
	return strings.Join(paths, string(os.PathListSeparator)), nil
}

func setEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	filtered := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}
