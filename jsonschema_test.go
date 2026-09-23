package yamlvalidator_test

import (
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestCompileJSONSchemaObjectConstraints(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"count": {"type": "integer"}
		},
		"required": ["name"],
		"additionalProperties": false
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{name: "valid", yaml: "name: app\ncount: 2\n", wantErrors: 0},
		{name: "missing required", yaml: "count: 2\n", wantErrors: 1},
		{name: "unknown key", yaml: "name: app\nextra: true\n", wantErrors: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewValidator(schema).ValidateBytes([]byte(tt.yaml))
			if got := len(res.Collector.Errors()); got != tt.wantErrors {
				t.Fatalf("got %d errors, want %d: %v", got, tt.wantErrors, res.Collector.Errors())
			}
		})
	}
}

func TestCompileJSONSchemaCanDowngradeAdditionalPropertiesToWarning(t *testing.T) {
	policy := UnknownKeyWarn
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"additionalProperties": false
	}`), JSONSchemaCompileOptions{AdditionalPropertiesFalsePolicy: &policy})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte("name: app\nextra: true\n"))
	if len(res.Collector.Errors()) != 0 || len(res.Collector.Warnings()) != 1 {
		t.Fatalf("expected one warning and no errors, got errors=%v warnings=%v", res.Collector.Errors(), res.Collector.Warnings())
	}
}

func TestCompileJSONSchemaEasyPCombinators(t *testing.T) {
	schemaJSON := []byte(`{
		"type": "object",
		"properties": {
			"directory": {
				"oneOf": [
					{"type": "string"},
					{
						"type": "object",
						"properties": {"path": {"type": "string"}, "root": {"type": "string"}},
						"required": ["path"],
						"additionalProperties": false
					}
				]
			},
			"plugin": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"remote": {"type": "string"},
					"path": {"type": "string"},
					"command": {"type": "array", "items": {"type": "string"}}
				},
				"oneOf": [
					{"required": ["name"]},
					{"required": ["remote"]},
					{"required": ["path"]},
					{"required": ["command"]}
				],
				"additionalProperties": false
			},
			"managed": {
				"type": "object",
				"properties": {
					"file_option": {"type": "string"},
					"field_option": {"type": "string"},
					"field": {"type": "string"},
					"value": true
				},
				"anyOf": [
					{"required": ["file_option"]},
					{"required": ["field_option"]}
				],
				"not": {"required": ["file_option", "field_option"]},
				"dependentRequired": {"field": ["field_option"]},
				"required": ["value"],
				"additionalProperties": false
			},
			"opts": {
				"type": "object",
				"additionalProperties": {
					"oneOf": [
						{"type": "string"},
						{"type": "number"},
						{"type": "boolean"},
						{"type": "array", "items": {"oneOf": [
							{"type": "string"}, {"type": "number"}, {"type": "boolean"}
						]}}
					]
				}
			}
		},
		"additionalProperties": false
	}`)

	schema, err := CompileJSONSchema(schemaJSON)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	valid := `directory:
  path: proto
  root: .
plugin:
  remote: example/plugin:v1
managed:
  field_option: go_package
  field: acme.Message.field
  value: example.com/acme
opts:
  env: node
  enabled: false
  timeout: 30
  flags: [grpc-js, false, 30]
`
	if res := NewValidator(schema).ValidateBytes([]byte(valid)); res.HasErrors() {
		t.Fatalf("expected EasyP-like document to validate, got %v", res.Collector.Errors())
	}

	t.Run("oneOf required groups", func(t *testing.T) {
		res := NewValidator(schema).ValidateBytes([]byte("plugin:\n  name: go\n  remote: example/plugin:v1\n"))
		if !res.HasErrors() || !containsDiagnostic(res, "exactly one required field group") {
			t.Fatalf("expected oneOf presence error, got %v", res.Collector.Errors())
		}
	})

	t.Run("not required group", func(t *testing.T) {
		res := NewValidator(schema).ValidateBytes([]byte("managed:\n  file_option: go_package\n  field_option: go_package\n  value: x\n"))
		if !res.HasErrors() || !containsDiagnostic(res, "must not be present together") {
			t.Fatalf("expected forbidden-together error, got %v", res.Collector.Errors())
		}
	})

	t.Run("dependentRequired", func(t *testing.T) {
		res := NewValidator(schema).ValidateBytes([]byte("managed:\n  file_option: go_package\n  field: acme.Message.field\n  value: x\n"))
		if !res.HasErrors() || !containsDiagnostic(res, `required when "field" is present`) {
			t.Fatalf("expected dependentRequired error, got %v", res.Collector.Errors())
		}
	})

	t.Run("oneOf schema", func(t *testing.T) {
		res := NewValidator(schema).ValidateBytes([]byte("directory:\n  root: .\n"))
		if !res.HasErrors() || !containsDiagnostic(res, "does not match any allowed schema") {
			t.Fatalf("expected directory union error, got %v", res.Collector.Errors())
		}
	})
}

func TestCompileJSONSchemaRejectsUnsupportedKeywords(t *testing.T) {
	_, err := CompileJSONSchema([]byte(`{"type":"string","pattern":"^[a-z]+$"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported JSON Schema keyword") {
		t.Fatalf("expected explicit unsupported-keyword error, got %v", err)
	}
}

func TestCompileJSONSchemaBooleanFalse(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`false`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte("anything"))
	if !res.HasErrors() {
		t.Fatalf("boolean false schema must reject every value")
	}
}

func containsDiagnostic(result *ValidationResult, part string) bool {
	for _, diagnostic := range result.Collector.All() {
		if strings.Contains(diagnostic.Message, part) {
			return true
		}
	}
	return false
}

func TestCompileJSONSchemaTypeArray(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": ["array", "null"],
		"items": {"type": "string"}
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, input := range []string{"null", "[a, b]"} {
		if res := NewValidator(schema).ValidateBytes([]byte(input)); res.HasErrors() {
			t.Fatalf("expected %q to be valid, got %v", input, res.Collector.Errors())
		}
	}
	if res := NewValidator(schema).ValidateBytes([]byte("plain")); !res.HasErrors() {
		t.Fatalf("expected scalar string to be rejected")
	}
}
