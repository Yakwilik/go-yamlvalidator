package yamlvalidator_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestCompileJSONSchemaCustomContentEncoding(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"type":"string",
		"contentEncoding":"digits"
	}`), JSONSchemaCompileOptions{
		AssertContent: true,
		ContentEncodings: []JSONSchemaContentEncoding{{
			Name: "digits",
			Decode: func(value string) ([]byte, error) {
				for _, ch := range value {
					if ch < '0' || ch > '9' {
						return nil, fmt.Errorf("non-digit content")
					}
				}
				return []byte(value), nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result := NewValidator(schema).ValidateBytes([]byte(`"123"`)); result.HasErrors() {
		t.Fatalf("valid custom encoding rejected: %v", result.Collector.Errors())
	}
	if result := NewValidator(schema).ValidateBytes([]byte(`"12x"`)); !containsDiagnosticCode(result, "contentEncoding") {
		t.Fatalf("invalid custom encoding accepted: %v", result.Collector.Errors())
	}
}

func TestCompileJSONSchemaCustomContentMediaType(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"type":"string",
		"contentMediaType":"application/x-upper"
	}`), JSONSchemaCompileOptions{
		AssertContent: true,
		ContentMediaTypes: []JSONSchemaContentMediaType{{
			Name: "application/x-upper",
			Validate: func(value []byte) error {
				text := string(value)
				if text != strings.ToUpper(text) {
					return fmt.Errorf("content must be uppercase")
				}
				return nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result := NewValidator(schema).ValidateBytes([]byte(`"HELLO"`)); result.HasErrors() {
		t.Fatalf("valid media type content rejected: %v", result.Collector.Errors())
	}
	if result := NewValidator(schema).ValidateBytes([]byte(`"Hello"`)); !containsDiagnosticCode(result, "contentMediaType") {
		t.Fatalf("invalid media type content accepted: %v", result.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsInvalidContentExtensions(t *testing.T) {
	tests := []struct {
		name string
		opts JSONSchemaCompileOptions
		part string
	}{
		{
			name: "encoding empty name",
			opts: JSONSchemaCompileOptions{
				ContentEncodings: []JSONSchemaContentEncoding{{Decode: func(string) ([]byte, error) { return nil, nil }}},
			},
			part: "name must not be empty",
		},
		{
			name: "encoding nil decoder",
			opts: JSONSchemaCompileOptions{
				ContentEncodings: []JSONSchemaContentEncoding{{Name: "x"}},
			},
			part: "decode function is nil",
		},
		{
			name: "media type nil validator",
			opts: JSONSchemaCompileOptions{
				ContentMediaTypes: []JSONSchemaContentMediaType{{Name: "application/x-test"}},
			},
			part: "validate function is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileJSONSchemaWithOptions([]byte(`true`), tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("expected error containing %q, got %v", tt.part, err)
			}
		})
	}
}
