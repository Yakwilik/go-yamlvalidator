package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestLoadValidatorJSONSchemaWithRelativeRef(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "schema.json")
	defsPath := filepath.Join(dir, "defs.json")

	if err := os.WriteFile(defsPath, []byte(`{
  "$defs": {
    "name": {"type":"string","minLength":3}
  }
}`), 0o644); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := os.WriteFile(rootPath, []byte(`{
  "$schema":"https://json-schema.org/draft/2020-12/schema",
  "$ref":"defs.json#/$defs/name"
}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	validator, err := loadValidator(rootPath, "jsonschema", jsonSchemaCLIOptions{DefaultDraft: v.JSONSchemaDraft2020})
	if err != nil {
		t.Fatalf("load JSON Schema: %v", err)
	}
	if res := validator.ValidateBytes([]byte(`"abc"`)); res.HasErrors() {
		t.Fatalf("valid value rejected: %v", res.Collector.Errors())
	}
	if res := validator.ValidateBytes([]byte(`"a"`)); !res.HasErrors() {
		t.Fatalf("invalid value accepted")
	}
}

func TestLoadValidatorJSONSchemaDraftOption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, []byte(`{
  "type":"number",
  "minimum":1,
  "exclusiveMinimum":true
}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	validator, err := loadValidator(path, "jsonschema", jsonSchemaCLIOptions{DefaultDraft: v.JSONSchemaDraft4})
	if err != nil {
		t.Fatalf("load draft4 schema: %v", err)
	}
	if res := validator.ValidateBytes([]byte("1")); !res.HasErrors() {
		t.Fatalf("draft4 exclusiveMinimum semantics not applied")
	}
}

func TestLoadValidatorFieldSchemaRunsCompileValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.yaml")
	if err := os.WriteFile(path, []byte("type: string\nminItems: 1\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	_, err := loadValidator(path, "field", jsonSchemaCLIOptions{})
	if err == nil || !strings.Contains(err.Error(), "sequence constraints require sequence/any type") {
		t.Fatalf("expected native schema compile error, got %v", err)
	}
}

func TestLoadValidatorRejectsUnknownSchemaFormat(t *testing.T) {
	_, err := loadValidator("ignored", "wat", jsonSchemaCLIOptions{})
	if err == nil || !strings.Contains(err.Error(), "unknown schema format") {
		t.Fatalf("expected schema format error, got %v", err)
	}
}
