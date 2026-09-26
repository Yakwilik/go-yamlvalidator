package recipes_test

import (
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func TestNativeUnknownPolicies(t *testing.T) {
	tests := []struct {
		name     string
		policy   v.UnknownKeyPolicy
		strict   bool
		valid    bool
		warnings int
	}{
		{"default warns", v.UnknownKeyInherit, false, true, 1},
		{"strict inherited errors", v.UnknownKeyInherit, true, false, 0},
		{"explicit errors", v.UnknownKeyError, false, false, 0},
		{"explicit warning", v.UnknownKeyWarn, true, true, 1},
		{"explicit ignore", v.UnknownKeyIgnore, true, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := stringFields("name")
			schema.UnknownKeyPolicy = tc.policy
			checkInputs(t, v.NewValidator(schema), v.ValidationContext{StrictKeys: tc.strict}, []inputCase{
				{name: "extra", input: "name: api\nextra: value\n", valid: tc.valid, warnings: tc.warnings},
			})
		})
	}
}

func TestNativePresenceNullAndDefaults(t *testing.T) {
	schema := &v.FieldSchema{
		Type: v.TypeMap, Required: true, UnknownKeyPolicy: v.UnknownKeyError,
		AllowedKeys: map[string]*v.FieldSchema{
			"name": {Type: v.TypeString, Required: true, Validators: []v.ValueValidator{valv.NonEmptyValidator{}}},
			"note": {Type: v.TypeString, Nullable: true},
		},
	}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{
		{name: "valid", input: "name: api\nnote: null\n", valid: true},
		{name: "missing", input: "{}", code: "required"},
		{name: "empty", input: "name: \"\"", code: "non_empty"},
		{name: "null", input: "name: null", code: "type_mismatch"},
		{name: "empty stream", input: "", code: "required_document"},
	})
	schema.AllowedKeys["mode"] = &v.FieldSchema{Type: v.TypeString, Default: "dev"}
	schema.AllowedKeys["legacy"] = &v.FieldSchema{Type: v.TypeString, Deprecated: "use name"}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{
		{name: "default warns only", input: "name: api", valid: true, code: "default", warnings: 1},
		{name: "deprecated warns", input: "name: api\nmode: dev\nlegacy: old", valid: true, code: "deprecated", warnings: 1},
	})
}

func TestNativeContainersAndUnions(t *testing.T) {
	schema := &v.FieldSchema{
		Type: v.TypeMap, UnknownKeyPolicy: v.UnknownKeyError,
		AllowedKeys: map[string]*v.FieldSchema{
			"labels":  {Type: v.TypeMap, AdditionalProperties: &v.FieldSchema{Type: v.TypeString}},
			"workers": {Type: v.TypeSequence, MinItems: v.Ptr(1), MaxItems: v.Ptr(2), ItemSchema: &v.FieldSchema{Type: v.TypeString}},
			"choice":  {AllowedTypes: []v.NodeType{v.TypeString, v.TypeInt}},
			"opaque":  {Type: v.TypeAny},
		},
	}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{
		{name: "valid", input: "labels: {team: core}\nworkers: [a]\nchoice: 2\nopaque: {nested: [1, true]}", valid: true},
		{name: "label type", input: "labels: {team: 12}", code: "type_mismatch"},
		{name: "item type", input: "workers: [12]", code: "type_mismatch"},
		{name: "min", input: "workers: []", code: "min_items"},
		{name: "max", input: "workers: [a, b, c]", code: "max_items"},
		{name: "union type", input: "choice: true", code: "type_mismatch"},
	})
	anySchema := &v.FieldSchema{Type: v.TypeAny}
	checkInputs(t, v.NewValidator(anySchema), v.ValidationContext{}, []inputCase{
		{name: "bare any", input: "anything: [a, 1]", valid: true},
	})
	checkInputs(t, v.NewValidator(&v.FieldSchema{Type: v.TypeMap}), v.ValidationContext{}, []inputCase{
		{name: "map with no known keys", input: "anything: 1", valid: true, warnings: 1},
	})
}

func TestNativeFieldRelationships(t *testing.T) {
	tests := []struct {
		name  string
		build func() *v.FieldSchema
		cases []inputCase
	}{
		{"mutually exclusive", func() *v.FieldSchema {
			s := stringFields("file", "url")
			s.MutuallyExclusive = []string{"file", "url"}
			return s
		}, []inputCase{
			{name: "zero", input: "{}", valid: true}, {name: "one", input: "file: local", valid: true},
			{name: "two", input: "file: local\nurl: remote", code: "mutually_exclusive"},
		}},
		{"exactly one", func() *v.FieldSchema {
			s := stringFields("file", "url")
			s.ExactlyOneOf = []string{"file", "url"}
			return s
		}, []inputCase{
			{name: "zero", input: "{}", code: "exactly_one_of"}, {name: "one", input: "url: remote", valid: true},
		}},
		{"at least one complete group", func() *v.FieldSchema {
			s := stringFields("file", "host", "port")
			s.AnyOf = [][]string{{"file"}, {"host", "port"}}
			return s
		}, []inputCase{
			{name: "complete", input: "host: local\nport: \"80\"", valid: true},
			{name: "incomplete", input: "host: local", code: "any_of_required"},
		}},
		{"auth groups and mixtures", func() *v.FieldSchema {
			s := stringFields("token", "username", "password")
			s.OneOfRequired = [][]string{{"token"}, {"username", "password"}}
			s.ForbiddenTogether = [][]string{{"token", "username"}, {"token", "password"}}
			s.DependentRequired = map[string][]string{"username": {"password"}, "password": {"username"}}
			return s
		}, []inputCase{
			{name: "token", input: "token: example", valid: true},
			{name: "pair", input: "username: example\npassword: example", valid: true},
			{name: "mixed incomplete", input: "token: example\nusername: example", code: "forbidden_together"},
			{name: "missing dependency", input: "username: example", code: "dependent_required"},
			{name: "none", input: "{}", code: "one_of_required"},
		}},
		{"conditional", func() *v.FieldSchema {
			s := stringFields("enabled", "endpoint", "legacy")
			s.AllowedKeys["enabled"].Type = v.TypeBool
			s.Conditions = []v.ConditionalRule{{ConditionField: "enabled", ConditionValue: "true", ThenRequired: []string{"endpoint"}, ThenForbidden: []string{"legacy"}}}
			return s
		}, []inputCase{
			{name: "off", input: "enabled: false", valid: true},
			{name: "on complete", input: "enabled: true\nendpoint: local", valid: true},
			{name: "on missing", input: "enabled: true", code: "condition_required"},
			{name: "on forbidden", input: "enabled: true\nendpoint: local\nlegacy: old", code: "condition_forbidden"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := tc.build()
			if err := v.ValidateFieldSchema(schema); err != nil {
				t.Fatal(err)
			}
			checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, tc.cases)
		})
	}
}

func TestNativeSchemaAlternatives(t *testing.T) {
	schema := &v.FieldSchema{Type: v.TypeAny, OneOfSchemas: []*v.FieldSchema{
		{Type: v.TypeString}, {Type: v.TypeSequence, ItemSchema: &v.FieldSchema{Type: v.TypeString}},
	}}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{
		{name: "string", input: "local", valid: true}, {name: "list", input: "[local]", valid: true},
		{name: "wrong", input: "12", code: "one_of_schema"},
	})
	schema = &v.FieldSchema{AnyOfSchemas: []*v.FieldSchema{{Type: v.TypeString}, {Type: v.TypeInt}}}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{
		{name: "int", input: "12", valid: true}, {name: "wrong", input: "true", code: "any_of_schema"},
	})
}
