package cmd

import "testing"

func TestValidateFactCommandRejectsUnsafeOrIncompleteOperations(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		config factConfig
	}{
		{
			name:   "role path traversal",
			args:   []string{"../../etc/passwd"},
			config: factConfig{method: "load"},
		},
		{
			name:   "unknown method",
			args:   []string{"plex"},
			config: factConfig{method: "unknown"},
		},
		{
			name:   "save without keys",
			args:   []string{"plex", "default"},
			config: factConfig{method: "save"},
		},
		{
			name:   "key deletion without keys",
			args:   []string{"plex", "default"},
			config: factConfig{method: "delete", deleteType: "key"},
		},
		{
			name:   "unknown delete type",
			args:   []string{"plex", "default"},
			config: factConfig{method: "delete", deleteType: "unknown"},
		},
		{
			name:   "reserved default instance",
			args:   []string{"plex", "DEFAULT"},
			config: factConfig{method: "save", keyValues: []string{"token=value"}},
		},
		{
			name:   "save key with newline",
			args:   []string{"plex", "default"},
			config: factConfig{method: "save", keyValues: []string{"safe\ninjected=value"}},
		},
		{
			name:   "save key interpreted as hash comment",
			args:   []string{"plex", "default"},
			config: factConfig{method: "save", keyValues: []string{"  #hidden=value"}},
		},
		{
			name:   "save key interpreted as semicolon comment",
			args:   []string{"plex", "default"},
			config: factConfig{method: "save", keyValues: []string{";hidden=value"}},
		},
		{
			name:   "delete key with carriage return",
			args:   []string{"plex", "default"},
			config: factConfig{method: "delete", deleteType: "key", keyValues: []string{"unsafe\rkey"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateFactCommand(tt.args, &tt.config); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateFactCommandAcceptsSupportedOperations(t *testing.T) {
	tests := []struct {
		args   []string
		config factConfig
	}{
		{args: []string{"plex"}, config: factConfig{method: "load"}},
		{args: []string{"plex", "default"}, config: factConfig{method: "save", keyValues: []string{"token=value"}}},
		{args: []string{"plex"}, config: factConfig{method: "delete", deleteType: "role"}},
		{args: []string{"plex", "default"}, config: factConfig{method: "delete", deleteType: "instance"}},
		{args: []string{"plex", "default"}, config: factConfig{method: "delete", deleteType: "key", keyValues: []string{"token"}}},
	}

	for _, tt := range tests {
		if err := validateFactCommand(tt.args, &tt.config); err != nil {
			t.Fatalf("validate %#v: %v", tt, err)
		}
	}
}
