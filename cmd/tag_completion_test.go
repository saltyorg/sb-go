package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/layout"
	"github.com/spf13/cobra"
)

func TestCompleteInstallTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		args       []string
		toComplete string
		want       []string
	}{
		{
			name: "sorted unique tags", tags: []string{"sonarr", "plex", "sonarr", "radarr"},
			want: []string{"plex", "radarr", "sonarr"},
		},
		{
			name: "prefix", tags: []string{"sonarr", "plex", "sabnzbd"}, toComplete: "so",
			want: []string{"sonarr"},
		},
		{
			name: "comma prefix", tags: []string{"sonarr", "plex", "radarr"}, toComplete: "plex,ra",
			want: []string{"plex,radarr"},
		},
		{
			name: "several commas", tags: []string{"sonarr", "plex", "radarr"}, toComplete: "plex,radarr,",
			want: []string{"plex,radarr,sonarr"},
		},
		{
			name: "comma whitespace", tags: []string{"sonarr", "plex", "radarr"}, toComplete: "plex, so",
			want: []string{"plex, sonarr"},
		},
		{
			name: "previous arguments and current prefix", tags: []string{"sonarr", "plex", "radarr", "sandbox-plex"},
			args: []string{" plex ,sonarr", "sandbox-plex"}, toComplete: "radarr,",
		},
		{
			name: "repository prefixes are distinct", tags: []string{"plex", "sandbox-plex"}, args: []string{"plex"},
			want: []string{"sandbox-plex"},
		},
		{
			name: "shell quoting belongs to adapters", tags: []string{"with space", "quote\"tag", "apostrophe'tag", "$tag", "semi;tag", "tick`tag"},
			want: []string{"$tag", "apostrophe'tag", "quote\"tag", "semi;tag", "tick`tag", "with space"},
		},
		{
			name: "protocol controls and delimiters", tags: []string{"good", "\nnew", "tab\ttag", "null\x00tag", "del\x7ftag", "two,tags", "", "   "},
			want: []string{"good"},
		},
		{
			name: "control in typed prefix", tags: []string{"sonarr"}, toComplete: "plex\n,so",
		},
		{
			name: "raw single quote", tags: []string{"sonarr"}, toComplete: "'so",
		},
		{
			name: "raw double quote", tags: []string{"sonarr"}, toComplete: "\"so",
		},
		{
			name: "raw single quote before comma", tags: []string{"sonarr"}, toComplete: "'plex,so",
		},
		{
			name: "raw double quote before comma", tags: []string{"sonarr"}, toComplete: "\"plex,so",
		},
		{
			name: "raw single quote before several commas", tags: []string{"sonarr"}, toComplete: "'plex,radarr,so",
		},
		{
			name: "raw double quote before several commas", tags: []string{"sonarr"}, toComplete: "\"plex,radarr,so",
		},
		{name: "missing tags", toComplete: "plex,"},
		{name: "unmatched prefix", tags: []string{"sonarr", "plex"}, toComplete: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagsBefore := append([]string(nil), tt.tags...)
			got, directive := completeInstallTags(tt.tags, tt.args, tt.toComplete)
			if !slices.Equal(got, tt.want) {
				t.Errorf("suggestions = %q, want %q", got, tt.want)
			}
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want NoFileComp", directive)
			}
			if !slices.Equal(tt.tags, tagsBefore) {
				t.Errorf("completion mutated source tags: %q", tt.tags)
			}
		})
	}
}

func TestInstallCompletionProtocol(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		loadErr       error
		want          []string
		wantDirective cobra.ShellCompDirective
		wantLoads     int
	}{
		{
			name: "noun prefix", args: []string{"install", "so"}, want: []string{"sonarr"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp, wantLoads: 1,
		},
		{
			name: "comma noun", args: []string{"install", "plex,so"}, want: []string{"plex,sonarr"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp, wantLoads: 1,
		},
		{
			name: "quoted comma whitespace", args: []string{"install", "plex, so"}, want: []string{"plex, sonarr"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp, wantLoads: 1,
		},
		{
			name: "previous tags with parsed flags", args: []string{"install", "--extra-vars", "sonarr", "plex", ""},
			want: []string{"radarr", "sonarr"}, wantDirective: cobra.ShellCompDirectiveNoFileComp, wantLoads: 1,
		},
		{
			name: "long flags", args: []string{"install", "--ver"}, want: []string{"--verbose\tIncrease verbosity level (can be used multiple times, e.g. -vvv)"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name: "flags after noun", args: []string{"install", "plex", "--no"}, want: []string{"--no-cache\tSkip cache validation and always perform tag checks"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp,
		},
		{name: "extra variable value", args: []string{"install", "--extra-vars", "so"}},
		{name: "extra variable equals", args: []string{"install", "--extra-vars=so"}},
		{name: "skip tag value", args: []string{"install", "--skip-tags", "so"}},
		{name: "skip tags equals", args: []string{"install", "--skip-tags=so"}},
		{
			name: "corrupt cache still no files", args: []string{"install", "so"}, loadErr: errors.New("invalid cache JSON"),
			wantDirective: cobra.ShellCompDirectiveNoFileComp, wantLoads: 1,
		},
		{
			name: "flags unaffected by corrupt cache", args: []string{"install", "--ver"}, loadErr: errors.New("invalid cache JSON"),
			want:          []string{"--verbose\tIncrease verbosity level (can be used multiple times, e.g. -vvv)"},
			wantDirective: cobra.ShellCompDirectiveNoFileComp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loads := 0
			root := &cobra.Command{Use: "sb", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(newInstallCommandWithTagLoader(func() ([]string, error) {
				loads++
				return []string{"sonarr", "plex", "radarr"}, tt.loadErr
			}))
			var out, stderr bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&stderr)
			root.SetArgs(append([]string{"__complete"}, tt.args...))
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("completion request failed: %v\n%s", err, stderr.String())
			}
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			wantCode := fmt.Sprintf(":%d", tt.wantDirective)
			if len(lines) == 0 || lines[len(lines)-1] != wantCode {
				t.Fatalf("protocol output = %q, want final directive %q", out.String(), wantCode)
			}
			got := lines[:len(lines)-1]
			if !slices.Equal(got, tt.want) {
				t.Errorf("protocol candidates = %q, want %q", got, tt.want)
			}
			if loads != tt.wantLoads {
				t.Errorf("cache reads = %d, want %d", loads, tt.wantLoads)
			}
		})
	}
}

func TestCompletionCacheRead(t *testing.T) {
	valid, err := json.Marshal(map[string]any{
		layout.SaltboxRepoPath:              map[string]any{"tags": []string{"plex", "sonarr"}},
		layout.Current().SandboxRepoPath:    map[string]any{"tags": []any{" radarr ", 42, "sabnzbd", "", " ", "\tbad", "two,tags"}},
		layout.Current().SaltboxModRepoPath: map[string]any{"tags": []string{"custom"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		content []byte
		want    []string
		wantErr bool
	}{
		{name: "absent"},
		{name: "corrupt", content: []byte("broken JSON"), wantErr: true},
		{name: "empty", content: []byte("{}")},
		{name: "repository routing", content: valid, want: []string{"plex", "sonarr", "sandbox-radarr", "sandbox-sabnzbd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "cache.json")
			if tt.content != nil {
				if err := os.WriteFile(path, tt.content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := loadCompletionTagsFromFile(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("cache error = %v, want error %t", err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("cached suggestions = %q, want %q", got, tt.want)
			}
			data, err := os.ReadFile(path)
			if tt.content == nil {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("absent cache changed: content %q, error %v", data, err)
				}
			} else if err != nil || !bytes.Equal(data, tt.content) {
				t.Fatalf("cache changed: content %q, error %v", data, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			wantFiles := 0
			if tt.content != nil {
				wantFiles = 1
			}
			if len(entries) != wantFiles {
				t.Errorf("read created artifacts: %v", entries)
			}
		})
	}
}
