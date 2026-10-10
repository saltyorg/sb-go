package completion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const probeTimeout = 5 * time.Second

const (
	zshProbeCommand  = `printf '%s\n' "${fpath[@]}"`
	fishProbeCommand = `printf '%s\n%s\n' "$version" "$__fish_sysconf_dir"`
)

func (m *Manager) detect(ctx context.Context, shell Shell) (Detection, error) {
	executable, err := m.shellExecutable(string(shell))
	if err != nil || executable == "" {
		return Detection{}, err
	}
	detection := Detection{Present: true}
	switch shell {
	case Bash:
		const framework = "/usr/share/bash-completion/bash_completion"
		info, err := os.Stat(framework)
		if errors.Is(err, os.ErrNotExist) {
			detection.Reason = "bash-completion is not installed"
			return detection, nil
		}
		if err != nil {
			return detection, fmt.Errorf("inspect bash-completion: %w", err)
		}
		if !info.Mode().IsRegular() {
			return detection, errors.New("bash-completion framework is not a regular file")
		}
	case Zsh:
		output, err := runProbe(ctx, executable, "-d", "-f", "-c", zshProbeCommand)
		if err != nil {
			return detection, err
		}
		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		if !slices.Contains(lines, m.Paths.ZshDir) {
			detection.Reason = fmt.Sprintf("Zsh's clean function path does not contain %s; add it to the administrator fpath before installing", m.Paths.ZshDir)
		}
	case Fish:
		output, err := runProbe(ctx, executable, "--no-config", "-c", fishProbeCommand)
		if err != nil {
			return detection, err
		}
		return fishMetadata(output)
	}
	return detection, nil
}

func fishMetadata(output string) (Detection, error) {
	detection := Detection{Present: true}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 {
		return detection, errors.New("fish returned malformed installation metadata")
	}
	if !versionPattern.MatchString(lines[0]) {
		return detection, fmt.Errorf("fish returned malformed version metadata %q", lines[0])
	}
	if !versionAtLeast(lines[0], 3, 3) {
		detection.Reason = "Fish 3.3 or newer is required for configuration-free detection"
		return detection, nil
	}
	sysconf := lines[1]
	if !filepath.IsAbs(sysconf) || filepath.Clean(sysconf) != sysconf || sysconf == "/" || strings.IndexByte(sysconf, 0) >= 0 {
		return detection, fmt.Errorf("fish returned an invalid system configuration directory %q", sysconf)
	}
	detection.Directory = filepath.Join(sysconf, "completions")
	return detection, nil
}

func (m *Manager) shellExecutable(name string) (string, error) {
	for _, directory := range []string{"/usr/local/bin", "/usr/bin", "/bin"} {
		candidate := filepath.Join(directory, name)
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect %s executable: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", fmt.Errorf("%s is not an executable regular file: %s", name, candidate)
		}
		return candidate, nil
	}
	return "", nil
}

// A different regular sb-go build, or a reused command name, can still need its
// shared completion. Preserve it without executing a binary to identify it.
func executableNameInUse(name string) (bool, error) {
	directories := []string{"/usr/local/bin", "/usr/bin", "/bin"}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		if !slices.Contains(directories, directory) {
			directories = append(directories, directory)
		}
	}
	for _, directory := range directories {
		info, err := os.Stat(filepath.Join(directory, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return true, nil
		}
	}
	return false, nil
}

func runProbe(ctx context.Context, executable string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/", "LC_ALL=C", "LANG=C"}
	cmd.WaitDelay = time.Second
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("shell probe interrupted: %w", ctx.Err())
		}
		return "", fmt.Errorf("shell probe failed: %w (%s)", err, strings.TrimSpace(output.String()))
	}
	return output.String(), nil
}

var versionPattern = regexp.MustCompile(`^([0-9]+)\.([0-9]+)(?:\.[0-9]+)?(?:[-(][A-Za-z0-9()._-]+)?$`)

func versionAtLeast(version string, minimumMajor, minimumMinor int) bool {
	parts := versionPattern.FindStringSubmatch(version)
	if len(parts) != 3 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[1])
	minor, minorErr := strconv.Atoi(parts[2])
	return majorErr == nil && minorErr == nil && (major > minimumMajor || major == minimumMajor && minor >= minimumMinor)
}
