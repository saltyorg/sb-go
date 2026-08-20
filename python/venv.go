package python

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/saltyorg/sb-go/buildinfo"
	"github.com/saltyorg/sb-go/executor"
	"github.com/saltyorg/sb-go/host"
	"github.com/saltyorg/sb-go/layout"
	"github.com/saltyorg/sb-go/terminal"
)

const (
	manifestSchemaVersion = 1
	manifestFileName      = "manifest.json"
	wrapperMarker         = "# Managed by sb-go Saltbox venv"
)

type Options struct {
	ForceVenv   bool
	ForcePython bool
	SaltboxUser string
	Verbose     bool
}

func (o Options) noCache() bool {
	return o.ForceVenv || o.ForcePython
}

type Manifest struct {
	SchemaVersion    int      `json:"schema_version"`
	GenerationID     string   `json:"generation_id"`
	PythonVersion    string   `json:"python_version"`
	PythonPath       string   `json:"python_path"`
	PythonInstall    string   `json:"python_install,omitempty"`
	UVVersion        string   `json:"uv_version"`
	LockSHA256       string   `json:"lock_sha256"`
	SaltboxCommit    string   `json:"saltbox_commit"`
	CreatedAt        string   `json:"created_at"`
	ExportedCommands []string `json:"exported_commands"`
}

type activeState struct {
	Manifest      *Manifest
	GenerationDir string
	VenvPath      string
	Healthy       bool
}

func ManageAnsibleVenv(
	ctx context.Context,
	task *terminal.Task,
	forceRecreate bool,
	saltboxUser string,
	verbose bool,
) error {
	return Reconcile(ctx, task, Options{
		ForceVenv:   forceRecreate,
		SaltboxUser: saltboxUser,
		Verbose:     verbose,
	})
}

func Reconcile(ctx context.Context, task *terminal.Task, options Options) error {
	config, err := Load()
	if err != nil {
		return fmt.Errorf("load Saltbox Python toolchain: %w", err)
	}
	uvVersion := buildinfo.Current().UVVersion
	compatible, err := AtLeast(uvVersion, config.MinimumUV)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf(
			"saltbox requires uv %s or newer, but this sb release provides %s; run sb self-update",
			config.MinimumUV,
			uvVersion,
		)
	}

	lockDigest, err := fileSHA256(layout.AnsibleRequirementsPath)
	if err != nil {
		return fmt.Errorf("hash Saltbox requirements lock: %w", err)
	}
	commit, err := saltboxCommit(ctx)
	if err != nil {
		return err
	}

	if err := task.RunStreaming(ctx, terminal.TaskSpec{Running: fmt.Sprintf("Ensuring uv %s is installed", uvVersion)}, func(taskCtx context.Context) error {
		return DownloadAndInstallUV(taskCtx, options.Verbose)
	}); err != nil {
		return fmt.Errorf("install pinned uv: %w", err)
	}

	libpqInstalled, err := host.IsPackageInstalled(ctx, "libpq-dev")
	if err != nil {
		return fmt.Errorf("check native Python build prerequisites: %w", err)
	}
	if !libpqInstalled {
		if err := task.RunStreaming(ctx, terminal.TaskSpec{Running: "Updating apt package cache"}, func(taskCtx context.Context) error {
			return host.UpdatePackageLists(taskCtx, options.Verbose)()
		}); err != nil {
			return fmt.Errorf("update apt cache for native Python build prerequisites: %w", err)
		}
		if err := task.RunStreaming(ctx, terminal.TaskSpec{Running: "Installing native Python build prerequisites"}, func(taskCtx context.Context) error {
			return host.InstallPackage(taskCtx, []string{"libpq-dev"}, options.Verbose)()
		}); err != nil {
			return fmt.Errorf("install native Python build prerequisites: %w", err)
		}
	}

	state, err := inspectActive(ctx, config.Python, lockDigest)
	if err != nil {
		return fmt.Errorf("inspect active Ansible environment: %w", err)
	}
	if err := task.Run(ctx, environmentStatusTaskSpec(state.Healthy, options), func(context.Context, *terminal.Task) error {
		return nil
	}); err != nil {
		return err
	}
	if state.Healthy && !options.ForceVenv && !options.ForcePython {
		if err := installWrappers(state.Manifest.ExportedCommands); err != nil {
			return fmt.Errorf("refresh Ansible command wrappers: %w", err)
		}
		if err := cleanupGenerations(); err != nil {
			return fmt.Errorf("clean old Ansible generations: %w", err)
		}
		return ensureManagedOwnership(options.SaltboxUser)
	}

	pythonPath := ""
	pythonInstall := ""
	if !options.ForcePython && state.Manifest != nil && state.Manifest.PythonVersion == config.Python {
		if err := validatePython(ctx, state.Manifest.PythonPath, config.Python); err == nil {
			pythonPath = state.Manifest.PythonPath
			pythonInstall = state.Manifest.PythonInstall
		}
	}
	if pythonPath == "" {
		pythonInstall, pythonPath, err = createPythonRelease(ctx, task, config.Python, options)
		if err != nil {
			return err
		}
	}

	generationID := generationID(lockDigest)
	generationDir := filepath.Join(layout.AnsibleReleasesPath, generationID)
	venvPath := filepath.Join(generationDir, "venv")
	activated := false
	defer func() {
		if !activated {
			_ = os.RemoveAll(generationDir)
			if pythonInstall != "" && strings.HasPrefix(pythonInstall, layout.PythonReleasesPath+string(os.PathSeparator)) {
				_ = removePythonIfUnreferenced(pythonInstall)
			}
		}
	}()

	if err := os.MkdirAll(generationDir, 0755); err != nil {
		return fmt.Errorf("create Ansible generation %s: %w", generationID, err)
	}
	if err := task.Run(ctx, terminal.TaskSpec{Running: "Creating Ansible virtual environment"}, func(taskCtx context.Context, _ *terminal.Task) error {
		return CreateVenvWithPython(taskCtx, venvPath, pythonPath, options.Verbose)
	}); err != nil {
		return err
	}

	venvPython := filepath.Join(venvPath, "bin", "python3")
	if err := task.RunOutput(ctx, terminal.TaskSpec{Running: "Syncing hashed Saltbox requirements"}, func(taskCtx context.Context, stdout, stderr io.Writer) error {
		return SyncRequirements(taskCtx, venvPython, layout.AnsibleRequirementsPath, SyncRequirementsOptions{
			NoCache: options.noCache(),
			Verbose: options.Verbose,
			Stdout:  stdout,
			Stderr:  stderr,
		})
	}); err != nil {
		return err
	}

	commands, err := discoverCommands(venvPath)
	if err != nil {
		return err
	}
	if err := validateEnvironment(ctx, venvPath, config.Python); err != nil {
		return fmt.Errorf("validate staged Ansible environment: %w", err)
	}

	manifest := &Manifest{
		SchemaVersion:    manifestSchemaVersion,
		GenerationID:     generationID,
		PythonVersion:    config.Python,
		PythonPath:       pythonPath,
		PythonInstall:    pythonInstall,
		UVVersion:        buildinfo.Current().UVVersion,
		LockSHA256:       lockDigest,
		SaltboxCommit:    commit,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		ExportedCommands: commands,
	}
	if err := writeManifest(generationDir, manifest); err != nil {
		return err
	}
	if err := installWrappers(commands); err != nil {
		return fmt.Errorf("install Ansible command wrappers: %w", err)
	}
	if err := activateGeneration(venvPath); err != nil {
		return err
	}
	activated = true

	previousCommands := []string(nil)
	if state.Manifest != nil {
		previousCommands = state.Manifest.ExportedCommands
	}
	if err := removeStaleWrappers(previousCommands, commands); err != nil {
		return fmt.Errorf("remove stale Ansible command wrappers: %w", err)
	}
	if err := cleanupGenerations(); err != nil {
		return fmt.Errorf("clean old Ansible generations: %w", err)
	}
	return ensureManagedOwnership(options.SaltboxUser)
}

func environmentStatusTaskSpec(healthy bool, options Options) terminal.TaskSpec {
	success := "Ansible virtual environment update required"
	if healthy && !options.ForceVenv && !options.ForcePython {
		success = "Ansible virtual environment was already up to date"
	} else if options.ForcePython {
		success = "Python and Ansible virtual environment recreation requested"
	} else if options.ForceVenv {
		success = "Ansible virtual environment recreation requested"
	}
	return terminal.TaskSpec{
		Running: "Checking Ansible virtual environment for updates",
		Success: success,
	}
}

func createPythonRelease(ctx context.Context, task *terminal.Task, version string, options Options) (string, string, error) {
	id := "python-" + strings.ReplaceAll(version, ".", "-") + "-" + time.Now().UTC().Format("20060102T150405.000000000")
	installDir := filepath.Join(layout.PythonReleasesPath, id)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", "", fmt.Errorf("create Python release directory: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(installDir)
		}
	}()
	if err := task.RunStreaming(ctx, terminal.TaskSpec{Running: fmt.Sprintf("Installing Python %s", version)}, func(taskCtx context.Context) error {
		return InstallPythonAt(taskCtx, version, installDir, options.ForcePython, options.noCache(), options.Verbose)
	}); err != nil {
		return "", "", fmt.Errorf("install Python release: %w", err)
	}
	pythonPath, err := FindPythonAt(ctx, version, installDir)
	if err != nil {
		return "", "", err
	}
	if err := validatePython(ctx, pythonPath, version); err != nil {
		return "", "", err
	}
	success = true
	return installDir, pythonPath, nil
}

func inspectActive(ctx context.Context, pythonVersion, lockDigest string) (activeState, error) {
	activePath := filepath.Join(layout.AnsibleVenvPath, "venv")
	info, err := os.Lstat(activePath)
	if errors.Is(err, os.ErrNotExist) {
		return activeState{}, nil
	}
	if err != nil {
		return activeState{}, err
	}
	if info.IsDir() {
		return activeState{VenvPath: activePath}, nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return activeState{}, fmt.Errorf("%s is neither a directory nor a symlink", activePath)
	}
	target, err := filepath.EvalSymlinks(activePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return activeState{}, nil
		}
		return activeState{}, err
	}
	generationDir := filepath.Dir(target)
	manifest, err := readManifest(generationDir)
	if err != nil {
		return activeState{}, err
	}
	state := activeState{Manifest: manifest, GenerationDir: generationDir, VenvPath: target}
	if manifest.SchemaVersion != manifestSchemaVersion || manifest.PythonVersion != pythonVersion || manifest.LockSHA256 != lockDigest {
		return state, nil
	}
	if err := validateEnvironment(ctx, target, pythonVersion); err != nil {
		return state, nil
	}
	state.Healthy = true
	return state, nil
}

func validatePython(ctx context.Context, path, version string) error {
	if path == "" {
		return fmt.Errorf("python path is empty")
	}
	result, err := executor.Run(ctx, path, executor.WithArgs("--version"))
	if err != nil {
		return fmt.Errorf("run Python at %s: %w", path, err)
	}
	if got := strings.TrimSpace(string(result.Combined)); got != "Python "+version {
		return fmt.Errorf("python at %s reports %q, expected %q", path, got, "Python "+version)
	}
	_, err = executor.Run(ctx, path, executor.WithArgs("-c", "import encodings, sys; sys.exit(0)"))
	if err != nil {
		return fmt.Errorf("python at %s cannot import its standard library: %w", path, err)
	}
	return nil
}

func validateEnvironment(ctx context.Context, venvPath, version string) error {
	pythonPath := filepath.Join(venvPath, "bin", "python3")
	if err := validatePython(ctx, pythonPath, version); err != nil {
		return err
	}
	if err := CheckPackages(ctx, pythonPath); err != nil {
		return err
	}
	if _, err := executor.Run(ctx, pythonPath, executor.WithArgs("-c", "import ansible, apprise, certbot")); err != nil {
		return fmt.Errorf("import Saltbox Python packages: %w", err)
	}
	return validateEntrypoints(ctx, venvPath)
}

func validateEntrypoints(ctx context.Context, venvPath string) error {
	checks := [][]string{
		{filepath.Join(venvPath, "bin", "ansible"), "--version"},
		{filepath.Join(venvPath, "bin", "certbot"), "--version"},
		{filepath.Join(venvPath, "bin", "apprise"), "--version"},
	}
	commandPath := filepath.Join(venvPath, "bin")
	if currentPath := os.Getenv("PATH"); currentPath != "" {
		commandPath += string(os.PathListSeparator) + currentPath
	}
	for _, check := range checks {
		if _, err := executor.Run(
			ctx,
			check[0],
			executor.WithArgs(check[1:]...),
			executor.WithInheritEnv("PATH="+commandPath),
			executor.WithWorkingDir(venvPath),
		); err != nil {
			return fmt.Errorf("run %s health check: %w", filepath.Base(check[0]), err)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func saltboxCommit(ctx context.Context) (string, error) {
	result, err := executor.Run(ctx, "git",
		executor.WithArgs("rev-parse", "HEAD"),
		executor.WithWorkingDir(layout.SaltboxRepoPath),
	)
	if err != nil {
		return "", fmt.Errorf("read Saltbox commit: %w", err)
	}
	return strings.TrimSpace(string(result.Combined)), nil
}

func generationID(lockDigest string) string {
	return time.Now().UTC().Format("20060102T150405.000000000") + "-" + lockDigest[:12]
}

func writeManifest(generationDir string, manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode generation manifest: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(generationDir, manifestFileName)
	temporary, err := os.CreateTemp(generationDir, ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("create generation manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write generation manifest: %w", err)
	}
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("chmod generation manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close generation manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate generation manifest: %w", err)
	}
	return nil
}

func readManifest(generationDir string) (*Manifest, error) {
	path := filepath.Join(generationDir, manifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read generation manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse generation manifest %s: %w", path, err)
	}
	return &manifest, nil
}

func discoverCommands(venvPath string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(venvPath, "bin"))
	if err != nil {
		return nil, fmt.Errorf("read venv commands: %w", err)
	}
	commands := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ansible") && name != "certbot" && name != "apprise" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect venv command %s: %w", name, err)
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			commands = append(commands, name)
		}
	}
	sort.Strings(commands)
	for _, required := range []string{"ansible", "certbot", "apprise"} {
		if !contains(commands, required) {
			return nil, fmt.Errorf("required command %s is missing from staged venv", required)
		}
	}
	return commands, nil
}

func contains(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

func ensureManagedOwnership(owner string) error {
	if err := host.EnsureForExistingUser(owner, layout.SaltboxGitPath); err != nil {
		return err
	}
	return host.EnsureRecursiveForExistingUser(
		owner,
		layout.SaltboxRepoPath,
		layout.AnsibleVenvPath,
		layout.PythonInstallDir,
	)
}
