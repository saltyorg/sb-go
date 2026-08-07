package toolchain

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/saltyorg/sb-go/internal/constants"
)

var exactVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Config struct {
	Python      string
	PythonMinor string
	MinimumUV   string
}

func Load() (Config, error) {
	return LoadFiles(constants.SaltboxPythonVersionPath, constants.SaltboxUVVersionPath)
}

func LoadFiles(pythonPath, uvPath string) (Config, error) {
	pythonVersion, err := readExactVersion(pythonPath, "Python")
	if err != nil {
		return Config{}, err
	}
	uvVersion, err := readExactVersion(uvPath, "uv")
	if err != nil {
		return Config{}, err
	}
	parts := strings.Split(pythonVersion, ".")
	return Config{
		Python:      pythonVersion,
		PythonMinor: strings.Join(parts[:2], "."),
		MinimumUV:   uvVersion,
	}, nil
}

func readExactVersion(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s version from %s: %w", name, path, err)
	}
	value := strings.TrimSpace(string(data))
	if !exactVersionPattern.MatchString(value) {
		return "", fmt.Errorf("%s must contain one exact major.minor.patch version, got %q", path, value)
	}
	return value, nil
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
