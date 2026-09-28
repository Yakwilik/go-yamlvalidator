package yamlvalidator_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/examples/codegen/model"
	"github.com/Yakwilik/go-yamlvalidator/genruntime"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
	"gopkg.in/yaml.v3"
)

type generatedFixtureParent struct {
	Child  model.Config            `yaml:"child"`
	List   []model.Config          `yaml:"list,omitempty"`
	Lookup map[string]model.Config `yaml:"lookup,omitempty"`
	Inline map[string]string       `yaml:",inline"`
}

type generatedFixtureManual struct{ Value string }

func (m *generatedFixtureManual) MarshalYAML() (any, error) { return "manual-" + m.Value, nil }

type generatedFixtureMixedCustom struct {
	Child  model.Config            `yaml:"child"`
	Manual *generatedFixtureManual `yaml:"manual" yamlvalidate:"type=any"`
}

type generatedFixturePromoted struct {
	model.Config `yaml:",inline"`
	Other        string `yaml:"other,omitempty"`
}

func TestCheckedInGeneratedFixture(t *testing.T) {
	var config model.Config
	if err := yamlvalidator.Unmarshal([]byte("name: fixture\nitems: [one, two]\nextra: {count: 3}\n"), &config); err != nil {
		t.Fatal(err)
	}
	if config.Name != "fixture" || len(config.Items) != 2 || config.Extra["count"] != 3 {
		t.Fatalf("%+v", config)
	}
	data, err := yamlvalidator.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: fixture") {
		t.Fatalf("%s", data)
	}
	standard, err := yaml.Marshal(config)
	if err != nil || !strings.Contains(string(standard), "name: fixture") {
		t.Fatalf("standard: %s %v", standard, err)
	}
	if err := yaml.Unmarshal([]byte("name: ''\n"), &config); err == nil {
		t.Fatal("standard generated hook skipped validation")
	} else {
		var validation *yamlvalidator.ValidationErrors
		if !errors.As(err, &validation) {
			t.Fatalf("unexpected standard error %T: %v", err, err)
		}
		if strings.Contains(validation.FormatWithSource(), "| ^") || strings.Contains(validation.FormatWithSource(), ">    1 |") {
			t.Fatalf("standard hook fabricated source: %s", validation.FormatWithSource())
		}
	}
	if err := (yamlvalidator.UnmarshalOptions{SkipValidation: true}).Unmarshal([]byte("name: ''\n"), &config); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedRecursiveNodeAndLink(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		out  any
	}{
		{"node", []byte("name: root\nchildren: [{name: leaf}]\n"), &model.Node{}},
		{"link", []byte("value: root\nnext: {value: leaf}\n"), &model.Link{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := yamlvalidator.Unmarshal(tc.in, tc.out); err != nil {
				t.Fatal(err)
			}
			encoded, err := yamlvalidator.Marshal(tc.out)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), "leaf") {
				t.Fatalf("missing recursive child: %s", encoded)
			}
			if err := (yamlvalidator.UnmarshalOptions{Limits: yamlvalidator.Limits{MaxDepth: 2}}).Unmarshal(tc.in, tc.out); err == nil {
				t.Fatal("depth limit accepted recursive input")
			}
		})
	}
	if err := yamlvalidator.Unmarshal([]byte("name: root\nchildren: [{}]\n"), &model.Node{}); err == nil {
		t.Fatal("nested required field was not validated")
	}
	for _, value := range []any{
		func() any {
			n := &model.Node{Name: "cycle"}
			n.Children = []model.Node{*n}
			n.Children[0].Children = n.Children
			return n
		}(),
		func() any { l := &model.Link{Value: "cycle"}; l.Next = l; return l }(),
	} {
		if _, err := yamlvalidator.Marshal(value); err == nil {
			t.Fatalf("cyclic %T accepted", value)
		}
	}
	for _, tc := range []struct {
		name  string
		input []byte
		out   any
	}{
		{"node", []byte("name: root\nchildren: [{name: child}]\n"), &model.Node{}},
		{"link", []byte("value: root\nnext: {value: child}\n"), &model.Link{}},
	} {
		if err := yaml.Unmarshal(tc.input, tc.out); err != nil {
			t.Fatalf("%s generated schema / standard hook: %v", tc.name, err)
		}
		if _, err := yaml.Marshal(tc.out); err != nil {
			t.Fatalf("%s generated schema / standard marshal: %v", tc.name, err)
		}
	}

}

func TestGeneratedRegistryRulesUseEmittedDeclarations(t *testing.T) {
	registry, err := yamlvalidator.NewRegistry(yamlvalidator.RegistryConfig{
		ValueValidators: map[string]yamlvalidator.ValueValidator{
			"registered": valv.EnumValidator{Allowed: []string{"allowed"}},
		},
		Schemas: map[string]*yamlvalidator.FieldSchema{
			"registeredSchema": {Type: yamlvalidator.TypeString},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var value model.Registered
	if err := (yamlvalidator.UnmarshalOptions{Registry: registry}).Unmarshal([]byte("value: allowed\n"), &value); err != nil {
		t.Fatal(err)
	}
	if value.Value != "allowed" {
		t.Fatalf("%+v", value)
	}
	if err := (yamlvalidator.UnmarshalOptions{Registry: registry}).Unmarshal([]byte("value: denied\n"), &value); err == nil {
		t.Fatal("named check skipped")
	}
	if _, err := (yamlvalidator.MarshalOptions{Registry: registry}).Marshal(model.Registered{Value: "allowed"}); err != nil {
		t.Fatal(err)
	}
	if err := yamlvalidator.Unmarshal([]byte("value: allowed\n"), &value); err == nil {
		t.Fatal("missing registry accepted")
	}
}

func TestGeneratedTypeBindingKeepsFieldRules(t *testing.T) {
	registry, err := yamlvalidator.NewRegistry(yamlvalidator.RegistryConfig{TypeBindings: map[reflect.Type]yamlvalidator.TypeBinding{
		reflect.TypeFor[string](): {Decode: &yamlvalidator.FieldSchema{Type: yamlvalidator.TypeString}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"items: [one]\n", "name: ''\nitems: [one]\n"} {
		var value model.Config
		if err := (yamlvalidator.UnmarshalOptions{Registry: registry}).Unmarshal([]byte(input), &value); err == nil {
			t.Fatalf("binding discarded generated field rules for %q", input)
		}
	}
}

func TestCheckedInGeneratedMixedParent(t *testing.T) {
	var parent generatedFixtureParent
	input := []byte("child: {name: ok, unknown: allowed}\nlist: [{name: first}]\nlookup: {one: {name: second}}\nextra: value\n")
	if err := (yamlvalidator.UnmarshalOptions{UnknownKeyPolicy: yamlvalidator.UnknownKeyIgnore}).Unmarshal(input, &parent); err != nil {
		t.Fatal(err)
	}
	if parent.Child.Name != "ok" || parent.List[0].Name != "first" || parent.Lookup["one"].Name != "second" || parent.Inline["extra"] != "value" || len(parent.Inline) != 1 {
		t.Fatalf("%+v", parent)
	}
	parent.Child.Name = ""
	if _, err := (yamlvalidator.MarshalOptions{SkipValidation: true}).Marshal(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := yamlvalidator.Marshal(parent); err == nil {
		t.Fatal("nested validation skipped")
	}
	parent.Child.Name = "ok"
	data, err := yamlvalidator.Marshal(parent)
	if err != nil || !strings.Contains(string(data), "extra: value") {
		t.Fatalf("%s %v", data, err)
	}
	custom := generatedFixtureMixedCustom{Child: model.Config{Name: "ok"}, Manual: &generatedFixtureManual{Value: "yes"}}
	data, err = yamlvalidator.Marshal(custom)
	if err != nil || !strings.Contains(string(data), "manual-yes") {
		t.Fatalf("%s %v", data, err)
	}
}

func TestCheckedInGeneratedCollectionsAndOpaqueOptions(t *testing.T) {
	type plainContainers model.Containers
	raw, err := yamlvalidator.Marshal(plainContainers{})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := yamlvalidator.Marshal(model.Containers{})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(generated) {
		t.Fatalf("nil containers differ: raw=%s generated=%s", raw, generated)
	}
	for _, opaque := range []any{[]model.Config{{Name: ""}}, map[string]model.Config{"one": {Name: ""}}} {
		if _, err := (yamlvalidator.MarshalOptions{SkipValidation: true}).Marshal(model.Dynamic{Value: opaque}); err != nil {
			t.Fatalf("opaque %T: %v", opaque, err)
		}
	}
}

func TestCheckedInGeneratedDecodeLimits(t *testing.T) {
	input := []byte("name: okay\nitems: [first]\n")
	for _, limits := range []yamlvalidator.Limits{{MaxNodeVisits: 1}, {MaxDepth: 1}} {
		var config model.Config
		if err := (yamlvalidator.UnmarshalOptions{SkipValidation: true, Limits: limits}).Unmarshal(input, &config); err == nil {
			t.Fatalf("limit %+v ignored", limits)
		}
	}
	var config model.Config
	if err := (yamlvalidator.UnmarshalOptions{SkipValidation: true, Limits: yamlvalidator.Limits{MaxNodeVisits: 100, MaxDepth: 20}}).Unmarshal(input, &config); err != nil {
		t.Fatal(err)
	}
}

func TestCheckedInGeneratedPromotionUsesOuterShape(t *testing.T) {
	var outer generatedFixturePromoted
	if err := yamlvalidator.Unmarshal([]byte("name: okay\nother: kept\n"), &outer); err != nil {
		t.Fatal(err)
	}
	if outer.Name != "okay" || outer.Other != "kept" {
		t.Fatalf("%+v", outer)
	}
	data, err := yamlvalidator.Marshal(outer)
	if err != nil || !strings.Contains(string(data), "other: kept") {
		t.Fatalf("%s %v", data, err)
	}
}

func TestGeneratedAdapterRejectsNilNode(t *testing.T) {
	if err := genruntime.UnmarshalYAML(nil, &model.Config{}); err == nil {
		t.Fatal("nil YAML node accepted")
	}
}
