package saltbox

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/terminal"
)

func TestVerboseSchemaValidationDoesNotLogCredentialValues(t *testing.T) {
	const (
		password        = "password-secret-sentinel"
		cloudflareAPI   = "cloudflare-api-secret-sentinel"
		cloudflareEmail = "cloudflare-login-sentinel@example.com"
		dockerhubUser   = "dockerhub-login-sentinel"
		dockerhubToken  = "dockerhub-token-secret-sentinel"
	)

	schema := &Schema{
		verbose: true,
		Rules: map[string]*SchemaRule{
			"user": {
				Type: "object",
				Properties: map[string]*SchemaRule{
					"pass": {Type: "password", NotEquals: "password1234"},
				},
			},
			"cloudflare": {
				Type: "object",
				Properties: map[string]*SchemaRule{
					"api":   {Type: "string"},
					"email": {Type: "string", Format: "email"},
				},
			},
			"dockerhub": {
				Type: "object",
				Properties: map[string]*SchemaRule{
					"user":  {Type: "string"},
					"token": {Type: "string"},
				},
			},
		},
	}
	config := map[string]any{
		"user":       map[string]any{"pass": password},
		"cloudflare": map[string]any{"api": cloudflareAPI, "email": cloudflareEmail},
		"dockerhub":  map[string]any{"user": dockerhubUser, "token": dockerhubToken},
	}

	output := captureStdout(t, func() {
		if err := schema.ValidateWithTypeFlexibility(config); err != nil {
			t.Fatal(err)
		}
	})
	for _, credential := range []string{password, cloudflareAPI, cloudflareEmail, dockerhubUser, dockerhubToken} {
		if strings.Contains(output, credential) {
			t.Fatalf("verbose validation output disclosed credential %q: %s", credential, output)
		}
	}
}

func TestAsyncPasswordWarningUsesTaskOutput(t *testing.T) {
	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"password": {
				Type:     "password",
				Required: true,
			},
		},
	}
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})
	err := runner.Run(context.Background(), terminal.TaskSpec{Running: "validating"}, func(ctx context.Context, task *terminal.Task) error {
		asyncCtx, err := schema.ValidateWithTypeFlexibilityAsync(ctx, task, map[string]any{"password": "pass123"})
		if err != nil {
			return err
		}
		if errors := asyncCtx.Wait(); len(errors) != 0 {
			return errors[0]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rendered := output.String(); !strings.Contains(rendered, "WARNING: Password is shorter than 12 characters (7).") {
		t.Fatalf("password warning was not routed through task output: %q", rendered)
	}
}

func TestAsyncRcloneWarningUsesTaskOutput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"remote": {
				Type:            "string",
				Required:        true,
				CustomValidator: "validate_rclone_remote",
			},
		},
	}
	var output bytes.Buffer
	runner := terminal.NewRunner(terminal.RunnerOptions{Verbose: true, Output: &output})
	err := runner.Run(context.Background(), terminal.TaskSpec{Running: "validating"}, func(ctx context.Context, task *terminal.Task) error {
		asyncCtx, err := schema.ValidateWithTypeFlexibilityAsync(ctx, task, map[string]any{"remote": "media:path"})
		if err != nil {
			return err
		}
		if errors := asyncCtx.Wait(); len(errors) != 0 {
			return errors[0]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rendered := output.String(); !strings.Contains(rendered, "Warning: rclone remote validation skipped: rclone is not installed") {
		t.Fatalf("rclone warning was not routed through task output: %q", rendered)
	}
}

func TestSchemaValidateWithTypeFlexibilityNumber(t *testing.T) {
	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"value": {
				Type:     "number",
				Required: true,
			},
		},
	}

	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "string number", value: "8080", wantErr: false},
		{name: "int number", value: 8080, wantErr: false},
		{name: "invalid string", value: "abc", wantErr: true},
		{name: "float value", value: 1.5, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithTypeFlexibility(map[string]any{"value": tt.value})
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for value %v, got none", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error for value %v, got: %v", tt.value, err)
			}
		})
	}
}

func TestSchemaValidateWithTypeFlexibilityFloat(t *testing.T) {
	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"value": {
				Type:     "float",
				Required: true,
			},
		},
	}

	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "string float", value: "1.25", wantErr: false},
		{name: "float value", value: 1.25, wantErr: false},
		{name: "integer value", value: 5, wantErr: true},
		{name: "invalid string", value: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithTypeFlexibility(map[string]any{"value": tt.value})
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for value %v, got none", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error for value %v, got: %v", tt.value, err)
			}
		})
	}
}

func TestSchemaValidateWithTypeFlexibilityRequiredWhenTrue(t *testing.T) {
	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"rclone": {
				Type:     "object",
				Required: true,
				Properties: map[string]*SchemaRule{
					"enabled": {
						Type:     "ansible_bool",
						Required: true,
					},
					"remotes": {
						Type:             "array",
						RequiredWhenTrue: []string{"enabled"},
						Items: &SchemaRule{
							Type: "string",
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name      string
		config    map[string]any
		errSubstr string
	}{
		{
			name: "enabled false does not require remotes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": false,
				},
			},
		},
		{
			name: "enabled false string does not require remotes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": "false",
				},
			},
		},
		{
			name: "enabled true requires remotes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": true,
				},
			},
			errSubstr: "field 'rclone.remotes' is required",
		},
		{
			name: "enabled yes requires remotes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": "yes",
				},
			},
			errSubstr: "field 'rclone.remotes' is required",
		},
		{
			name: "enabled true with remotes passes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": true,
					"remotes": []any{
						"remote1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithTypeFlexibility(tt.config)
			if tt.errSubstr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tt.errSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
			}
		})
	}
}

func TestSchemaValidateWithTypeFlexibilityValidateWhenTrue(t *testing.T) {
	schema := &Schema{
		Rules: map[string]*SchemaRule{
			"rclone": {
				Type:     "object",
				Required: true,
				Properties: map[string]*SchemaRule{
					"enabled": {
						Type:     "ansible_bool",
						Required: true,
					},
					"remotes": {
						Type:             "array",
						ValidateWhenTrue: []string{"enabled"},
						Items: &SchemaRule{
							Type: "object",
							Properties: map[string]*SchemaRule{
								"settings": {
									Type:     "object",
									Required: true,
									Properties: map[string]*SchemaRule{
										"template": {
											Type:     "rclone_template",
											Required: true,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name      string
		config    map[string]any
		errSubstr string
	}{
		{
			name: "enabled false skips remotes validation",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": false,
					"remotes": []any{
						map[string]any{
							"settings": map[string]any{
								"template": nil,
							},
						},
					},
				},
			},
		},
		{
			name: "enabled true validates remotes",
			config: map[string]any{
				"rclone": map[string]any{
					"enabled": true,
					"remotes": []any{
						map[string]any{
							"settings": map[string]any{
								"template": nil,
							},
						},
					},
				},
			},
			errSubstr: "field 'rclone.remotes[0].settings.template': rclone template must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.ValidateWithTypeFlexibility(tt.config)
			if tt.errSubstr == "" && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tt.errSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
			}
		})
	}
}
