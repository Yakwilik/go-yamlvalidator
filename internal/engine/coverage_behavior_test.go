package yamlvalidator

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidationContextHelpers(t *testing.T) {
	ctx := NewValidationContext()
	if ctx == nil || ctx.Collector() == nil || ctx.IsStopped() {
		t.Fatal("invalid default validation context")
	}
	if ctx.Context() == nil || ctx.ContextErr() != nil {
		t.Fatal("invalid default run context")
	}
	schema := &FieldSchema{Type: TypeString}
	result, err := NewValidator(schema).ValidateContextWithOptions(
		context.Background(), []byte("value"), ValidationContext{StopOnFirst: true},
	)
	if err != nil || result.HasErrors() {
		t.Fatalf("context validation failed: result=%v err=%v", result, err)
	}
}

func TestInterFieldRelationshipBranches(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"token":    {Type: TypeString},
			"username": {Type: TypeString},
			"password": {Type: TypeString},
		},
		OneOfRequired: [][]string{{"token"}, {"username", "password"}},
		ForbiddenTogether: [][]string{
			{"token", "username"},
			{"token", "password"},
		},
		DependentRequired: map[string][]string{
			"username": {"password"},
			"password": {"username"},
		},
	}
	validator := NewValidator(schema)
	cases := []struct {
		input     string
		wantError bool
	}{
		{"token: t", false},
		{"username: u\npassword: p", false},
		{"username: u", true},
		{"token: t\nusername: u", true},
		{"token: t\npassword: p", true},
		{"token: t\nusername: u\npassword: p", true},
		{"{}", true},
	}
	for _, tc := range cases {
		if got := validator.ValidateBytes([]byte(tc.input)).HasErrors(); got != tc.wantError {
			t.Errorf("input %q errors=%v want %v", tc.input, got, tc.wantError)
		}
	}
}

func TestSchemaAlternativeBranches(t *testing.T) {
	one := NewValidator(&FieldSchema{
		Type: TypeAny,
		OneOfSchemas: []*FieldSchema{
			{Type: TypeString},
			{Type: TypeInt},
		},
	})
	if result := one.ValidateBytes([]byte("hello")); result.HasErrors() {
		t.Fatalf("string alternative rejected: %v", result.Collector.Errors())
	}
	if result := one.ValidateBytes([]byte("true")); !result.HasErrors() {
		t.Fatal("value matching no alternative accepted")
	}

	ambiguous := NewValidator(&FieldSchema{
		Type: TypeAny,
		OneOfSchemas: []*FieldSchema{
			{Type: TypeInt},
			{Type: TypeFloat},
		},
	})
	if result := ambiguous.ValidateBytes([]byte("1")); !result.HasErrors() {
		t.Fatal("value matching two oneOf branches accepted")
	}
}

func TestPathAndDraftHelpers(t *testing.T) {
	if got := joinJSONSchemaInstancePath("root", []string{"items", "0", "a.b"}); got != "root.items[0][\"a.b\"]" {
		t.Fatalf("unexpected path %q", got)
	}
	for _, draft := range []JSONSchemaDraft{
		JSONSchemaDraft4,
		JSONSchemaDraft6,
		JSONSchemaDraft7,
		JSONSchemaDraft2019,
		JSONSchemaDraft2020,
		"",
	} {
		if _, err := resolveJSONSchemaDraft(draft); err != nil {
			t.Errorf("draft %q rejected: %v", draft, err)
		}
	}
	if _, err := resolveJSONSchemaDraft("future"); err == nil {
		t.Fatal("unknown draft accepted")
	}
}

func TestInferNodeTypeBranches(t *testing.T) {
	cases := []struct {
		node *yaml.Node
		want NodeType
	}{
		{&yaml.Node{Kind: yaml.MappingNode}, TypeMap},
		{&yaml.Node{Kind: yaml.SequenceNode}, TypeSequence},
		{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x"}, TypeString},
		{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"}, TypeInt},
		{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: "1.5"}, TypeFloat},
		{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}, TypeBool},
		{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, TypeNull},
	}
	for _, tc := range cases {
		if got := InferNodeType(tc.node, nil); got != tc.want {
			t.Errorf("node %#v type=%v want %v", tc.node, got, tc.want)
		}
	}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "2"}}
	if got := InferNodeType(alias, nil); got != TypeInt {
		t.Fatalf("alias type=%v", got)
	}
}
