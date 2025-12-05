package constants

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/saltyorg/sb-go/internal/paths"
)

const (
	AnsiblePlaybookBinaryPath         = "/usr/local/bin/ansible-playbook"
	SaltboxGitPath                    = "/srv/git"
	SaltboxRepoPath                   = "/srv/git/saltbox"
	SaltboxRepoURL                    = "https://github.com/saltyorg/saltbox.git"
	SaltboxAccountsConfigPath         = "/srv/git/saltbox/accounts.yml"
	SaltboxAdvancedSettingsConfigPath = "/srv/git/saltbox/adv_settings.yml"
	SaltboxBackupConfigPath           = "/srv/git/saltbox/backup_config.yml"
	SaltboxHetznerVLANConfigPath      = "/srv/git/saltbox/hetzner_vlan.yml"
	SaltboxSettingsConfigPath         = "/srv/git/saltbox/settings.yml"
	SaltboxMOTDConfigPath             = "/srv/git/saltbox/motd.yml"
	SaltboxMOTDSchemaPath             = "/srv/git/saltbox/schema/motd.schema.yml"
	SaltboxInventoryConfigPath        = "/srv/git/saltbox/inventories/host_vars/localhost.yml"
	SaltboxCacheFile                  = "/srv/git/saltbox/cache.json"
	AnsibleVenvPath                   = "/srv/ansible"
	AnsibleRequirementsPath           = "/srv/git/saltbox/requirements/requirements-saltbox.txt"
	AnsibleVenvPythonVersion          = "3.12"
	PythonInstallDir                  = "/srv/python"
	SupportedUbuntuReleases           = "22.04,24.04"
	DockerControllerServiceFile       = "/etc/systemd/system/saltbox_managed_docker_controller.service"
	DockerControllerAPIURL            = "http://127.0.0.1:3377"
)

// These paths are configurable based on server_appdata_path from inventory.
// They default to /opt but can be overridden via inventories/host_vars/localhost.yml.
// They are aliased from the paths package to maintain backward compatibility.
var (
	SaltboxFactsPath   = paths.SaltboxFactsPath
	SandboxRepoPath    = paths.SandboxRepoPath
	SaltboxModRepoPath = paths.SaltboxModRepoPath
)

func SaltboxPlaybookPath() string {
	return SaltboxRepoPath + "/saltbox.yml"
}

func SandboxPlaybookPath() string {
	return SandboxRepoPath + "/sandbox.yml"
}

func SaltboxModPlaybookPath() string {
	return SaltboxModRepoPath + "/saltbox_mod.yml"
}

// AnsibleVenvPythonPath returns the full path to the Python binary in the Ansible virtual environment.
func AnsibleVenvPythonPath() string {
	return filepath.Join(AnsibleVenvPath, "venv", "bin", fmt.Sprintf("python%s", AnsibleVenvPythonVersion))
}

// GetSupportedUbuntuReleases returns a slice of supported Ubuntu release codenames.
func GetSupportedUbuntuReleases() []string {
	return strings.Split(SupportedUbuntuReleases, ",")
}
