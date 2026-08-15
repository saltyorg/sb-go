package host

import (
	"testing"

	"github.com/saltyorg/sb-go/layout"
)

func TestValidateUbuntuSupportRuntimePolicy(t *testing.T) {
	supportedVersions := layout.GetSupportedUbuntuRuntimeReleases()
	tests := []struct {
		name      string
		osName    string
		osRelease map[string]string
		wantError string
	}{
		{
			name:      "Jammy is supported",
			osName:    "linux",
			osRelease: map[string]string{"ID": "ubuntu", "VERSION_ID": "22.04"},
		},
		{
			name:      "Noble is supported",
			osName:    "linux",
			osRelease: map[string]string{"ID": "ubuntu", "VERSION_ID": "24.04"},
		},
		{
			name:      "Resolute is supported",
			osName:    "linux",
			osRelease: map[string]string{"ID": "ubuntu", "VERSION_ID": "26.04"},
		},
		{
			name:      "Focal is rejected with the exact policy",
			osName:    "linux",
			osRelease: map[string]string{"ID": "ubuntu", "VERSION_ID": "20.04"},
			wantError: "unsupported Ubuntu version (detected version: 20.04, supported versions: 22.04, 24.04, 26.04)",
		},
		{
			name:      "non-Ubuntu Linux is rejected",
			osName:    "linux",
			osRelease: map[string]string{"ID": "debian", "VERSION_ID": "13"},
			wantError: "not an Ubuntu distribution (detected ID: debian)",
		},
		{
			name:      "non-Linux OS is rejected",
			osName:    "freebsd",
			osRelease: map[string]string{"ID": "ubuntu", "VERSION_ID": "26.04"},
			wantError: "not running on Linux (detected OS: freebsd)",
		},
		{
			name:      "missing version is rejected",
			osName:    "linux",
			osRelease: map[string]string{"ID": "ubuntu"},
			wantError: "a Ubuntu version ID not found in /etc/os-release",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateUbuntuSupport(test.osName, test.osRelease, supportedVersions)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateUbuntuSupport() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("validateUbuntuSupport() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestValidateUbuntuSupportSetupPolicy(t *testing.T) {
	supportedVersions := layout.GetSupportedUbuntuSetupReleases()
	for _, test := range []struct {
		version   string
		wantError string
	}{
		{
			version:   "22.04",
			wantError: "unsupported Ubuntu version (detected version: 22.04, supported versions: 24.04, 26.04)",
		},
		{version: "24.04"},
		{version: "26.04"},
	} {
		t.Run(test.version, func(t *testing.T) {
			err := validateUbuntuSupport("linux", map[string]string{
				"ID":         "ubuntu",
				"VERSION_ID": test.version,
			}, supportedVersions)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("validateUbuntuSupport() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("validateUbuntuSupport() error = %v, want %q", err, test.wantError)
			}
		})
	}
}
