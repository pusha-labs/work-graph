package httpapi

import (
	"encoding/json"
	"testing"
)

func TestValidateModuleConfiguration(t *testing.T) {
	schema := json.RawMessage(`{"fields":[{"key":"method","type":"select","required":true,"options":["GET","POST"]},{"key":"url","type":"url","required":true},{"key":"timeout","type":"number","required":true}]}`)
	tests := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{"valid", map[string]any{"method": "POST", "url": "https://example.com", "timeout": float64(30)}, ""},
		{"missing required", map[string]any{"method": "POST", "timeout": float64(30)}, "url is required"},
		{"unsupported option", map[string]any{"method": "DELETE", "url": "https://example.com", "timeout": float64(30)}, "method has an unsupported value"},
		{"wrong type", map[string]any{"method": "GET", "url": "https://example.com", "timeout": "30"}, "timeout must be a number"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validateModuleConfiguration(schema, test.config); got != test.want {
				t.Fatalf("validateModuleConfiguration() = %q, want %q", got, test.want)
			}
		})
	}
}
