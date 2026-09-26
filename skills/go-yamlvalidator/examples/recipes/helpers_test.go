package recipes_test

import (
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

type inputCase struct {
	name     string
	input    string
	valid    bool
	code     string
	warnings int
}

func checkInputs(t *testing.T, validator *v.Validator, opts v.ValidationContext, cases []inputCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateWithOptions([]byte(tc.input), opts)
			if got := !result.HasErrors(); got != tc.valid {
				t.Fatalf("valid=%v, want %v; %s", got, tc.valid, result.FormatAll(true))
			}
			if tc.code != "" && !hasCode(result, tc.code) {
				t.Fatalf("missing code %q: %+v", tc.code, result.Collector.All())
			}
			if tc.warnings >= 0 && len(result.Collector.Warnings()) != tc.warnings {
				t.Fatalf("warnings=%v, want count %d", result.Collector.Warnings(), tc.warnings)
			}
		})
	}
}

func hasCode(result *v.ValidationResult, code string) bool {
	for _, diagnostic := range result.Collector.All() {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func compileJSON(t *testing.T, source string, opts v.JSONSchemaCompileOptions) *v.Validator {
	t.Helper()
	schema, err := v.CompileJSONSchemaWithOptions([]byte(source), opts)
	if err != nil {
		t.Fatal(err)
	}
	return v.NewValidator(schema)
}

func stringFields(names ...string) *v.FieldSchema {
	schema := &v.FieldSchema{
		Type:             v.TypeMap,
		UnknownKeyPolicy: v.UnknownKeyError,
		AllowedKeys:      make(map[string]*v.FieldSchema),
	}
	for _, name := range names {
		schema.AllowedKeys[name] = &v.FieldSchema{Type: v.TypeString}
	}
	return schema
}
