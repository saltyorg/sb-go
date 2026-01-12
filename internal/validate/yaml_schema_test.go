package validate

import "testing"

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
