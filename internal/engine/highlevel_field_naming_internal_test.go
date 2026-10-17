package yamlvalidator

import (
	"reflect"
	"testing"

	"github.com/Yakwilik/go-yamlvalidator/internal/genspec"
	"github.com/Yakwilik/go-yamlvalidator/internal/yamlcodec"
	"gopkg.in/yaml.v3"
)

// Invalid source flags make accidental reflection-based name selection fail.
// Generated metadata remains the authority for this probe.
type namingGeneratedProbe struct {
	Name    string `yaml:",invalid"`
	Decodes int    `yaml:"-"`
}

func (namingGeneratedProbe) YAMLValidatorGeneratedType() reflect.Type {
	return reflect.TypeFor[namingGeneratedProbe]()
}
func (p namingGeneratedProbe) YAMLValidatorSchemaSpec() (genspec.Graph, []reflect.Type) {
	return p.YAMLValidatorSchemaSpecWithNaming(FieldNamingYAML)
}
func (namingGeneratedProbe) YAMLValidatorSchemaSpecWithNaming(naming FieldNaming) (genspec.Graph, []reflect.Type) {
	key := yamlcodec.FieldName("Name", "", naming)
	return genspec.Graph{Nodes: []genspec.Node{{Type: genspec.NodeType(TypeMap), HasAllowedKeys: true, AllowedKeys: map[string]int{key: 1}}, {Type: genspec.NodeType(TypeString)}}}, nil
}
func (namingGeneratedProbe) YAMLValidatorValidateFieldNamingWithContext(*yamlcodec.FieldNamingContext) error {
	return nil
}
func (namingGeneratedProbe) YAMLValidatorFieldNamesWithNaming(naming FieldNaming) []string {
	return []string{yamlcodec.FieldName("Name", "", naming)}
}
func (p namingGeneratedProbe) YAMLValidatorEncode() (*yaml.Node, error) {
	return p.YAMLValidatorEncodeWithNaming(FieldNamingYAML)
}
func (p namingGeneratedProbe) YAMLValidatorEncodeWithNaming(naming FieldNaming) (*yaml.Node, error) {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: yamlcodec.FieldName("Name", "", naming)}, {Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Name}}}, nil
}
func (p *namingGeneratedProbe) YAMLValidatorDecode(node *yaml.Node) error {
	return p.YAMLValidatorDecodeWithContext(node, yamlcodec.NewDecodeContext(yamlcodec.Limits{}, false))
}
func (p *namingGeneratedProbe) YAMLValidatorDecodeWithContext(node *yaml.Node, ctx *yamlcodec.DecodeContext) error {
	pairs, err := ctx.MappingPairs(node)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if pair.Key == yamlcodec.FieldName("Name", "", ctx.FieldNaming) {
			if err := pair.Value.Decode(&p.Name); err != nil {
				return err
			}
		}
	}
	p.Decodes++
	return nil
}

func TestNamingGeneratedMetadataBypassesReflection(t *testing.T) {
	for _, naming := range []FieldNaming{FieldNamingYAML, FieldNamingGo} {
		key := yamlcodec.FieldName("Name", "", naming)
		var root namingGeneratedProbe
		before := reflectSchemaCompileCalls.Load()
		if err := (UnmarshalOptions{FieldNaming: naming}).Unmarshal([]byte(key+": root\n"), &root); err != nil {
			t.Fatal(err)
		}
		if root.Name != "root" || root.Decodes != 1 {
			t.Fatalf("typed root decoder bypassed: %#v", root)
		}
		if _, err := (MarshalOptions{FieldNaming: naming}).Marshal(root); err != nil {
			t.Fatal(err)
		}
		if reflectSchemaCompileCalls.Load() != before {
			t.Fatal("generated root compiled schema with reflection")
		}
		for _, skip := range []bool{false, true} {
			var parent struct {
				Child namingGeneratedProbe `yaml:",inline"`
			}
			if err := (UnmarshalOptions{FieldNaming: naming, SkipValidation: skip}).Unmarshal([]byte(key+": child\n"), &parent); err != nil {
				t.Fatal(err)
			}
			if parent.Child.Name != "child" || parent.Child.Decodes != 1 {
				t.Fatalf("inline typed decoder bypassed: %#v", parent)
			}
			if _, err := (MarshalOptions{FieldNaming: naming, SkipValidation: skip}).Marshal(parent); err != nil {
				t.Fatal(err)
			}
		}
	}
}
