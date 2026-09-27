package yamlvalidator_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/examples/codegen/model"
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
