package recipes_test

import (
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestJSONSchemaStandardRules(t *testing.T) {
	tests := []struct{ name, schema, good, bad, code string }{
		{"types", `{"type":["string","null"]}`, `null`, `12`, "type"},
		{"enum", `{"enum":["dev","prod"]}`, `dev`, `stage`, "enum"},
		{"const", `{"const":"prod"}`, `prod`, `dev`, "const"},
		{"pattern", `{"type":"string","pattern":"^[a-z]+$","minLength":2}`, `api`, `API`, "pattern"},
		{"numeric", `{"type":"number","multipleOf":2,"minimum":0}`, `4`, `3`, "multipleOf"},
		{"object", `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`, `{name: api}`, `{name: api, extra: true}`, "additionalProperties"},
		{"key names", `{"type":"object","propertyNames":{"pattern":"^[a-z]+$"}}`, `{name: x}`, `{Name: x}`, "pattern"},
		{"pattern properties", `{"type":"object","patternProperties":{"^x-":{"type":"integer"}},"additionalProperties":false}`, `{x-count: 1}`, `{x-count: bad}`, "type"},
		{"dependencies", `{"type":"object","dependentRequired":{"username":["password"]}}`, `{username: x, password: y}`, `{username: x}`, "dependentRequired"},
		{"dependent schemas", `{"type":"object","dependentSchemas":{"enabled":{"required":["url"]}}}`, `{enabled: true, url: local}`, `{enabled: true}`, "required"},
		{"items", `{"type":"array","items":{"type":"integer"},"uniqueItems":true}`, `[1,2]`, `[1,1]`, "uniqueItems"},
		{"tuple", `{"type":"array","prefixItems":[{"type":"string"},{"type":"integer"}],"items":false}`, `[name,2]`, `[name,bad]`, "type"},
		{"contains", `{"type":"array","contains":{"type":"integer"},"minContains":2}`, `[1,2]`, `[x,1]`, "minContains"},
		{"allOf", `{"allOf":[{"type":"integer"},{"minimum":2}]}`, `2`, `1`, "minimum"},
		{"anyOf", `{"anyOf":[{"type":"integer"},{"type":"string"}]}`, `name`, `true`, "anyOf"},
		{"oneOf", `{"oneOf":[{"type":"integer"},{"type":"number"}]}`, `1.5`, `1`, "oneOf"},
		{"not", `{"not":{"const":"reserved"}}`, `api`, `reserved`, "not"},
		{"conditional", `{"type":"object","if":{"properties":{"mode":{"const":"remote"}},"required":["mode"]},"then":{"required":["url"]},"else":{"required":["file"]}}`, `{mode: remote, url: local}`, `{mode: remote}`, "required"},
		{"unevaluated properties", `{"type":"object","allOf":[{"properties":{"name":{"type":"string"}}}],"unevaluatedProperties":false}`, `{name: api}`, `{name: api, extra: 1}`, ""},
		{"unevaluated items", `{"prefixItems":[{"type":"integer"}],"unevaluatedItems":false}`, `[1]`, `[1,2]`, ""},
		{"recursive ref", `{"$defs":{"node":{"type":"object","properties":{"name":{"type":"string"},"children":{"type":"array","items":{"$ref":"#/$defs/node"}}},"required":["name"]}},"$ref":"#/$defs/node"}`, `{name: a, children: [{name: b}]}`, `{name: a, children: [{}]}`, "required"},
		{"anchor", `{"$defs":{"name":{"$anchor":"name","type":"string"}},"$ref":"#name"}`, `api`, `12`, "type"},
		{"dynamic recursive", `{"$dynamicAnchor":"node","type":"object","properties":{"name":{"type":"string"},"children":{"type":"array","items":{"$dynamicRef":"#node"}}},"required":["name"]}`, `{name: a, children: [{name: b}]}`, `{name: a, children: [{}]}`, "required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := compileJSON(t, tc.schema, v.JSONSchemaCompileOptions{})
			checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: tc.good, valid: true}, {name: "invalid", input: tc.bad, code: tc.code}})
		})
	}
}

func TestJSONSchemaDialectsAndOptions(t *testing.T) {
	tests := []struct {
		name   string
		draft  v.JSONSchemaDraft
		source string
	}{
		{"draft4", v.JSONSchemaDraft4, `{"type":"number","minimum":1,"exclusiveMinimum":true}`},
		{"draft6", v.JSONSchemaDraft6, `{"type":"number","exclusiveMinimum":1}`},
		{"draft7", v.JSONSchemaDraft7, `{"type":"number","exclusiveMinimum":1}`},
		{"2019", v.JSONSchemaDraft2019, `{"type":"number","exclusiveMinimum":1}`},
		{"2020", v.JSONSchemaDraft2020, `{"type":"number","exclusiveMinimum":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkInputs(t, compileJSON(t, tc.source, v.JSONSchemaCompileOptions{DefaultDraft: tc.draft}), v.ValidationContext{}, []inputCase{
				{name: "valid", input: "2", valid: true}, {name: "boundary", input: "1", code: "exclusiveMinimum"},
			})
		})
	}
	checkInputs(t, compileJSON(t, `{"$schema":"http://json-schema.org/draft-04/schema#","minimum":1,"exclusiveMinimum":true}`, v.JSONSchemaCompileOptions{DefaultDraft: v.JSONSchemaDraft2020}), v.ValidationContext{}, []inputCase{
		{name: "declared draft wins", input: "1", code: "exclusiveMinimum"},
	})
	checkInputs(t, compileJSON(t, `false`, v.JSONSchemaCompileOptions{}), v.ValidationContext{}, []inputCase{{name: "false schema", input: "name", code: "falseSchema"}})
	for _, assert := range []bool{false, true} {
		validator := compileJSON(t, `{"type":"string","format":"email"}`, v.JSONSchemaCompileOptions{AssertFormat: assert})
		checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "email", input: "not-an-email", valid: !assert}})
	}
	warn := v.UnknownKeyWarn
	validator := compileJSON(t, `{"type":"object","additionalProperties":false}`, v.JSONSchemaCompileOptions{AdditionalPropertiesFalsePolicy: &warn})
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "warning policy", input: "extra: value", valid: true, code: "additionalProperties", warnings: 1}})
	if _, err := v.CompileJSONSchema([]byte(`{"type":123}`)); err == nil {
		t.Fatal("expected schema compilation failure")
	}
	schema, err := v.CompileJSONSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	schema.Required = true
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{{name: "empty stream", code: "required_document"}})
}
