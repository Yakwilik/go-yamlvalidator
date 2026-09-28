package schemacompiler

import (
	engine "github.com/Yakwilik/go-yamlvalidator/internal/engine"
	"github.com/Yakwilik/go-yamlvalidator/internal/genspec"
	"testing"
)

func TestLowerSourceGraph(t *testing.T) {
	src := genspec.SourceGraph{Nodes: []genspec.SourceNode{
		{Type: genspec.NodeType(engine.TypeMap), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1, Fields: []genspec.SourceField{{Key: "name", Node: 1, FieldName: "Name", Rules: []genspec.Rule{{Key: "required"}}}}},
		{Type: genspec.NodeType(engine.TypeString), AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1},
	}}
	graph, err := Lower(src, 0)
	if err != nil {
		t.Fatal(err)
	}
	name := graph.Nodes[graph.Nodes[graph.Root].AllowedKeys["name"]]
	if !name.Required || name.Type != genspec.NodeType(engine.TypeString) {
		t.Fatalf("not lowered: %+v", name)
	}
	if _, err := Lower(src, 9); err == nil {
		t.Fatal("invalid root accepted")
	}
}
