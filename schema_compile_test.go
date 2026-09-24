package yamlvalidator_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func TestCompileFieldSchemaValid(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {
				Type:     TypeString,
				Required: true,
				Validators: []ValueValidator{
					valv.RegexValidator{Pattern: regexp.MustCompile(`^[a-z]+$`)},
				},
			},
			"items": {
				Type:       TypeSequence,
				MinItems:   Ptr(1),
				ItemSchema: &FieldSchema{Type: TypeString},
			},
		},
		ExactlyOneOf: []string{"name", "items"},
		KeyValidators: []KeyValidator{
			keyv.LengthKeyValidator{Min: Ptr(1)},
		},
	}
	if _, err := CompileFieldSchema(schema); err != nil {
		t.Fatalf("compile valid schema: %v", err)
	}
}

func TestCompileFieldSchemaRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name   string
		schema *FieldSchema
		part   string
	}{
		{
			name:   "nil regex",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.RegexValidator{}}},
			part:   "regex pattern must not be nil",
		},
		{
			name:   "inverted range",
			schema: &FieldSchema{Type: TypeInt, Validators: []ValueValidator{valv.RangeValidator{Min: Ptr(10.0), Max: Ptr(1.0)}}},
			part:   "minimum must not exceed maximum",
		},
		{
			name:   "invalid URL scheme definition",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.URLValidator{AllowedSchemes: []string{"1http"}}}},
			part:   "invalid allowed URL scheme",
		},
		{
			name:   "inverted length",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.LengthValidator{Min: Ptr(3), Max: Ptr(2)}}},
			part:   "minimum length must not exceed maximum length",
		},
		{
			name:   "mapping constraints on scalar",
			schema: &FieldSchema{Type: TypeString, AllowedKeys: map[string]*FieldSchema{"x": {Type: TypeString}}},
			part:   "mapping constraints require map/any type",
		},
		{
			name:   "sequence constraints on map",
			schema: &FieldSchema{Type: TypeMap, MinItems: Ptr(1)},
			part:   "sequence constraints require sequence/any type",
		},
		{
			name: "unknown inter-field reference",
			schema: &FieldSchema{
				Type:         TypeMap,
				AllowedKeys:  map[string]*FieldSchema{"a": {Type: TypeString}},
				ExactlyOneOf: []string{"a", "b"},
			},
			part: `field "b" is not declared`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileFieldSchema(tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("expected error containing %q, got %v", tt.part, err)
			}
		})
	}
}

func TestCompileFieldSchemaRejectsRecursiveGraph(t *testing.T) {
	schema := &FieldSchema{Type: TypeMap}
	schema.AdditionalProperties = schema
	_, err := CompileFieldSchema(schema)
	if err == nil || !strings.Contains(err.Error(), "recursive FieldSchema graph") {
		t.Fatalf("expected recursive graph error, got %v", err)
	}
}
