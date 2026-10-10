// Package completion installs shared shell completions for installed shells.
package completion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

// Shell identifies a supported completion engine.
type Shell string

const (
	Bash Shell = "bash"
	Zsh  Shell = "zsh"
	Fish Shell = "fish"
)

// SupportedShells returns the engines supported by the installer.
func SupportedShells() []Shell { return []Shell{Bash, Zsh, Fish} }

// Generator renders native completion for an executable name.
type Generator func(Shell, string) ([]byte, error)

type Status string

const (
	Installed Status = "installed"
	Skipped   Status = "skipped"
	Failed    Status = "failed"
	Cleaned   Status = "cleaned"
)

// Result describes one installation, skipped engine, or obsolete file cleanup.
type Result struct {
	Shell   Shell
	Name    string
	Path    string
	Status  Status
	Reason  string
	Removed []string
}

// Paths separates administrator autoload directories from historical outputs.
type Paths struct {
	BashDir       string
	ZshDir        string
	BashLegacyDir string
	ZshLegacyDir  string
}

// Detection reports a usable shell and its administrator completion directory.
// Present may be true with Reason set when a dependency or supported version is missing.
type Detection struct {
	Present   bool
	Reason    string
	Directory string
}

// Probe detects a shell without evaluating user configuration.
type Probe func(context.Context, Shell) (Detection, error)

// Manager replaces completion files for active names and retires old outputs.
// Paths and Probe can be replaced for isolated verification.
type Manager struct {
	Paths     Paths
	Probe     Probe
	NameInUse func(string) (bool, error)
}

// NewManager uses the shared loading paths on supported Ubuntu installations.
func NewManager() *Manager {
	return &Manager{Paths: Paths{
		BashDir:       "/usr/local/share/bash-completion/completions",
		ZshDir:        "/usr/local/share/zsh/site-functions",
		BashLegacyDir: "/etc/bash_completion.d",
		ZshLegacyDir:  "/usr/share/zsh/vendor-completions",
	}}
}

var commandName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Install reconciles the selected installed shells and their active executable names.
// Failures are collected while independent shells and names continue processing.
func (m *Manager) Install(ctx context.Context, shells []Shell, names []string, generate Generator) ([]Result, error) {
	var results []Result
	if generate == nil {
		return nil, errors.New("completion generator is required")
	}
	names = slices.Clone(names)
	slices.Sort(names)
	names = slices.Compact(names)
	if len(names) == 0 {
		return nil, errors.New("at least one executable name is required")
	}
	for _, name := range names {
		if !commandName.MatchString(name) || name == "." || name == ".." {
			return nil, fmt.Errorf("invalid executable name %q", name)
		}
	}
	var failures []error
	seen := make(map[Shell]bool)
	for _, shell := range shells {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}
		if seen[shell] {
			continue
		}
		seen[shell] = true
		shellNames := names
		if !slices.Contains(SupportedShells(), shell) {
			failures = append(failures, fmt.Errorf("unsupported completion shell %q", shell))
			results = append(results, Result{Shell: shell, Status: Failed, Reason: "unsupported shell"})
			continue
		}
		cleanup := func(directory string) {
			if directory == "" {
				return
			}
			if err := ctx.Err(); err != nil {
				failures = append(failures, err)
				results = append(results, Result{Shell: shell, Status: Failed, Reason: err.Error()})
				return
			}
			cleaned, cleanupErr := m.cleanupObsolete(shell, shellNames, directory)
			results = append(results, cleaned...)
			if cleanupErr != nil {
				failures = append(failures, cleanupErr)
				results = append(results, Result{Shell: shell, Status: Failed, Reason: cleanupErr.Error()})
			}
		}
		probe := m.Probe
		if probe == nil {
			probe = m.detect
		}
		detection, err := probe(ctx, shell)
		if err != nil {
			failures = append(failures, fmt.Errorf("detect %s: %w", shell, err))
			results = append(results, Result{Shell: shell, Status: Failed, Reason: err.Error()})
			cleanup(m.directory(shell, Detection{}))
			continue
		}
		if !detection.Present || detection.Reason != "" {
			reason := detection.Reason
			if reason == "" {
				reason = "shell is not installed"
			}
			results = append(results, Result{Shell: shell, Status: Skipped, Reason: reason})
			cleanup(m.directory(shell, detection))
			continue
		}
		directory := m.directory(shell, detection)
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
			err = fmt.Errorf("invalid %s completion directory %q", shell, directory)
			failures = append(failures, err)
			results = append(results, Result{Shell: shell, Status: Failed, Reason: err.Error()})
			continue
		}
		shellGenerator := generate
		if shell == Bash {
			bashNames, err := m.bashAliasNames(directory, shellNames)
			if err != nil {
				failures = append(failures, err)
				results = append(results, Result{Shell: shell, Status: Failed, Reason: err.Error()})
				cleanup(directory)
				continue
			}
			shellNames = bashNames
			bundle, err := bashCompletionBundle(ctx, shellNames, generate)
			if err != nil {
				failures = append(failures, err)
				results = append(results, Result{Shell: shell, Status: Failed, Reason: err.Error()})
				cleanup(directory)
				continue
			}
			// Loader versions disagree on bare-name versus .bash precedence.
			// Every alias file registers all names using unchanged native output.
			shellGenerator = func(Shell, string) ([]byte, error) { return bundle, nil }
		}
		for _, name := range shellNames {
			result, installErr := m.installOne(ctx, shell, name, directory, shellNames, shellGenerator)
			results = append(results, result)
			if installErr != nil {
				failures = append(failures, installErr)
			}
		}
		// Active registrations remain until replaced; obsolete aliases have no consumer.
		cleanup(directory)
	}
	return results, errors.Join(failures...)
}

func (m *Manager) bashAliasNames(directory string, names []string) ([]string, error) {
	// Separate live builds can share a loader filename through a .bash suffix.
	// Refresh only their identified sb-go outputs together, in either direction.
	names = slices.Clone(names)
	for i := 0; i < len(names); i++ {
		for _, candidate := range []string{trimSuffix(names[i], ".bash"), names[i] + ".bash"} {
			if candidate == "" || slices.Contains(names, candidate) {
				continue
			}
			live, err := m.nameInUse(candidate)
			if err != nil {
				return nil, err
			}
			if !live {
				continue
			}
			path := filepath.Join(directory, fileName(Bash, candidate))
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !identifiesCompletion(data, Bash, candidate) {
				return nil, fmt.Errorf("bash completion filename collision between %s and %s: %s belongs to another command", names[i], candidate, path)
			}
			names = append(names, candidate)
		}
	}
	slices.Sort(names)
	return names, nil
}

func bashCompletionBundle(ctx context.Context, names []string, generate Generator) ([]byte, error) {
	var bundle []byte
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, err := generate(Bash, name)
		if err == nil && len(body) == 0 {
			err = errors.New("generator returned an empty completion")
		}
		if err != nil {
			return nil, fmt.Errorf("generate bash completion for %s: %w", name, err)
		}
		bundle = append(bundle, body...)
	}
	return bundle, nil
}

func (m *Manager) directory(shell Shell, detection Detection) string {
	switch shell {
	case Bash:
		return m.Paths.BashDir
	case Zsh:
		return m.Paths.ZshDir
	default:
		return detection.Directory
	}
}

func fileName(shell Shell, name string) string {
	switch shell {
	case Bash:
		return name + ".bash"
	case Zsh:
		return "_" + name
	default:
		return name + ".fish"
	}
}

func (m *Manager) legacyPath(shell Shell, name string) string {
	switch shell {
	case Bash:
		return filepath.Join(m.Paths.BashLegacyDir, name)
	case Zsh:
		return filepath.Join(m.Paths.ZshLegacyDir, "_"+name)
	default:
		return ""
	}
}

func (m *Manager) installOne(ctx context.Context, shell Shell, name, directory string, names []string, generate Generator) (Result, error) {
	path := filepath.Join(directory, fileName(shell, name))
	result := Result{Shell: shell, Name: name, Path: path, Status: Failed}
	fail := func(err error) (Result, error) {
		result.Reason = err.Error()
		return result, fmt.Errorf("install %s completion for %s: %w", shell, name, err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	// Render before touching either destination or historical output.
	body, err := generate(shell, name)
	if err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if len(body) == 0 {
		return fail(errors.New("generator returned an empty completion"))
	}
	var migration []string
	if legacy := m.legacyPath(shell, name); legacy != "" {
		migration = append(migration, legacy)
	}
	if shell == Bash && !slices.Contains(names, trimSuffix(name, ".bash")) {
		// A bare alias path can also be another active name's .bash file.
		bare := filepath.Join(directory, name)
		migration = append(migration, bare)
	}
	if err := ensureDirectory(directory); err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := m.atomicWrite(path, completionContent(shell, name, body)); err != nil {
		return fail(err)
	}
	for _, old := range migration {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		removed, err := removeCompletion(old)
		if err != nil {
			return fail(err)
		}
		if removed {
			result.Removed = append(result.Removed, old)
		}
	}
	result.Status = Installed
	return result, nil
}

func (m *Manager) cleanupObsolete(shell Shell, names []string, directory string) ([]Result, error) {
	directories := []string{directory}
	if legacy := m.legacyPath(shell, "sb"); legacy != "" {
		directories = append(directories, filepath.Dir(legacy))
	}
	var results []Result
	var failures []error
	for i, dir := range directories {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			failures = append(failures, fmt.Errorf("inspect obsolete %s completions: %w", shell, err))
			continue
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			name := aliasFromFile(shell, entry.Name(), i == 1)
			if !commandName.MatchString(name) || slices.Contains(names, name) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if !identifiesCompletion(data, shell, name) {
				continue
			}
			live, err := m.nameInUse(name)
			if err != nil {
				failures = append(failures, fmt.Errorf("check obsolete completion executable %s: %w", name, err))
				continue
			}
			if live {
				results = append(results, Result{Shell: shell, Name: name, Path: path, Status: Skipped, Reason: "preserved completion for a live executable outside this installation"})
				continue
			}
			removed, err := removeCompletion(path)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if removed {
				results = append(results, Result{Shell: shell, Name: name, Path: path, Status: Cleaned, Removed: []string{path}})
			}
		}
	}
	return results, errors.Join(failures...)
}

func (m *Manager) nameInUse(name string) (bool, error) {
	if m.NameInUse != nil {
		return m.NameInUse(name)
	}
	return executableNameInUse(name)
}

func aliasFromFile(shell Shell, file string, legacy bool) string {
	switch shell {
	case Bash:
		if legacy {
			return file
		}
		return trimSuffix(file, ".bash")
	case Zsh:
		if len(file) > 1 && file[0] == '_' {
			return file[1:]
		}
	case Fish:
		return trimSuffix(file, ".fish")
	}
	return ""
}

func trimSuffix(value, suffix string) string {
	if len(value) > len(suffix) && value[len(value)-len(suffix):] == suffix {
		return value[:len(value)-len(suffix)]
	}
	return ""
}
