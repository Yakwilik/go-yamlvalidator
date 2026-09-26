package recipes_test

import (
	"fmt"
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestJSONSchemaCustomFormats(t *testing.T) {
	format := v.JSONSchemaFormat{Name: "lowercase", Validate: func(value any) error {
		s, ok := value.(string)
		if ok && s != strings.ToLower(s) {
			return fmt.Errorf("name must be lowercase")
		}
		return nil
	}}
	for _, tc := range []struct{ name, source, good, bad string }{
		{"value", `{"type":"string","format":"lowercase"}`, "api", "API"},
		{"keys", `{"type":"object","propertyNames":{"format":"lowercase"}}`, "name: value", "Name: value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validator := compileJSON(t, tc.source, v.JSONSchemaCompileOptions{AssertFormat: true, Formats: []v.JSONSchemaFormat{format}})
			checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: tc.good, valid: true}, {name: "invalid", input: tc.bad, code: "format"}})
		})
	}
}

func forbiddenKeyword() v.JSONSchemaKeyword {
	return v.JSONSchemaKeyword{Name: "x-forbidden", Compile: func(raw any) (v.JSONSchemaKeywordValidateFunc, error) {
		forbidden, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected string configuration")
		}
		return func(instance any, ctx *v.JSONSchemaKeywordValidationContext) {
			if value, ok := instance.(string); ok && value == forbidden {
				ctx.AddError("value is reserved")
			}
		}, nil
	}}
}

func TestJSONSchemaKeywordCallbacks(t *testing.T) {
	compiled := forbiddenKeyword()
	simple := v.JSONSchemaKeyword{Name: "x-forbidden", Validate: func(raw, instance any, ctx *v.JSONSchemaKeywordValidationContext) {
		forbidden, ok := raw.(string)
		if !ok {
			ctx.AddError("expected string configuration")
			return
		}
		if s, ok := instance.(string); ok && s == forbidden {
			ctx.AddError("value is reserved")
		}
	}}
	for _, tc := range []struct {
		name    string
		keyword v.JSONSchemaKeyword
	}{{"Validate", simple}, {"Compile", compiled}} {
		t.Run(tc.name, func(t *testing.T) {
			validator := compileJSON(t, `{"type":"string","x-forbidden":"system"}`, v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{tc.keyword}})
			checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "api", valid: true}, {name: "invalid", input: "system", code: "x-forbidden"}})
		})
	}
	if _, err := v.CompileJSONSchemaWithOptions([]byte(`{"x-forbidden":123}`), v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{compiled}}); err == nil {
		t.Fatal("expected compile-time configuration error")
	}
	both := compiled
	both.Validate = simple.Validate
	if _, err := v.CompileJSONSchemaWithOptions([]byte(`true`), v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{both}}); err == nil {
		t.Fatal("multiple callbacks must fail")
	}
}

func TestJSONSchemaEvaluatedMembersAndPaths(t *testing.T) {
	objectKeyword := v.JSONSchemaKeyword{Name: "x-name", Validate: func(raw, instance any, ctx *v.JSONSchemaKeywordValidationContext) {
		if enabled, ok := raw.(bool); !ok || !enabled {
			return
		}
		object, ok := instance.(map[string]any)
		if !ok {
			return
		}
		if name, exists := object["name"]; exists {
			ctx.MarkPropertyEvaluated("name")
			if s, ok := name.(string); !ok || s == "" {
				ctx.AddErrorAt([]string{"name"}, "nonempty name required")
			}
		} else {
			ctx.AddErrorAt([]string{"name"}, "name is missing")
		}
	}}
	validator := compileJSON(t, `{"type":"object","x-name":true,"unevaluatedProperties":false}`, v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{objectKeyword}})
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{
		{name: "evaluated", input: "name: api", valid: true}, {name: "missing", input: "{}", code: "x-name"},
		{name: "unhandled", input: "name: api\nextra: value"},
	})
	result := validator.ValidateBytes([]byte("name: 12\n"))
	found := false
	for _, diagnostic := range result.Collector.Errors() {
		if diagnostic.Code == "x-name" {
			found = true
			if diagnostic.Path != "name" || diagnostic.Line != 1 || diagnostic.Column != 7 {
				t.Fatalf("wrong location: %+v", diagnostic)
			}
		}
	}
	if !found {
		t.Fatal("custom diagnostic not found")
	}
	itemKeyword := v.JSONSchemaKeyword{Name: "x-first", Validate: func(_, instance any, ctx *v.JSONSchemaKeywordValidationContext) {
		items, ok := instance.([]any)
		if !ok || len(items) == 0 {
			return
		}
		ctx.MarkItemEvaluated(0)
		if _, ok := items[0].(string); !ok {
			ctx.AddErrorAt([]string{"0"}, "string required")
		}
	}}
	checkInputs(t, compileJSON(t, `{"type":"array","x-first":true,"unevaluatedItems":false}`, v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{itemKeyword}}), v.ValidationContext{}, []inputCase{
		{name: "evaluated first", input: "[api]", valid: true}, {name: "first type", input: "[1]", code: "x-first"}, {name: "extra item", input: "[api,other]"},
	})
}

func TestJSONSchemaSubschemaSelectors(t *testing.T) {
	tests := []struct {
		name        string
		declaration v.JSONSchemaSubschemaPath
		path        []string
		raw         string
	}{
		{"self", v.JSONSchemaSubschemaPath{}, nil, `{"type":"string"}`},
		{"property", v.JSONSchemaSubschemaPath{v.JSONSchemaSubschemaProperty("schema")}, []string{"schema"}, `{"schema":{"type":"string"}}`},
		{"all properties", v.JSONSchemaSubschemaPath{v.JSONSchemaSubschemaAllProperties()}, []string{"text"}, `{"text":{"type":"string"}}`},
		{"item", v.JSONSchemaSubschemaPath{v.JSONSchemaSubschemaItem(0)}, []string{"0"}, `[{"type":"string"}]`},
		{"all items", v.JSONSchemaSubschemaPath{v.JSONSchemaSubschemaAllItems()}, []string{"0"}, `[{"type":"string"}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyword := v.JSONSchemaKeyword{
				Name: "x-check", Subschemas: []v.JSONSchemaSubschemaPath{tc.declaration},
				CompileWithContext: func(ctx *v.JSONSchemaKeywordCompileContext, _ any) (v.JSONSchemaKeywordValidateFunc, error) {
					nested := ctx.Subschema(tc.path...)
					return func(instance any, run *v.JSONSchemaKeywordValidationContext) {
						run.ValidateSubschema(nested, instance, nil)
					}, nil
				},
			}
			validator := compileJSON(t, `{"x-check":`+tc.raw+`}`, v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{keyword}})
			checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "api", valid: true}, {name: "invalid", input: "123", code: "type"}})
		})
	}
}

func TestJSONSchemaSubschemaReference(t *testing.T) {
	keyword := v.JSONSchemaKeyword{Name: "x-reference", CompileWithContext: func(ctx *v.JSONSchemaKeywordCompileContext, raw any) (v.JSONSchemaKeywordValidateFunc, error) {
		ref, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected reference string")
		}
		nested, err := ctx.Reference(ref)
		if err != nil {
			return nil, err
		}
		return func(instance any, run *v.JSONSchemaKeywordValidationContext) {
			run.ValidateSubschema(nested, instance, nil)
		}, nil
	}}
	validator := compileJSON(t, `{"$defs":{"positive":{"type":"integer","minimum":1}},"x-reference":"#/$defs/positive"}`, v.JSONSchemaCompileOptions{Keywords: []v.JSONSchemaKeyword{keyword}})
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "2", valid: true}, {name: "invalid", input: "0", code: "minimum"}})
}

func TestJSONSchemaVocabularyWithSchema(t *testing.T) {
	vocabulary := v.JSONSchemaVocabulary{
		URL:      "https://schemas.example.test/vocab/names",
		Schema:   []byte(`{"properties":{"x-forbidden":{"$ref":"https://schemas.example.test/meta/string"}}}`),
		Keywords: []v.JSONSchemaKeyword{forbiddenKeyword()},
	}
	options := v.JSONSchemaCompileOptions{
		Vocabularies: []v.JSONSchemaVocabulary{vocabulary},
		Resources:    map[string][]byte{"https://schemas.example.test/meta/string": []byte(`{"type":"string"}`)},
	}
	checkInputs(t, compileJSON(t, `{"type":"string","x-forbidden":"system"}`, options), v.ValidationContext{}, []inputCase{
		{name: "valid", input: "api", valid: true}, {name: "invalid", input: "system", code: "x-forbidden"},
	})
	if _, err := v.CompileJSONSchemaWithOptions([]byte(`{"x-forbidden":12}`), options); err == nil {
		t.Fatal("vocabulary schema must reject malformed configuration")
	}
}
