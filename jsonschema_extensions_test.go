package yamlvalidator_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestCompileJSONSchemaCustomFormat(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"propertyNames":{"format":"lower-key"}
	}`), JSONSchemaCompileOptions{
		AssertFormat: true,
		Formats: []JSONSchemaFormat{{
			Name: "lower-key",
			Validate: func(value any) error {
				key, ok := value.(string)
				if ok && key != strings.ToLower(key) {
					return fmt.Errorf("key must be lowercase")
				}
				return nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if res := NewValidator(schema).ValidateBytes([]byte("good: 1\n")); res.HasErrors() {
		t.Fatalf("valid key rejected: %v", res.Collector.Errors())
	}
	res := NewValidator(schema).ValidateBytes([]byte("BadKey: 1\n"))
	if !res.HasErrors() || !containsDiagnosticCode(res, "format") {
		t.Fatalf("expected custom format error, got %v", res.Collector.Errors())
	}
	if !containsDiagnostic(res, "key must be lowercase") {
		t.Fatalf("custom format message lost: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaCustomKeyword(t *testing.T) {
	keyword := JSONSchemaKeyword{
		Name: "x-min-property",
		Compile: func(keywordValue any) (JSONSchemaKeywordValidateFunc, error) {
			config, ok := keywordValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected object configuration")
			}
			name, ok := config["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("name must be a non-empty string")
			}

			minimum, ok := config["minimum"].(json.Number)
			if !ok {
				return nil, fmt.Errorf("minimum must be a number")
			}
			min, err := minimum.Int64()
			if err != nil {
				return nil, fmt.Errorf("minimum must be an integer: %w", err)
			}

			return func(instance any, ctx *JSONSchemaKeywordValidationContext) {
				object, ok := instance.(map[string]any)
				if !ok {
					return
				}
				ctx.MarkPropertyEvaluated(name)
				value, exists := object[name]
				if !exists {
					ctx.AddErrorAt([]string{name}, fmt.Sprintf("property %q is required", name))
					return
				}
				number, ok := value.(json.Number)
				if !ok {
					ctx.AddErrorAt([]string{name}, "property must be an integer")
					return
				}
				got, err := number.Int64()
				if err != nil || got < min {
					ctx.AddErrorAt([]string{name}, fmt.Sprintf("value must be >= %d", min))
				}
			}, nil
		},
	}

	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"x-min-property":{"name":"count","minimum":5},
		"unevaluatedProperties":false
	}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{keyword},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if res := NewValidator(schema).ValidateBytes([]byte("count: 5\n")); res.HasErrors() {
		t.Fatalf("valid custom keyword value rejected: %v", res.Collector.Errors())
	}

	res := NewValidator(schema).ValidateBytes([]byte("count: 3\n"))
	if !containsDiagnosticCode(res, "x-min-property") {
		t.Fatalf("expected custom keyword diagnostic, got %v", res.Collector.Errors())
	}

	var diagnostic *ValidationError
	for _, item := range res.Collector.Errors() {
		if item.Code == "x-min-property" {
			copy := item
			diagnostic = &copy
			break
		}
	}
	if diagnostic == nil {
		t.Fatal("custom keyword diagnostic missing")
	}
	if diagnostic.Path != "count" || diagnostic.Line != 1 || diagnostic.Column != 8 {
		t.Fatalf("unexpected custom keyword source mapping: %+v", *diagnostic)
	}
	if !strings.Contains(diagnostic.SchemaPath, "/x-min-property") {
		t.Fatalf("custom keyword schema path lost: %q", diagnostic.SchemaPath)
	}
	if containsDiagnosticCode(res, "unevaluatedProperties") {
		t.Fatalf("custom keyword did not mark property evaluated: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaCustomKeywordCompileError(t *testing.T) {
	_, err := CompileJSONSchemaWithOptions([]byte(`{"x-positive":123}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{{
			Name: "x-positive",
			Compile: func(value any) (JSONSchemaKeywordValidateFunc, error) {

				if _, ok := value.(bool); !ok {
					return nil, fmt.Errorf("expected boolean")
				}
				return func(any, *JSONSchemaKeywordValidationContext) {}, nil
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), `keyword "x-positive"`) ||
		!strings.Contains(err.Error(), "expected boolean") {
		t.Fatalf("expected keyword compile error, got %v", err)
	}
}

func TestCompileJSONSchemaCustomVocabulary(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{"x-forbid":"bad"}`), JSONSchemaCompileOptions{
		Vocabularies: []JSONSchemaVocabulary{{
			URL: "https://example.test/vocab/business",
			Keywords: []JSONSchemaKeyword{{
				Name: "x-forbid",
				Validate: func(keywordValue, instance any, ctx *JSONSchemaKeywordValidationContext) {
					forbidden, ok := keywordValue.(string)
					if !ok {
						ctx.AddError("x-forbid expects a string")
						return
					}
					if instance == forbidden {
						ctx.AddError("forbidden value")
					}
				},
			}},
		}},
	})

	if err != nil {
		t.Fatalf("compile vocabulary: %v", err)
	}
	if res := NewValidator(schema).ValidateBytes([]byte(`"bad"`)); !containsDiagnosticCode(res, "x-forbid") {
		t.Fatalf("named vocabulary did not run: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsInvalidFunctionalExtensions(t *testing.T) {
	tests := []struct {
		name string
		opts JSONSchemaCompileOptions
		part string
	}{
		{
			name: "nil format",
			opts: JSONSchemaCompileOptions{Formats: []JSONSchemaFormat{{Name: "x"}}},
			part: "validate function is nil",
		},
		{
			name: "nil keyword function",
			opts: JSONSchemaCompileOptions{Keywords: []JSONSchemaKeyword{{Name: "x-rule"}}},
			part: "validate/compile function is nil",
		},
		{
			name: "relative vocabulary URL",
			opts: JSONSchemaCompileOptions{Vocabularies: []JSONSchemaVocabulary{{
				URL: "relative/vocab",
				Keywords: []JSONSchemaKeyword{{
					Name: "x-rule",
					Compile: func(any) (JSONSchemaKeywordValidateFunc, error) {
						return func(any, *JSONSchemaKeywordValidationContext) {}, nil
					},
				}},
			}}},
			part: "must be absolute",
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

func TestCompileJSONSchemaCustomKeywordMarksEvaluatedItems(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"array",
		"x-first-integer":true,
		"unevaluatedItems":false
	}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{{
			Name: "x-first-integer",
			Validate: func(keywordValue, instance any, ctx *JSONSchemaKeywordValidationContext) {
				enabled, _ := keywordValue.(bool)
				if !enabled {
					return
				}
				items, ok := instance.([]any)
				if !ok || len(items) == 0 {
					return
				}
				ctx.MarkItemEvaluated(0)
				if _, ok := items[0].(json.Number); !ok {
					ctx.AddErrorAt([]string{"0"}, "first item must be an integer")
				}
			},
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if res := NewValidator(schema).ValidateBytes([]byte("[1]\n")); res.HasErrors() {
		t.Fatalf("valid evaluated item rejected: %v", res.Collector.Errors())
	}
	res := NewValidator(schema).ValidateBytes([]byte("[nope]\n"))
	if !containsDiagnosticCode(res, "x-first-integer") {
		t.Fatalf("expected custom item diagnostic, got %v", res.Collector.Errors())
	}
	if containsDiagnosticCode(res, "unevaluatedItems") {
		t.Fatalf("custom keyword did not mark item evaluated: %v", res.Collector.Errors())
	}
	for _, diagnostic := range res.Collector.Errors() {
		if diagnostic.Code == "x-first-integer" && diagnostic.Path != "[0]" {
			t.Fatalf("unexpected item path: %+v", diagnostic)
		}
	}
}

func TestCompileJSONSchemaRejectsKeywordValidateAndCompileTogether(t *testing.T) {
	_, err := CompileJSONSchemaWithOptions([]byte(`{"x-rule":true}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{{
			Name:     "x-rule",
			Validate: func(any, any, *JSONSchemaKeywordValidationContext) {},
			Compile: func(any) (JSONSchemaKeywordValidateFunc, error) {
				return func(any, *JSONSchemaKeywordValidationContext) {}, nil
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "set exactly one of Validate, Compile, or CompileWithContext") {
		t.Fatalf("expected mutually exclusive keyword function error, got %v", err)
	}
}

func TestCompileJSONSchemaCustomKeywordWithSubschemas(t *testing.T) {
	keyword := JSONSchemaKeyword{
		Name: "x-dispatch",
		Subschemas: []JSONSchemaSubschemaPath{
			{JSONSchemaSubschemaAllProperties()},
		},
		CompileWithContext: func(
			ctx *JSONSchemaKeywordCompileContext,
			keywordValue any,
		) (JSONSchemaKeywordValidateFunc, error) {
			config, ok := keywordValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected object")
			}
			branches := make(map[string]*JSONSchemaSubschema, len(config))
			for name := range config {
				branches[name] = ctx.Subschema(name)
			}
			return func(instance any, validation *JSONSchemaKeywordValidationContext) {
				object, ok := instance.(map[string]any)
				if !ok {
					return
				}
				kind, _ := object["kind"].(string)
				branch := branches[kind]
				if branch == nil {
					validation.AddErrorAt([]string{"kind"}, "unknown dispatch kind")
					return
				}
				validation.ValidateSubschema(branch, instance, nil)
			}, nil
		},
	}

	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{
			"positiveObject":{
				"type":"object",
				"properties":{"value":{"type":"integer","minimum":1}}
			}
		},
		"type":"object",
		"properties":{
			"kind":{"type":"string"},
			"value":true
		},
		"required":["kind","value"],
		"x-dispatch":{
			"text":{
				"type":"object",
				"properties":{"value":{"type":"string"}}
			},
			"positive":{"$ref":"#/$defs/positiveObject"}
		}
	}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{keyword},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for _, input := range []string{
		`kind: text
value: hello
`,
		`kind: positive
value: 2
`,
	} {
		if result := NewValidator(schema).ValidateBytes([]byte(input)); result.HasErrors() {
			t.Fatalf("valid input %q rejected: %v", input, result.Collector.Errors())
		}
	}

	result := NewValidator(schema).ValidateBytes([]byte(`kind: positive
value: -1
`))
	if !containsDiagnosticCode(result, "minimum") {
		t.Fatalf("nested $ref subschema did not run: %v", result.Collector.Errors())
	}

	result = NewValidator(schema).ValidateBytes([]byte(`kind: text
value: 2
`))
	if !containsDiagnosticCode(result, "type") {
		t.Fatalf("nested inline subschema did not run: %v", result.Collector.Errors())
	}

	result = NewValidator(schema).ValidateBytes([]byte(`kind: missing
value: 2
`))
	if !containsDiagnosticCode(result, "x-dispatch") {
		t.Fatalf("custom dispatch error missing: %v", result.Collector.Errors())
	}
}

func TestCompileJSONSchemaCustomKeywordReferenceHelper(t *testing.T) {
	keyword := JSONSchemaKeyword{
		Name: "x-ref",
		CompileWithContext: func(
			ctx *JSONSchemaKeywordCompileContext,
			keywordValue any,
		) (JSONSchemaKeywordValidateFunc, error) {
			ref, ok := keywordValue.(string)
			if !ok {
				return nil, fmt.Errorf("expected string reference")
			}
			schema, err := ctx.Reference(ref)
			if err != nil {
				return nil, err
			}
			return func(instance any, validation *JSONSchemaKeywordValidationContext) {
				validation.ValidateSubschema(schema, instance, nil)
			}, nil
		},
	}

	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$defs":{"positive":{"type":"integer","minimum":1}},
		"x-ref":"#/$defs/positive"
	}`), JSONSchemaCompileOptions{
		Keywords: []JSONSchemaKeyword{keyword},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result := NewValidator(schema).ValidateBytes([]byte("2")); result.HasErrors() {
		t.Fatalf("valid referenced schema rejected: %v", result.Collector.Errors())
	}
	if result := NewValidator(schema).ValidateBytes([]byte("0")); !containsDiagnosticCode(result, "minimum") {
		t.Fatalf("reference helper did not validate: %v", result.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsInvalidSubschemaPaths(t *testing.T) {
	tests := []struct {
		name string
		path JSONSchemaSubschemaPath
		part string
	}{
		{
			name: "zero-value position",
			path: JSONSchemaSubschemaPath{{}},
			part: "invalid subschema position",
		},
		{
			name: "negative item",
			path: JSONSchemaSubschemaPath{JSONSchemaSubschemaItem(-1)},
			part: "item index must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileJSONSchemaWithOptions([]byte(`{"x-rule":true}`), JSONSchemaCompileOptions{
				Keywords: []JSONSchemaKeyword{{
					Name:       "x-rule",
					Subschemas: []JSONSchemaSubschemaPath{tt.path},
					Validate:   func(any, any, *JSONSchemaKeywordValidationContext) {},
				}},
			})
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("expected error containing %q, got %v", tt.part, err)
			}
		})
	}
}

func TestCompileJSONSchemaVocabularySchemaValidatesKeywordDefinitions(t *testing.T) {
	vocabulary := JSONSchemaVocabulary{
		URL: "https://example.test/vocab/typed-keyword",
		Schema: []byte(`{
			"$schema":"https://json-schema.org/draft/2020-12/schema",
			"properties":{
				"x-forbid":{"type":"string"}
			}
		}`),
		Keywords: []JSONSchemaKeyword{{
			Name: "x-forbid",
			Validate: func(keywordValue, instance any, ctx *JSONSchemaKeywordValidationContext) {
				if instance == keywordValue {
					ctx.AddError("forbidden value")
				}
			},
		}},
	}

	if _, err := CompileJSONSchemaWithOptions(
		[]byte(`{"x-forbid":"bad"}`),
		JSONSchemaCompileOptions{Vocabularies: []JSONSchemaVocabulary{vocabulary}},
	); err != nil {
		t.Fatalf("valid vocabulary keyword definition rejected: %v", err)
	}

	_, err := CompileJSONSchemaWithOptions(
		[]byte(`{"x-forbid":123}`),
		JSONSchemaCompileOptions{Vocabularies: []JSONSchemaVocabulary{vocabulary}},
	)
	if err == nil {
		t.Fatal("vocabulary schema must reject invalid keyword definition")
	}
}

func TestCompileJSONSchemaRejectsMalformedVocabularySchema(t *testing.T) {
	_, err := CompileJSONSchemaWithOptions(
		[]byte(`true`),
		JSONSchemaCompileOptions{
			Vocabularies: []JSONSchemaVocabulary{{
				URL:    "https://example.test/vocab/bad-schema",
				Schema: []byte(`{"type":`),
				Keywords: []JSONSchemaKeyword{{
					Name:     "x-rule",
					Validate: func(any, any, *JSONSchemaKeywordValidationContext) {},
				}},
			}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "decode JSON Schema vocabulary schema") {
		t.Fatalf("expected malformed vocabulary schema error, got %v", err)
	}
}

func TestJSONSchemaVocabularySchemaCanUsePreloadedResource(t *testing.T) {
	vocabulary := JSONSchemaVocabulary{
		URL: "https://example.test/vocab/with-ref",
		Schema: []byte(`{
			"$schema":"https://json-schema.org/draft/2020-12/schema",
			"properties":{
				"x-name":{"$ref":"https://example.test/meta/common.json#/$defs/name"}
			}
		}`),
		Keywords: []JSONSchemaKeyword{{
			Name:     "x-name",
			Validate: func(any, any, *JSONSchemaKeywordValidationContext) {},
		}},
	}

	_, err := CompileJSONSchemaWithOptions(
		[]byte(`{"x-name":"ok"}`),
		JSONSchemaCompileOptions{
			Resources: map[string][]byte{
				"https://example.test/meta/common.json": []byte(`{
					"$defs":{"name":{"type":"string","minLength":2}}
				}`),
			},
			Vocabularies: []JSONSchemaVocabulary{vocabulary},
		},
	)
	if err != nil {
		t.Fatalf("vocabulary schema could not resolve preloaded resource: %v", err)
	}

	_, err = CompileJSONSchemaWithOptions(
		[]byte(`{"x-name":"x"}`),
		JSONSchemaCompileOptions{
			Resources: map[string][]byte{
				"https://example.test/meta/common.json": []byte(`{
					"$defs":{"name":{"type":"string","minLength":2}}
				}`),
			},
			Vocabularies: []JSONSchemaVocabulary{vocabulary},
		},
	)
	if err == nil {
		t.Fatal("vocabulary schema must enforce referenced keyword definition")
	}
}
