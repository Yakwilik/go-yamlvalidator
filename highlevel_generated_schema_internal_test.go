package yamlvalidator

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// Its deliberately invalid source tag makes a reflection schema compile fail.
type generatedSchemaProbe struct {
	Name string `yaml:"name" yamlvalidate:"not_a_rule"`
}

func (generatedSchemaProbe) YAMLValidatorGeneratedType() reflect.Type {
	return reflect.TypeFor[generatedSchemaProbe]()
}
func (generatedSchemaProbe) YAMLValidatorSchema(*Registry, bool) (*FieldSchema, error) {
	return &FieldSchema{Type: TypeMap, AllowedKeys: map[string]*FieldSchema{"name": {Type: TypeString, Required: true}}}, nil
}
func (value generatedSchemaProbe) YAMLValidatorEncode() (*yaml.Node, error) {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: value.Name},
	}}, nil
}
func (value *generatedSchemaProbe) YAMLValidatorDecode(node *yaml.Node) error {
	var plain struct {
		Name string `yaml:"name"`
	}
	if err := node.Decode(&plain); err != nil {
		return err
	}
	value.Name = plain.Name
	return nil
}

func TestGeneratedSchemaBypassesReflectCompiler(t *testing.T) {
	before := reflectSchemaCompileCalls.Load()
	var root generatedSchemaProbe
	if err := Unmarshal([]byte("name: valid\n"), &root); err != nil {
		t.Fatal(err)
	}
	if _, err := Marshal(root); err != nil {
		t.Fatal(err)
	}
	if got := reflectSchemaCompileCalls.Load(); got != before {
		t.Fatalf("generated root called reflection schema compiler %d times", got-before)
	}
	traversals := generatedStructReflectionCalls.Load()
	var parent struct {
		Child generatedSchemaProbe `yaml:"child"`
	}
	if err := Unmarshal([]byte("child: {name: valid}\n"), &parent); err != nil {
		t.Fatal(err)
	}
	if got := generatedStructReflectionCalls.Load(); got != traversals {
		t.Fatalf("generated nested type traversed reflect.StructField %d times", got-traversals)
	}
}
