package yamlvalidator

import (
	"reflect"
	"strconv"
	"testing"

	genspec "github.com/Yakwilik/go-yamlvalidator/genruntime/spec"
)

func generatedTestRules(t *testing.T, source string) []genspec.Rule {
	t.Helper()
	rules, err := parseTagRules(source)
	if err != nil {
		t.Fatal(err)
	}
	var convertValue func(tagValue) genspec.RuleValue
	var convertRules func([]tagRule) []genspec.Rule
	convertValue = func(v tagValue) genspec.RuleValue {
		out := genspec.RuleValue{Kind: v.kind, Text: v.text}
		for _, child := range v.list {
			out.List = append(out.List, convertValue(child))
		}
		out.Rules = convertRules(v.rules)
		return out
	}
	convertRules = func(rs []tagRule) []genspec.Rule {
		out := make([]genspec.Rule, len(rs))
		for i, r := range rs {
			out[i] = genspec.Rule{Key: r.key, Value: convertValue(r.value), HasValue: r.hasValue, Offset: r.offset}
		}
		return out
	}
	return convertRules(rules)
}

func TestGeneratedNormalizedGraphContainsLoweredConstraints(t *testing.T) {
	graph := genspec.SourceGraph{Nodes: []genspec.SourceNode{
		{Type: genspec.NodeType(TypeMap), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1, Fields: []genspec.SourceField{
			{Key: "mode", Node: 1, FieldName: "Mode", Rules: generatedTestRules(t, "when={field=mode,eq=on,require=[items]}")},
			{Key: "count", Node: 2, FieldName: "Count", Rules: generatedTestRules(t, "min=1,max=3")},
			{Key: "items", Node: 3, FieldName: "Items", Rules: generatedTestRules(t, "minItems=1,items={enum=[red,blue]}")},
		}},
		{Type: genspec.NodeType(TypeString), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1},
		{Type: genspec.NodeType(TypeInt), NumberKind: reflect.Int, NumberBits: strconv.IntSize, AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1},
		{Type: genspec.NodeType(TypeSequence), Nullable: true, Item: 1, AliasOf: -1, Value: -1, ArrayLen: -1},
	}}
	lowered, err := LowerGeneratedSchema(graph, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lowered.Nodes[lowered.Root].Conditions) != 1 {
		t.Fatalf("condition not lowered: %+v", lowered.Nodes[lowered.Root])
	}
	schema, err := BuildGeneratedSchema(lowered, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		yaml  string
		valid bool
	}{
		{"mode: on\ncount: 2\nitems: [red]\n", true},
		{"mode: on\ncount: 5\n", false},
		{"mode: off\ncount: 2\nitems: [green]\n", false},
	} {
		if got := !compiled.ValidateBytes([]byte(tc.yaml)).HasErrors(); got != tc.valid {
			t.Fatalf("valid=%t for %q, want %t", got, tc.yaml, tc.valid)
		}
	}
}

func TestGeneratedNormalizedGraphResolvesNamedAlternatives(t *testing.T) {
	graph := genspec.SourceGraph{Nodes: []genspec.SourceNode{
		{Type: genspec.NodeType(TypeMap), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1, Fields: []genspec.SourceField{{Key: "value", Node: 1, FieldName: "Value", Rules: generatedTestRules(t, "oneOfSchemas=[named]")}}},
		{Type: genspec.NodeType(TypeString), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1},
	}}
	lowered, err := LowerGeneratedSchema(graph, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildGeneratedSchema(lowered, nil, nil, false); err == nil {
		t.Fatal("missing named schema accepted")
	}
	registry, err := NewRegistry(RegistryConfig{Schemas: map[string]*FieldSchema{"named": {Type: TypeString, Validators: []ValueValidator{enumRule{"ok"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := BuildGeneratedSchema(lowered, nil, registry, false)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	if result := compiled.ValidateBytes([]byte("value: ok\n")); result.HasErrors() {
		t.Fatalf("named alternative rejected: %s", result.FormatAll(true))
	}
	if result := compiled.ValidateBytes([]byte("value: bad\n")); !result.HasErrors() {
		t.Fatal("named alternative skipped")
	}
}
