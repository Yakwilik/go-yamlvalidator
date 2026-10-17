package yamlcodec_test

import (
	"reflect"
	"testing"

	yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/internal/yamlcodec"
	"gopkg.in/yaml.v3"
)

func TestGoNamingRejectsMalformedMappingKeysAndMergeSources(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, source string }{
		{"duplicate in direct merge", "<<: {Name: first, Name: second}\n"},
		{"duplicate in aliased merge", "Base: &base {Name: first, Name: second}\n<<: *base\n"},
		{"duplicate in overridden merge", "<<: {Name: first, Name: second}\nName: explicit\n"},
		{"duplicate in merge sequence", "<<: [{Name: first, Name: second}, {Name: fallback}]\n"},
		{"sequence key", "? [complex, key]\n: value\n"},
		{"mapping key", "? {complex: key}\n: value\n"},
		{"merged sequence key", "<<: {? [complex, key]: value}\n"},
		{"alias sequence key", "Base: &key [complex, key]\n? *key\n: value\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var reference map[string]any
			if err := yaml.Unmarshal([]byte(test.source), &reference); err == nil {
				t.Fatal("fixture must also be rejected by yaml.v3")
			}
			var actual struct{ Name string }
			err := (yamlvalidator.UnmarshalOptions{FieldNaming: yamlvalidator.FieldNamingGo, SkipValidation: true}).Unmarshal([]byte(test.source), &actual)
			if err == nil {
				t.Fatalf("Go naming accepted malformed mapping: %s", test.source)
			}
		})
	}
}

func TestMappingPairsUsesYAMLScalarStringKeys(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"!!binary TmFtZQ==: binary\n",
		"&name Name: anchored\nOther: ignored\n",
		"? &key Name\n: first\n<<: {? *key: merged}\n",
		"42: integer\ntrue: boolean\n1.5: float\n",
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			var expected map[string]string
			if err := yaml.Unmarshal([]byte(source), &expected); err != nil {
				t.Fatal(err)
			}
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(source), &document); err != nil {
				t.Fatal(err)
			}
			ctx := yamlcodec.NewDecodeContext(yamlcodec.Limits{}, false)
			ctx.FieldNaming = yamlcodec.FieldNamingGo
			pairs, err := ctx.MappingPairs(document.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, pair := range pairs {
				var value string
				if err := pair.Value.Decode(&value); err != nil {
					t.Fatal(err)
				}
				got[pair.Key] = value
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("got %v, want %v", got, expected)
			}
		})
	}
}

func TestMappingPairsPreservesMergePrecedenceAndLimits(t *testing.T) {
	t.Parallel()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("<<: [{Name: first, Other: inherited}, {Name: second, Last: fallback}]\nName: explicit\n"), &document); err != nil {
		t.Fatal(err)
	}
	ctx := yamlcodec.NewDecodeContext(yamlcodec.Limits{}, false)
	ctx.FieldNaming = yamlcodec.FieldNamingGo
	pairs, err := ctx.MappingPairs(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, pair := range pairs {
		got[pair.Key] = pair.Value.Value
	}
	if !reflect.DeepEqual(got, map[string]string{"Name": "explicit", "Other": "inherited", "Last": "fallback"}) {
		t.Fatalf("merge precedence: %v", got)
	}
	limited := yamlcodec.NewDecodeContext(yamlcodec.Limits{MaxNodeVisits: 3}, true)
	if _, err := limited.MappingPairs(document.Content[0]); err == nil {
		t.Fatal("merge expansion ignored visit limit")
	}
	cyclic := &yaml.Node{Kind: yaml.MappingNode}
	cyclic.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"}, {Kind: yaml.AliasNode, Alias: cyclic}}
	if _, err := ctx.MappingPairs(cyclic); err == nil {
		t.Fatal("cyclic merge accepted")
	}
}

func TestDecodeContextTracksOnlyActiveMixedNodes(t *testing.T) {
	t.Parallel()
	ctx := yamlcodec.NewDecodeContext(yamlcodec.Limits{MaxDepth: 1}, true)
	node := &yaml.Node{Kind: yaml.MappingNode}
	leave, err := ctx.EnterMixedNode(node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.EnterMixedNode(node); err == nil {
		t.Fatal("active composite cycle accepted")
	}
	other := &yaml.Node{Kind: yaml.SequenceNode}
	leaveOther, err := ctx.EnterMixedNode(other)
	if err != nil {
		t.Fatalf("cycle tracking changed depth accounting: %v", err)
	}
	leaveOther()
	leave()
	leaveAgain, err := ctx.EnterMixedNode(node)
	if err != nil {
		t.Fatalf("reused alias incorrectly treated as cycle: %v", err)
	}
	leaveAgain()
}

func TestMappingPairsRejectsCyclicMergeValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value *yaml.Node
	}{
		{"alias chain", &yaml.Node{Kind: yaml.AliasNode}},
		{"merge sequence", &yaml.Node{Kind: yaml.SequenceNode}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.value.Kind == yaml.AliasNode {
				test.value.Alias = test.value
			} else {
				test.value.Content = []*yaml.Node{{Kind: yaml.AliasNode, Alias: test.value}}
			}
			root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!merge", Value: "<<"}, test.value}}
			ctx := yamlcodec.NewDecodeContext(yamlcodec.Limits{MaxNodeVisits: 10}, true)
			if _, err := ctx.MappingPairs(root); err == nil {
				t.Fatal("cyclic merge source accepted")
			}
			if _, err := yamlcodec.NewDecodeContext(yamlcodec.Limits{}, false).MappingPairs(root); err == nil {
				t.Fatal("cyclic merge source accepted without limit enforcement")
			}
		})
	}
}
