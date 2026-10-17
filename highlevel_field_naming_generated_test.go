package yamlvalidator_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/examples/codegen/model"
	"github.com/Yakwilik/go-yamlvalidator/genruntime"
	"github.com/Yakwilik/go-yamlvalidator/genruntime/spec"
	"gopkg.in/yaml.v3"
)

// This deliberately implements the old generated interfaces, including the
// context decoder that still hard-codes lowercase names.
type legacyFieldNames struct{ ImageName string }

func (legacyFieldNames) YAMLValidatorGeneratedType() reflect.Type {
	return reflect.TypeFor[legacyFieldNames]()
}
func (legacyFieldNames) YAMLValidatorSchemaSpec() (spec.Graph, []reflect.Type) {
	return spec.Graph{Nodes: []spec.Node{{Type: spec.NodeType(v.TypeMap), HasAllowedKeys: true, AllowedKeys: map[string]int{"imagename": 1}}, {Type: spec.NodeType(v.TypeString)}}}, nil
}
func (c legacyFieldNames) YAMLValidatorEncode() (*yaml.Node, error) {
	var node yaml.Node
	err := node.Encode(map[string]string{"imagename": c.ImageName})
	return &node, err
}
func (c *legacyFieldNames) YAMLValidatorDecode(node *yaml.Node) error {
	var plain struct{ ImageName string }
	if err := node.Decode(&plain); err != nil {
		return err
	}
	c.ImageName = plain.ImageName
	return nil
}
func (c *legacyFieldNames) YAMLValidatorDecodeWithContext(node *yaml.Node, _ *genruntime.DecodeContext) error {
	return c.YAMLValidatorDecode(node)
}
func (c legacyFieldNames) MarshalYAML() (any, error) { return genruntime.MarshalYAML(c) }
func (c *legacyFieldNames) UnmarshalYAML(node *yaml.Node) error {
	return genruntime.UnmarshalYAML(node, c)
}

func TestFieldNamingLegacyGeneratedFallback(t *testing.T) {
	t.Parallel()
	type parent struct {
		Child legacyFieldNames
		List  []legacyFieldNames
	}
	for _, skip := range []bool{false, true} {
		var got parent
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("Child: {ImageName: child}\nList: [{ImageName: list}]\n"), &got); err != nil {
			t.Fatal(err)
		}
		if got.Child.ImageName != "child" || got.List[0].ImageName != "list" {
			t.Fatalf("legacy mixed fallback: %#v", got)
		}
		data, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Marshal(got)
		if err != nil || !strings.Contains(string(data), "ImageName: child") {
			t.Fatalf("legacy marshal: %s %v", data, err)
		}
		var root legacyFieldNames
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("ImageName: root\n"), &root); err != nil {
			t.Fatal(err)
		}
		if root.ImageName != "root" {
			t.Fatalf("legacy root: %#v", root)
		}
		if _, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Marshal(root); err != nil {
			t.Fatal(err)
		}
	}
	var standard legacyFieldNames
	if err := v.Unmarshal([]byte("imagename: standard\n"), &standard); err != nil || standard.ImageName != "standard" {
		t.Fatalf("default legacy: %#v %v", standard, err)
	}
	data, err := yaml.Marshal(standard)
	if err != nil || !strings.Contains(string(data), "imagename: standard") {
		t.Fatalf("legacy standard hook: %s %v", data, err)
	}
}

func TestFieldNamingGeneratedAndMixedTrees(t *testing.T) {
	t.Parallel()
	original := model.FieldNames{ImageName: "root", LiteralName: "literal", JSONName: "json", Child: &model.NamingChild{ImageName: "child"}, Children: []model.NamingChild{{ImageName: "list"}}, Array: [1]model.NamingChild{{ImageName: "array"}}, Workers: map[string]*model.NamingChild{"product_UP": {ImageName: "upper"}, "product_up": {ImageName: "lower"}}, NamingInline: model.NamingInline{InlineName: "inline"}, Extra: map[string]string{"extra_KEY": "extra"}}
	registry, err := v.NewRegistry(v.RegistryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	run := func(naming v.FieldNaming, skip bool) {
		t.Helper()
		data, err := (v.MarshalOptions{FieldNaming: naming, SkipValidation: skip, Registry: registry}).Marshal(original)
		if err != nil {
			t.Error(err)
			return
		}
		var got model.FieldNames
		if err := (v.UnmarshalOptions{FieldNaming: naming, SkipValidation: skip, Registry: registry}).Unmarshal(data, &got); err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(original, got) {
			t.Errorf("generated roundtrip mode %d: %#v", naming, got)
		}
		type parent struct {
			Child  model.FieldNames
			List   []model.FieldNames
			Lookup map[string]model.FieldNames
		}
		mixed := parent{Child: original, List: []model.FieldNames{original}, Lookup: map[string]model.FieldNames{"product_UP": original}}
		data, err = (v.MarshalOptions{FieldNaming: naming, SkipValidation: skip, Registry: registry}).Marshal(mixed)
		if err != nil {
			t.Error(err)
			return
		}
		var mixedGot parent
		if err := (v.UnmarshalOptions{FieldNaming: naming, SkipValidation: skip, Registry: registry}).Unmarshal(data, &mixedGot); err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(mixed, mixedGot) {
			t.Errorf("mixed roundtrip mode %d: %#v", naming, mixedGot)
		}
	}
	for _, naming := range []v.FieldNaming{v.FieldNamingYAML, v.FieldNamingGo, v.FieldNamingYAML} {
		for _, skip := range []bool{false, true} {
			run(naming, skip)
		}
	}
	var group sync.WaitGroup
	for i := range 12 {
		group.Add(1)
		go func() { defer group.Done(); run(v.FieldNaming(i%2), false) }()
	}
	group.Wait()
	data, err := yaml.Marshal(original)
	if err != nil || !strings.Contains(string(data), "imagename: root") {
		t.Fatalf("standard generated hook: %s %v", data, err)
	}
}

func TestFieldNamingGeneratedConflicts(t *testing.T) {
	t.Parallel()
	for _, skip := range []bool{false, true} {
		got := model.GoNamingCollision{Field: "original"}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("Field: changed\n"), &got); err == nil {
			t.Fatal("generated conflict accepted")
		}
		if got.Field != "original" {
			t.Fatal("generated conflict mutated destination")
		}
		// Conflicts in a nested generated value are found before preceding fields decode.
		parent := struct {
			Before string
			Child  model.GoNamingCollision
		}{Before: "original"}
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("Before: changed\nChild: {Field: changed}\n"), &parent); err == nil {
			t.Fatal("nested generated conflict accepted")
		}
		if parent.Before != "original" {
			t.Fatal("nested conflict mutated preceding field")
		}
		var distinct model.YAMLCollision
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("ID: upper\nId: mixed\n"), &distinct); err != nil {
			t.Fatal(err)
		}
		if distinct.ID != "upper" || distinct.Id != "mixed" {
			t.Fatalf("mode-specific keys: %#v", distinct)
		}
	}
	var rules model.GoRuleNames
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("Mode: prod\nToken: token\n"), &rules); err != nil {
		t.Fatal(err)
	}
	if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo}).Unmarshal([]byte("Mode: prod\n"), &rules); err == nil {
		t.Fatal("generated literal conditional ignored")
	}
}

func TestFieldNamingGeneratedYAMLConflictUnchecked(t *testing.T) {
	t.Parallel()
	got := model.YAMLCollision{ID: "original"}
	if err := (v.UnmarshalOptions{SkipValidation: true}).Unmarshal([]byte("id: changed\n"), &got); err == nil {
		t.Fatal("unchecked generated YAML conflict accepted")
	}
	if got.ID != "original" {
		t.Fatal("unchecked generated conflict mutated destination")
	}
}

func TestFieldNamingInlineGeneratedMapCapture(t *testing.T) {
	t.Parallel()
	type parent struct {
		Inline     model.FieldNames `yaml:",inline"`
		ParentName string
	}
	for _, skip := range []bool{true} {
		var got parent
		if err := (v.UnmarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Unmarshal([]byte("ImageName: image\nParentName: parent\nextra_data: extra\n"), &got); err != nil {
			t.Fatal(err)
		}
		if got.ParentName != "parent" || !reflect.DeepEqual(got.Inline.Extra, map[string]string{"extra_data": "extra"}) {
			t.Fatalf("inline capture: %#v", got)
		}
		if _, err := (v.MarshalOptions{FieldNaming: v.FieldNamingGo, SkipValidation: skip}).Marshal(got); err != nil {
			t.Fatal(err)
		}
	}
}
