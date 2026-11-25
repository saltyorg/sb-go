package python

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/saltyorg/sb-go/layout"
)

var exactVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// FallbackPythonMinor is the Python minor for a Saltbox checkout without
// .python-version, such as the pre-* branches that predate pinning. It is the
// minor sb-go used before Saltbox pinned its Python.
const FallbackPythonMinor = "3.12"

// Config is a Saltbox checkout's Python toolchain request. Python is empty
// when the checkout has no .python-version, and MinimumUV is empty when it
// has no .uv-version.
type Config struct {
	Python      string
	PythonMinor string
	MinimumUV   string
}

// Pinned reports whether the checkout pins an exact Python.
func (config Config) Pinned() bool {
	return config.Python != ""
}

// Toolchain is the exact Python to install for a checkout, as resolved
// against the Pythons this sb's uv can install.
type Toolchain struct {
	Python string
	// Hashed is true when the checkout pins its Python, and so carries a
	// hashed requirements lock.
	Hashed bool
	// Notice is the line to show when the checkout has no pinned Python.
	Notice string
}

func Load() (Config, error) {
	return loadFromRoot(layout.SaltboxRepoPath)
}

func loadFromRoot(root string) (Config, error) {
	return LoadFiles(filepath.Join(root, ".python-version"), filepath.Join(root, ".uv-version"))
}

// LoadFiles reads a checkout's Python pin and uv minimum. A missing
// .python-version falls back to FallbackPythonMinor, and a missing .uv-version
// sets no minimum; a file that is present must hold one exact version.
func LoadFiles(pythonPath, uvPath string) (Config, error) {
	pythonVersion, err := readOptionalExactVersion(pythonPath, "Python")
	if err != nil {
		return Config{}, err
	}
	uvVersion, err := readOptionalExactVersion(uvPath, "uv")
	if err != nil {
		return Config{}, err
	}
	config := Config{Python: pythonVersion, PythonMinor: FallbackPythonMinor, MinimumUV: uvVersion}
	if pythonVersion != "" {
		parts := strings.Split(pythonVersion, ".")
		config.PythonMinor = strings.Join(parts[:2], ".")
	}
	return config, nil
}

func readOptionalExactVersion(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s version from %s: %w", name, path, err)
	}
	value := strings.TrimSpace(string(data))
	if !exactVersionPattern.MatchString(value) {
		return "", fmt.Errorf("%s must contain one exact major.minor.patch version, got %q", path, value)
	}
	return value, nil
}

// checkMinimumUV refuses a uv older than the checkout's .uv-version. A
// checkout without .uv-version sets no minimum.
func checkMinimumUV(config Config, uvVersion string) error {
	if config.MinimumUV == "" {
		return nil
	}
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
	return nil
}

func resolveToolchain(ctx context.Context, config Config, uvVersion string) (Toolchain, error) {
	return resolveToolchainWithRuntime(ctx, defaultManagedRuntime(), config, uvVersion)
}

// resolveToolchainWithRuntime picks the exact Python from the ones the
// installed uv can install, before anything is installed. A pin uv cannot
// install is refused; without a pin, the newest FallbackPythonMinor patch is
// used and the requirements are installed unhashed.
func resolveToolchainWithRuntime(ctx context.Context, runtime *managedRuntime, config Config, uvVersion string) (Toolchain, error) {
	available, err := listPythonDownloadsWithRuntime(ctx, runtime, uvVersion)
	if err != nil {
		return Toolchain{}, err
	}
	if config.Pinned() {
		if slices.Contains(available, config.Python) {
			return Toolchain{Python: config.Python, Hashed: true}, nil
		}
		return Toolchain{}, fmt.Errorf(
			"this Saltbox branch needs Python %s, which this sb's uv (%s) can't install; run sb self-update",
			config.Python,
			uvVersion,
		)
	}
	newest := ""
	var newestParts [3]int
	for _, version := range available {
		parts, err := numericVersion(version)
		if err != nil || fmt.Sprintf("%d.%d", parts[0], parts[1]) != config.PythonMinor {
			continue
		}
		if newest == "" || parts[2] > newestParts[2] {
			newest, newestParts = version, parts
		}
	}
	if newest == "" {
		return Toolchain{}, fmt.Errorf(
			"this Saltbox branch has no pinned Python, and this sb's uv (%s) offers no Python %s; run sb self-update",
			uvVersion,
			config.PythonMinor,
		)
	}
	return Toolchain{
		Python: newest,
		Notice: fmt.Sprintf(
			"This Saltbox branch has no pinned Python; using Python %s, the newest %s this sb's uv (%s) provides",
			newest,
			config.PythonMinor,
			uvVersion,
		),
	}, nil
}

func AtLeast(actual, minimum string) (bool, error) {
	actualParts, err := numericVersion(actual)
	if err != nil {
		return false, fmt.Errorf("invalid actual version: %w", err)
	}
	minimumParts, err := numericVersion(minimum)
	if err != nil {
		return false, fmt.Errorf("invalid minimum version: %w", err)
	}
	for index := range actualParts {
		if actualParts[index] != minimumParts[index] {
			return actualParts[index] > minimumParts[index], nil
		}
	}
	return true, nil
}

func numericVersion(value string) ([3]int, error) {
	var parsed [3]int
	if !exactVersionPattern.MatchString(value) {
		return parsed, fmt.Errorf("expected major.minor.patch, got %q", value)
	}
	for index, part := range strings.Split(value, ".") {
		number, err := strconv.Atoi(part)
		if err != nil {
			return parsed, fmt.Errorf("parse %q: %w", value, err)
		}
		parsed[index] = number
	}
	return parsed, nil
}
