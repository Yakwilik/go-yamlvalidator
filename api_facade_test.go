package yamlvalidator_test

import (
	"errors"
	v "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/genruntime"
	"github.com/Yakwilik/go-yamlvalidator/genruntime/spec"
	"github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
	"gopkg.in/yaml.v3"
	"reflect"
	"sync/atomic"
	"testing"
)

// A real external validator must still satisfy both validation and cloning
// contracts through the original root import, without internal package imports.
type externalProbe struct{}

func (externalProbe) Validate(n *yaml.Node, path string, c *v.ValidationContext) {
	if n.Value == "bad" {
		c.AddError(v.ValidationError{Level: v.LevelError, Code: "external", Path: path, Line: n.Line, Column: n.Column, Message: "invalid external value"})
	}
}
func (externalProbe) CloneValueValidator() v.ValueValidator { return externalProbe{} }

var _ v.ValueValidator = externalProbe{}
var _ v.ValueValidatorCloner = externalProbe{}
var _ v.ValueValidator = valuevalidator.NonEmptyValidator{}

func TestPublicFacadeCustomValidatorAndErrors(t *testing.T) {
	sch := &v.FieldSchema{Type: v.TypeMap, AllowedKeys: map[string]*v.FieldSchema{"name": {Type: v.TypeString, Required: true, Validators: []v.ValueValidator{externalProbe{}}}}}
	compiled, err := v.CompileFieldSchema(sch)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.ValidateBytes([]byte("name: ok\n")).HasErrors() {
		t.Fatal("valid external model rejected")
	}
	if !compiled.ValidateBytes([]byte("name: bad\n")).HasErrors() {
		t.Fatal("custom rule not invoked")
	}
	var dst struct {
		Name string `yaml:"name" yamlvalidate:"required,nonempty"`
	}
	err = v.Unmarshal([]byte("name: ''\n"), &dst)
	var validation *v.ValidationErrors
	if !errors.As(err, &validation) || len(validation.Diagnostics()) == 0 {
		t.Fatalf("root errors.As failed: %T %v", err, err)
	}
	// Keep useful display names even though canonical reflection package paths
	// now identify the implementation package. The migration is documented.
	if got := reflect.TypeFor[v.FieldSchema]().String(); got != "yamlvalidator.FieldSchema" {
		t.Fatalf("unexpected type display: %s", got)
	}
}

func TestGeneratedAdaptersRejectInvalidReceivers(t *testing.T) {
	if _, err := genruntime.MarshalYAML(struct{}{}); err == nil {
		t.Fatal("ordinary value accepted as generated encoder")
	}
	if err := genruntime.UnmarshalYAML(&yaml.Node{}, nil); err == nil {
		t.Fatal("nil receiver accepted")
	}
	var dst *int
	if err := genruntime.UnmarshalYAML(&yaml.Node{}, dst); err == nil {
		t.Fatal("typed nil receiver accepted")
	}
	if err := genruntime.UnmarshalYAML(&yaml.Node{}, new(int)); err == nil {
		t.Fatal("ordinary destination accepted")
	}
}

// Both public entry points must reach one engine/cache, not two copied
// validation implementations introduced by the package split.
type sharedPlanProbe struct {
	Name string `yaml:"name" yamlvalidate:"not_a_real_rule"`
}

var sharedPlanCompiles atomic.Int64

func (sharedPlanProbe) YAMLValidatorGeneratedType() reflect.Type {
	return reflect.TypeFor[sharedPlanProbe]()
}
func (sharedPlanProbe) YAMLValidatorSchemaSpec() (spec.Graph, []reflect.Type) {
	sharedPlanCompiles.Add(1)
	return spec.Graph{Nodes: []spec.Node{
		{Type: spec.NodeType(v.TypeMap), HasAllowedKeys: true, AllowedKeys: map[string]int{"name": 1}},
		{Type: spec.NodeType(v.TypeString), Required: true},
	}}, nil
}
func (p sharedPlanProbe) YAMLValidatorEncode() (*yaml.Node, error) {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"}, {Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Name}}}, nil
}
func (p *sharedPlanProbe) YAMLValidatorDecode(n *yaml.Node) error {
	p.Name = n.Content[1].Value
	return nil
}
func (p sharedPlanProbe) MarshalYAML() (any, error)         { return genruntime.MarshalYAML(p) }
func (p *sharedPlanProbe) UnmarshalYAML(n *yaml.Node) error { return genruntime.UnmarshalYAML(n, p) }

func TestPublicAndStandardHooksShareGeneratedPlan(t *testing.T) {
	sharedPlanCompiles.Store(0)
	var value sharedPlanProbe
	for range 3 {
		if err := yaml.Unmarshal([]byte("name: standard\n"), &value); err != nil {
			t.Fatal(err)
		}
		if err := v.Unmarshal([]byte("name: own\n"), &value); err != nil {
			t.Fatal(err)
		}
	}
	if count := sharedPlanCompiles.Load(); count != 1 {
		t.Fatalf("decode plans built %d times; want one shared plan", count)
	}
	for range 3 {
		if _, err := v.Marshal(value); err != nil {
			t.Fatal(err)
		}
		if _, err := yaml.Marshal(value); err != nil {
			t.Fatal(err)
		}
	}
	if count := sharedPlanCompiles.Load(); count != 2 {
		t.Fatalf("plans built %d times; want one per direction", count)
	}
	var diag *v.ValidationErrors
	if err := yaml.Unmarshal([]byte("{}"), &value); !errors.As(err, &diag) {
		t.Fatalf("standard hook error not exposed as root ValidationErrors: %v", err)
	}
	if value.Name != "own" {
		t.Fatal("validation failure changed destination")
	}
}
