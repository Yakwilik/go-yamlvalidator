package main

import (
	"os"
	"path/filepath"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestLoadSchemaFromFile_YAML(t *testing.T) {
	tmp := t.TempDir()
	schemaPath := filepath.Join(tmp, "schema.yaml")
	err := os.WriteFile(schemaPath, []byte(`
type: map
required: true
allowedKeys:
  name:
    type: string
    required: true
  replicas:
    type: int
    validators:
      - name: range
        min: 1
        max: 10
unknownKeyPolicy: warn
`), 0o644)
	if err != nil {
		t.Fatalf("write schema: %v", err)
	}

	schema, err := loadSchemaFromFile(schemaPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	if schema.Type != v.TypeMap || !schema.Required {
		t.Fatalf("unexpected root schema: %+v", schema)
	}
	if got := schema.AllowedKeys["name"]; got == nil || got.Type != v.TypeString || !got.Required {
		t.Fatalf("unexpected name schema: %+v", got)
	}
}

func TestLoadSchemaFromFile_JSON(t *testing.T) {
	tmp := t.TempDir()
	schemaPath := filepath.Join(tmp, "schema.json")
	err := os.WriteFile(schemaPath, []byte(`{
  "type": "sequence",
  "itemSchema": {
    "type": "int"
  },
  "minItems": 1,
  "validators": [
    {"name": "length", "minLength": 1, "maxLength": 3}
  ]
}`), 0o644)
	if err != nil {
		t.Fatalf("write schema: %v", err)
	}

	schema, err := loadSchemaFromFile(schemaPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	if schema.Type != v.TypeSequence || schema.ItemSchema == nil || schema.ItemSchema.Type != v.TypeInt {
		t.Fatalf("unexpected schema: %+v", schema)
	}
}

func TestLoadSchemaFromFile_UnknownValidator(t *testing.T) {
	tmp := t.TempDir()
	schemaPath := filepath.Join(tmp, "schema.yaml")
	err := os.WriteFile(schemaPath, []byte(`type: string
validators:
  - name: not-a-real-validator
`), 0o644)
	if err != nil {
		t.Fatalf("write schema: %v", err)
	}
	if _, err := loadSchemaFromFile(schemaPath); err == nil {
		t.Fatalf("expected error for unknown validator")
	}
}
