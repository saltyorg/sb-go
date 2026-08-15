package layout

import (
	"path/filepath"
	"strings"
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
	SaltboxFactPath                   = "/srv/git/saltbox/ansible_facts.d/saltbox.fact"
	SaltboxCacheFile                  = "/srv/git/saltbox/cache.json"
	AnsibleVenvPath                   = "/srv/ansible"
	AnsibleReleasesPath               = "/srv/ansible/releases"
	AnsibleRequirementsPath           = "/srv/git/saltbox/requirements/requirements-saltbox.txt"
	SaltboxPythonVersionPath          = "/srv/git/saltbox/.python-version"
	SaltboxUVVersionPath              = "/srv/git/saltbox/.uv-version"
	PythonInstallDir                  = "/srv/python"
	PythonReleasesPath                = "/srv/python/releases"
	SupportedUbuntuReleases           = "22.04,24.04"
	DockerControllerServiceFile       = "/etc/systemd/system/saltbox_managed_docker_controller.service"
	DockerControllerAPIURL            = "http://127.0.0.1:3377"
	SVMVersionProxyURL                = "https://svm.saltbox.dev/version"
)

func SaltboxPlaybookPath() string {
	return SaltboxRepoPath + "/saltbox.yml"
}

func SandboxPlaybookPath() string {
	return Current().SandboxRepoPath + "/sandbox.yml"
}

func SaltboxModPlaybookPath() string {
	return Current().SaltboxModRepoPath + "/saltbox_mod.yml"
}

// AnsibleVenvPythonPath returns the full path to the Python binary in the Ansible virtual environment.
func AnsibleVenvPythonPath() string {
	return filepath.Join(AnsibleVenvPath, "venv", "bin", "python3")
}

// GetSupportedUbuntuReleases returns a slice of supported Ubuntu release codenames.
func GetSupportedUbuntuReleases() []string {
	return strings.Split(SupportedUbuntuReleases, ",")
}
