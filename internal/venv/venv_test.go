package venv

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requirements.txt")
	if err := os.WriteFile(path, []byte("saltbox\n"), 0644); err != nil {
		t.Fatal(err)
	}

	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "2b419ef6f04a92c1ddabebef7e9c2f87d95e26640cbd88d98d32a7dc69f6a7a8"
	if digest != expected {
		t.Fatalf("fileSHA256() = %q, want %q", digest, expected)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &Manifest{
		SchemaVersion:    manifestSchemaVersion,
		GenerationID:     "20260807T120000.000000000-0123456789ab",
		PythonVersion:    "3.12.13",
		PythonPath:       "/srv/python/releases/python-3-12-13/bin/python3.12",
		PythonInstall:    "/srv/python/releases/python-3-12-13",
		UVVersion:        "0.12.3",
		LockSHA256:       strings.Repeat("a", 64),
		SaltboxCommit:    strings.Repeat("b", 40),
		CreatedAt:        "2026-08-07T12:00:00Z",
		ExportedCommands: []string{"ansible", "ansible-playbook", "apprise", "certbot"},
	}
	if err := writeManifest(dir, want); err != nil {
		t.Fatal(err)
	}

	got, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readManifest() = %#v, want %#v", got, want)
	}
}

func TestDiscoverCommands(t *testing.T) {
	venvPath := t.TempDir()
	binPath := filepath.Join(venvPath, "bin")
	if err := os.Mkdir(binPath, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ansible", "ansible-playbook", "apprise", "certbot", "unrelated"} {
		if err := os.WriteFile(filepath.Join(binPath, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := discoverCommands(venvPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ansible", "ansible-playbook", "apprise", "certbot"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverCommands() = %v, want %v", got, want)
	}
}

func TestDiscoverCommandsRequiresEntrypoints(t *testing.T) {
	venvPath := t.TempDir()
	binPath := filepath.Join(venvPath, "bin")
	if err := os.Mkdir(binPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binPath, "ansible"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := discoverCommands(venvPath)
	if err == nil || !strings.Contains(err.Error(), "certbot") {
		t.Fatalf("discoverCommands() error = %v, want missing certbot", err)
	}
}

func TestValidateEntrypointsUsesVenvWorkingDirectory(t *testing.T) {
	venvPath := t.TempDir()
	binPath := filepath.Join(venvPath, "bin")
	if err := os.Mkdir(binPath, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXPECTED_VENV_CWD", venvPath)
	for _, name := range []string{"ansible", "certbot", "apprise"} {
		content := []byte("#!/bin/sh\n[ \"$PWD\" = \"$EXPECTED_VENV_CWD\" ]\n")
		if err := os.WriteFile(filepath.Join(binPath, name), content, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := validateEntrypoints(context.Background(), venvPath); err != nil {
		t.Fatalf("validateEntrypoints() error = %v", err)
	}
}

func TestGenerationIDIncludesLockDigest(t *testing.T) {
	digest := strings.Repeat("c", 64)
	id := generationID(digest)
	if !strings.HasSuffix(id, "-"+digest[:12]) {
		t.Fatalf("generationID() = %q, want digest suffix", id)
	}
}

func TestManagedWrapperSetsResolvedVenvPath(t *testing.T) {
	content, err := managedWrapperContent("ansible-lint")
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"venv_bin=$(/usr/bin/readlink -f /srv/ansible/venv/bin) || exit 1",
		"PATH=\"${venv_bin}${PATH:+:${PATH}}\"",
		"exec /srv/ansible/venv/bin/ansible-lint \"$@\"",
	} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("wrapper content does not contain %q:\n%s", expected, content)
		}
	}
}

func TestEnvironmentStatusTaskSpec(t *testing.T) {
	tests := []struct {
		name    string
		healthy bool
		options Options
		want    string
	}{
		{
			name:    "already current",
			healthy: true,
			want:    "Ansible virtual environment was already up to date",
		},
		{
			name: "update required",
			want: "Ansible virtual environment update required",
		},
		{
			name:    "forced venv recreation",
			healthy: true,
			options: Options{ForceVenv: true},
			want:    "Ansible virtual environment recreation requested",
		},
		{
			name:    "forced Python recreation",
			healthy: true,
			options: Options{ForceVenv: true, ForcePython: true},
			want:    "Python and Ansible virtual environment recreation requested",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := environmentStatusTaskSpec(test.healthy, test.options)
			if spec.Success != test.want {
				t.Fatalf("Success = %q, want %q", spec.Success, test.want)
			}
		})
	}
}
