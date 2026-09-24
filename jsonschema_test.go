package yamlvalidator_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/Yakwilik/go-yamlvalidator"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
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
		if !res.HasErrors() || !containsDiagnosticCode(res, "oneOf") {
			t.Fatalf("expected oneOf presence error, got %v", res.Collector.Errors())
		}
	})

	t.Run("not required group", func(t *testing.T) {
		res := NewValidator(schema).ValidateBytes([]byte("managed:\n  file_option: go_package\n  field_option: go_package\n  value: x\n"))
		if !res.HasErrors() || !containsDiagnosticCode(res, "not") {
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
		if !res.HasErrors() || !containsDiagnosticCode(res, "oneOf") {
			t.Fatalf("expected directory union error, got %v", res.Collector.Errors())
		}
	})
}

func TestCompileJSONSchemaSupportsValidationKeywordsAndIgnoresUnknownAnnotations(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": "string",
		"pattern": "^[a-z]+$",
		"minLength": 3,
		"x-custom-annotation": {"anything": true}
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if res := NewValidator(schema).ValidateBytes([]byte(`"abc"`)); res.HasErrors() {
		t.Fatalf("expected valid string, got %v", res.Collector.Errors())
	}
	res := NewValidator(schema).ValidateBytes([]byte(`"A"`))
	if !containsDiagnosticCode(res, "pattern") || !containsDiagnosticCode(res, "minLength") {
		t.Fatalf("expected pattern and minLength diagnostics, got %v", res.Collector.Errors())
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

func containsDiagnosticCode(result *ValidationResult, code string) bool {
	for _, diagnostic := range result.Collector.All() {
		if diagnostic.Code == code {
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

func TestCompileJSONSchemaDraft2020Features(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs": {
			"positiveEven": {"type": "integer", "minimum": 0, "multipleOf": 2}
		},
		"type": "object",
		"properties": {
			"id": {"$ref": "#/$defs/positiveEven"},
			"tuple": {
				"type": "array",
				"prefixItems": [{"type": "string"}, {"type": "integer"}],
				"items": false
			},
			"choice": {"oneOf": [{"const": "a"}, {"const": "b"}]},
			"conditional": {
				"type": "object",
				"properties": {
					"kind": {"enum": ["x", "y"]},
					"x": {"type": "string"}
				},
				"if": {"properties": {"kind": {"const": "x"}}, "required": ["kind"]},
				"then": {"required": ["x"]}
			},
			"bag": {
				"type": "array",
				"contains": {"type": "integer", "minimum": 10},
				"minContains": 2,
				"maxContains": 3
			}
		},
		"required": ["id", "tuple", "choice", "conditional", "bag"],
		"additionalProperties": false
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	valid := `id: 8
tuple: [name, 42]
choice: a
conditional:
  kind: x
  x: value
bag: [10, 11, nope]
`
	if res := NewValidator(schema).ValidateBytes([]byte(valid)); res.HasErrors() {
		t.Fatalf("expected valid 2020-12 document, got %v", res.Collector.Errors())
	}

	invalid := `id: 7
tuple: [name, 42, extra]
choice: c
conditional:
  kind: x
bag: [1, 10]
`
	res := NewValidator(schema).ValidateBytes([]byte(invalid))
	for _, code := range []string{"multipleOf", "oneOf", "required", "minContains"} {
		if !containsDiagnosticCode(res, code) {
			t.Fatalf("expected diagnostic code %q, got %v", code, res.Collector.Errors())
		}
	}
}

func TestCompileJSONSchemaObjectApplicators(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"allOf": [
			{"properties": {"fixed": {"type": "string"}}}
		],
		"patternProperties": {
			"^x-": {"type": "integer"}
		},
		"propertyNames": {"pattern": "^[a-z][a-z0-9-]*$"},
		"dependentSchemas": {
			"feature": {"required": ["feature-config"]}
		},
		"unevaluatedProperties": false
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	valid := `fixed: ok
x-count: 2
`
	if res := NewValidator(schema).ValidateBytes([]byte(valid)); res.HasErrors() {
		t.Fatalf("expected valid object applicators, got %v", res.Collector.Errors())
	}

	for name, input := range map[string]string{
		"patternProperties":     "x-count: nope\n",
		"propertyNames":         "Bad_Key: 1\n",
		"dependentSchemas":      "feature: true\n",
		"unevaluatedProperties": "unknown: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			res := NewValidator(schema).ValidateBytes([]byte(input))
			if !res.HasErrors() {
				t.Fatalf("expected %s failure", name)
			}
		})
	}
}

func TestCompileJSONSchemaRespectsDeclaredDrafts(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		valid  string
		bad    string
	}{
		{
			name:   "draft4",
			schema: `{"$schema":"http://json-schema.org/draft-04/schema#","type":"number","minimum":1,"exclusiveMinimum":true}`,
			valid:  "2",
			bad:    "1",
		},
		{
			name:   "draft6",
			schema: `{"$schema":"http://json-schema.org/draft-06/schema#","type":"number","exclusiveMinimum":1}`,
			valid:  "2",
			bad:    "1",
		},
		{
			name:   "draft7",
			schema: `{"$schema":"http://json-schema.org/draft-07/schema#","if":{"const":"x"},"then":{"minLength":2}}`,
			valid:  `"xy"`,
			bad:    `"x"`,
		},
		{
			name:   "2019-09",
			schema: `{"$schema":"https://json-schema.org/draft/2019-09/schema","$defs":{"x":{"type":"integer"}},"$ref":"#/$defs/x"}`,
			valid:  "1",
			bad:    `"1"`,
		},
		{
			name:   "2020-12",
			schema: `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","prefixItems":[{"type":"string"}],"items":false}`,
			valid:  `["x"]`,
			bad:    `["x", "y"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := CompileJSONSchema([]byte(tt.schema))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if res := NewValidator(schema).ValidateBytes([]byte(tt.valid)); res.HasErrors() {
				t.Fatalf("valid input rejected: %v", res.Collector.Errors())
			}
			if res := NewValidator(schema).ValidateBytes([]byte(tt.bad)); !res.HasErrors() {
				t.Fatalf("invalid input accepted")
			}
		})
	}
}

func TestCompileJSONSchemaDefaultDraftOption(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"type": "number",
		"minimum": 1,
		"exclusiveMinimum": true
	}`), JSONSchemaCompileOptions{DefaultDraft: JSONSchemaDraft4})
	if err != nil {
		t.Fatalf("compile draft4 schema without $schema: %v", err)
	}
	if res := NewValidator(schema).ValidateBytes([]byte("1")); !res.HasErrors() {
		t.Fatalf("draft4 exclusiveMinimum boolean semantics were not applied")
	}
}

func TestCompileJSONSchemaExternalRefs(t *testing.T) {
	t.Run("preloaded resources", func(t *testing.T) {
		schema, err := CompileJSONSchemaWithOptions([]byte(`{
			"$ref": "types.json#/$defs/name"
		}`), JSONSchemaCompileOptions{
			SchemaURL: "https://example.test/schemas/root.json",
			Resources: map[string][]byte{
				"https://example.test/schemas/types.json": []byte(`{
					"$defs": {"name": {"type": "string", "minLength": 3}}
				}`),
			},
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if res := NewValidator(schema).ValidateBytes([]byte(`"abc"`)); res.HasErrors() {
			t.Fatalf("valid external ref rejected: %v", res.Collector.Errors())
		}
		if res := NewValidator(schema).ValidateBytes([]byte(`"a"`)); !res.HasErrors() {
			t.Fatalf("invalid external ref input accepted")
		}
	})

	t.Run("custom URL loader", func(t *testing.T) {
		loaded := false
		schema, err := CompileJSONSchemaWithOptions([]byte(`{
			"$ref": "https://schemas.example.test/value.json"
		}`), JSONSchemaCompileOptions{
			LoadURL: func(url string) ([]byte, error) {
				loaded = true
				if url != "https://schemas.example.test/value.json" {
					t.Fatalf("unexpected URL: %s", url)
				}
				return []byte(`{"type":"integer","minimum":10}`), nil
			},
		})
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if !loaded {
			t.Fatalf("custom loader was not used")
		}
		if res := NewValidator(schema).ValidateBytes([]byte("10")); res.HasErrors() {
			t.Fatalf("valid loaded schema rejected: %v", res.Collector.Errors())
		}
	})
}

func TestCompileJSONSchemaFormatAssertions(t *testing.T) {
	schemaJSON := []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "string",
		"format": "email"
	}`)

	annotationOnly, err := CompileJSONSchema(schemaJSON)
	if err != nil {
		t.Fatalf("compile annotation schema: %v", err)
	}
	if res := NewValidator(annotationOnly).ValidateBytes([]byte(`"not-an-email"`)); res.HasErrors() {
		t.Fatalf("2020-12 format should be annotation-only by default, got %v", res.Collector.Errors())
	}

	asserted, err := CompileJSONSchemaWithOptions(schemaJSON, JSONSchemaCompileOptions{AssertFormat: true})
	if err != nil {
		t.Fatalf("compile asserted schema: %v", err)
	}
	res := NewValidator(asserted).ValidateBytes([]byte(`"not-an-email"`))
	if !containsDiagnosticCode(res, "format") {
		t.Fatalf("expected format assertion error, got %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaContentAssertions(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"type": "string",
		"contentEncoding": "base64"
	}`), JSONSchemaCompileOptions{AssertContent: true})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte(`"%%%"`))
	if !containsDiagnosticCode(res, "contentEncoding") {
		t.Fatalf("expected contentEncoding error, got %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaDiagnosticsKeepYAMLPositionAndSchemaPath(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": "object",
		"properties": {
			"nested": {
				"type": "object",
				"properties": {"count": {"type": "integer", "minimum": 10}}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte("nested:\n  count: 3\n"))
	if len(res.Collector.Errors()) == 0 {
		t.Fatalf("expected validation error")
	}
	var found *ValidationError
	for i := range res.Collector.Errors() {
		if res.Collector.Errors()[i].Code == "minimum" {
			candidate := res.Collector.Errors()[i]
			found = &candidate
			break
		}
	}
	if found == nil {
		t.Fatalf("minimum diagnostic not found: %v", res.Collector.Errors())
	}
	if found.Path != "nested.count" || found.Line != 2 || found.Column != 10 {
		t.Fatalf("unexpected source location/path: %+v", *found)
	}
	if !strings.Contains(found.SchemaPath, "/properties/nested/properties/count/minimum") {
		t.Fatalf("unexpected schema path: %q", found.SchemaPath)
	}
}

func TestCompileJSONSchemaYAMLToJSONDataModel(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": "object",
		"properties": {
			"legacyOctal": {"const": 511},
			"hex": {"const": 16},
			"huge": {"type": "integer", "minimum": 18446744073709551616},
			"quotedBool": {"type": "string"}
		},
		"required": ["legacyOctal", "hex", "huge", "quotedBool"]
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	input := "legacyOctal: 0777\nhex: 0x10\nhuge: 18446744073709551616\nquotedBool: \"yes\"\n"
	if res := NewValidator(schema).ValidateWithOptions([]byte(input), ValidationContext{YAML11Booleans: true}); res.HasErrors() {
		t.Fatalf("YAML scalar conversion lost JSON numeric/string semantics: %v", res.Collector.Errors())
	}

	nonFinite, err := CompileJSONSchema([]byte(`{"type":"number"}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(nonFinite).ValidateBytes([]byte(".nan"))
	if !containsDiagnosticCode(res, "json_instance_conversion") {
		t.Fatalf("non-finite YAML number must fail JSON data-model conversion: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsNonStringYAMLObjectKeys(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte("1: value\n"))
	if !containsDiagnosticCode(res, "json_instance_conversion") {
		t.Fatalf("non-string YAML key must not be silently coerced into JSON object key: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsMalformedSchema(t *testing.T) {
	_, err := CompileJSONSchema([]byte(`{"type": 123}`))
	if err == nil {
		t.Fatalf("expected metaschema compilation error")
	}
}

func TestCompileJSONSchemaECMAScriptRegex(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type": "string",
		"pattern": "\\p{Letter}cole"
	}`))
	if err != nil {
		t.Fatalf("compile ECMAScript unicode pattern: %v", err)
	}
	if res := NewValidator(schema).ValidateBytes([]byte(`"l'école"`)); res.HasErrors() {
		t.Fatalf("expected ECMAScript unicode property pattern to match: %v", res.Collector.Errors())
	}
	if res := NewValidator(schema).ValidateBytes([]byte(`"L'ÉCOLE"`)); !res.HasErrors() {
		t.Fatalf("pattern matching must remain case-sensitive")
	}

	_, err = CompileJSONSchema([]byte(`{"type":"string","pattern":"(?i)abc"}`))
	if err == nil {
		t.Fatalf("global .NET-style inline flags must not be accepted as ECMAScript regex syntax")
	}
}

func TestCompileJSONSchemaConfigureCompiler(t *testing.T) {
	configured := false
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "string",
		"format": "even-length"
	}`), JSONSchemaCompileOptions{
		AssertFormat: true,
		ConfigureCompiler: func(compiler *jsonschema.Compiler) error {
			configured = true
			compiler.RegisterFormat(&jsonschema.Format{
				Name: "even-length",
				Validate: func(value any) error {
					text, ok := value.(string)
					if ok && len(text)%2 != 0 {
						return fmt.Errorf("length must be even")
					}
					return nil
				},
			})
			return nil
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !configured {
		t.Fatalf("ConfigureCompiler callback was not called")
	}
	if res := NewValidator(schema).ValidateBytes([]byte(`"ab"`)); res.HasErrors() {
		t.Fatalf("custom format rejected valid value: %v", res.Collector.Errors())
	}
	if res := NewValidator(schema).ValidateBytes([]byte(`"abc"`)); !containsDiagnosticCode(res, "format") {
		t.Fatalf("custom format did not produce format diagnostic: %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaStandardFormatOverrides(t *testing.T) {
	tests := []struct {
		name   string
		format string
		valid  string
		bad    string
	}{
		{name: "email", format: "email", valid: `"\\a"@iana.org`, bad: `@example.com`},
		{name: "hostname", format: "hostname", valid: `example.com`, bad: `example.`},
		{name: "ipv4", format: "ipv4", valid: `127.0.0.1`, bad: `+1.2.3.4`},
		{name: "duration", format: "duration", valid: `P1Y2M3DT4H5M6S`, bad: `P1Y2D`},
		{name: "idn-hostname", format: "idn-hostname", valid: `실례.테스트`, bad: `a·l`},
		{name: "idn-email", format: "idn-email", valid: `δοκιμή@παράδειγμα.δοκιμή`, bad: `a..b@παράδειγμα.δοκιμή`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaData := []byte(fmt.Sprintf(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string","format":%q}`, tt.format))
			schema, err := CompileJSONSchemaWithOptions(schemaData, JSONSchemaCompileOptions{AssertFormat: true})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			validYAML := fmt.Sprintf("%q", tt.valid)
			if res := NewValidator(schema).ValidateBytes([]byte(validYAML)); res.HasErrors() {
				t.Fatalf("valid %s rejected: %v", tt.format, res.Collector.Errors())
			}
			badYAML := fmt.Sprintf("%q", tt.bad)
			if res := NewValidator(schema).ValidateBytes([]byte(badYAML)); !res.HasErrors() {
				t.Fatalf("invalid %s accepted: %s", tt.format, tt.bad)
			}
		})
	}
}

func TestCompileJSONSchemaIDNEmailEscapedYAMLEdgeCodepoints(t *testing.T) {
	schema, err := CompileJSONSchemaWithOptions([]byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "string",
		"format": "idn-email"
	}`), JSONSchemaCompileOptions{AssertFormat: true})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// JSON Schema's optional idn-email suite includes code points which are not
	// legal as raw YAML source characters. YAML escapes preserve the actual
	// Unicode scalar value and therefore exercise the JSON Schema format without
	// violating YAML's character-set rules.
	for _, input := range []string{`"\u0085@example.com"`, `"\uFFFF@example.com"`} {
		if res := NewValidator(schema).ValidateBytes([]byte(input)); res.HasErrors() {
			t.Fatalf("escaped IDN email %s rejected: %v", input, res.Collector.Errors())
		}
	}
}

func TestCompileJSONSchemaMaxDepthAppliesDuringYAMLBridge(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateWithOptions([]byte("a:\n  b:\n    c: 1\n"), ValidationContext{MaxDepth: 2})
	if !containsDiagnosticCode(res, "max_depth") {
		t.Fatalf("expected max_depth during YAML-to-JSON conversion, got %v", res.Collector.Errors())
	}
}

func TestCompileJSONSchemaPathDistinguishesNumericObjectKeysFromArrayIndexes(t *testing.T) {
	schema, err := CompileJSONSchema([]byte(`{
		"type":"object",
		"properties": {
			"0": {"type":"integer"},
			"items": {"type":"array", "items":{"type":"integer"}}
		}
	}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := NewValidator(schema).ValidateBytes([]byte("\"0\": nope\nitems: [nope]\n"))
	paths := map[string]bool{}
	for _, diagnostic := range res.Collector.Errors() {
		if diagnostic.Code == "type" {
			paths[diagnostic.Path] = true
		}
	}
	if !paths[`["0"]`] || !paths["items[0]"] {
		t.Fatalf("unexpected paths: %#v diagnostics=%v", paths, res.Collector.Errors())
	}
}

func TestCompileJSONSchemaRejectsNegativeRegexpTimeout(t *testing.T) {
	_, err := CompileJSONSchemaWithOptions([]byte(`{"type":"string","pattern":"a"}`), JSONSchemaCompileOptions{
		RegexpTimeout: -time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "regexp timeout must be non-negative") {
		t.Fatalf("expected regexp timeout error, got %v", err)
	}
}
