package layout

import "testing"

func TestSaltboxLayoutContract(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"repository URL": {SaltboxRepoURL, "https://github.com/saltyorg/saltbox.git"},
		"cache":          {SaltboxCacheFile, "/srv/git/saltbox/cache.json"},
		"requirements":   {AnsibleRequirementsPath, "/srv/git/saltbox/requirements/requirements-saltbox.txt"},
		"playbook":       {SaltboxPlaybookPath(), "/srv/git/saltbox/saltbox.yml"},
		"providers":      {SaltboxProvidersConfigPath, "/srv/git/saltbox/providers.yml"},
		"version proxy":  {SVMVersionProxyURL, "https://svm.saltbox.dev/version"},
		"facts binary":   {SaltboxFactPath, "/srv/git/saltbox/ansible_facts.d/saltbox.fact"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}
