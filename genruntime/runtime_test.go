package genruntime_test

import (
	"github.com/Yakwilik/go-yamlvalidator/genruntime"
	"github.com/Yakwilik/go-yamlvalidator/genruntime/spec"
	"gopkg.in/yaml.v3"
	"reflect"
	"testing"
	"time"
)

func parse(t *testing.T, s string) *yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(s), &n); err != nil {
		t.Fatal(err)
	}
	return n.Content[0]
}

func TestRuntimeAliasesMergesAndLimits(t *testing.T) {
	root := parse(t, "a: &a {x: first, y: inherited}\nb: &b {x: second, z: fallback}\nresult: {<<: [*a, *b], x: override}\n")
	node := root.Content[5]
	ctx := genruntime.NewDecodeContext(genruntime.Limits{}, false)
	if err := ctx.Enter(); err != nil {
		t.Fatal(err)
	}
	defer ctx.Leave()
	pairs, err := ctx.MappingPairs(node)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, p := range pairs {
		values[p.Key] = p.Value.Value
	}
	if !reflect.DeepEqual(values, map[string]string{"x": "override", "y": "inherited", "z": "fallback"}) {
		t.Fatalf("merge precedence: %+v", values)
	}
	if _, err := genruntime.MappingPairs(node); err != nil {
		t.Fatal(err)
	}
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: node}
	if got, err := genruntime.ResolveNode(alias); err != nil || got != node {
		t.Fatalf("resolve: %p %v", got, err)
	}
	if _, err := genruntime.NewDecodeContext(genruntime.Limits{MaxNodeVisits: 1}, true).MappingPairs(node); err == nil {
		t.Fatal("merge budget ignored")
	}
	if _, err := ctx.MappingPairs(parse(t, "{x: one, x: two}")); err == nil {
		t.Fatal("duplicate keys accepted")
	}
	if _, err := ctx.MappingPairs(parse(t, "{<<: 42}")); err == nil {
		t.Fatal("invalid merge accepted")
	}
	var empty *genruntime.DecodeContext
	if err := empty.Enter(); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := empty.MappingPairs(node); err == nil {
		t.Fatal("nil mapping context accepted")
	}
	parent := parse(t, "{a: one}")
	child := parse(t, "{b: two}")
	if err := genruntime.AppendInline(parent, child); err != nil {
		t.Fatal(err)
	}
	if err := genruntime.AppendInline(parent, child); err == nil {
		t.Fatal("inline collision accepted")
	}
}

func TestRuntimeFallbackAndMixedFacade(t *testing.T) {
	stamp := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	node, err := genruntime.FallbackEncode(stamp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded time.Time
	if err := genruntime.FallbackDecode(node, &decoded); err != nil || !decoded.Equal(stamp) {
		t.Fatalf("fallback: %v %v", decoded, err)
	}
	if err := genruntime.FallbackDecodeWithContext(node, &decoded, genruntime.NewDecodeContext(genruntime.Limits{}, true)); err != nil {
		t.Fatal(err)
	}
	scalar, err := genruntime.ScalarEncode("hello")
	if err != nil {
		t.Fatal(err)
	}
	var text string
	if err := genruntime.ScalarDecode(scalar, &text); err != nil || text != "hello" {
		t.Fatalf("scalar: %q %v", text, err)
	}
	var value struct{ Name string }
	data := parse(t, "name: mixed")
	if err := genruntime.DecodeMixed(data, reflect.ValueOf(&value).Elem()); err != nil || value.Name != "mixed" {
		t.Fatalf("mixed: %+v %v", value, err)
	}
	if err := genruntime.DecodeMixedWithContext(data, reflect.ValueOf(&value).Elem(), genruntime.NewDecodeContext(genruntime.Limits{}, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := genruntime.EncodeMixed(reflect.ValueOf(value)); err != nil {
		t.Fatal(err)
	}
	if err := genruntime.DecodeMixed(data, reflect.Value{}); err == nil {
		t.Fatal("invalid destination accepted")
	}
	if genruntime.HasGenerated(reflect.TypeOf(value), map[reflect.Type]bool{}) {
		t.Fatal("ordinary struct marked generated")
	}
	if !genruntime.IsEmpty(uint16(0)) || !genruntime.IsEmpty(float32(0)) || genruntime.IsEmpty(uint64(1)) || genruntime.IsEmpty(1.0) {
		t.Fatal("zero semantics")
	}
	if !genruntime.IsEmpty(nil) || genruntime.IsEmpty(struct{ X string }{"value"}) {
		t.Fatal("empty semantics")
	}
	if *spec.Int(12) != 12 || spec.Number("1.25").String() != "1.25" {
		t.Fatal("schema literal helpers")
	}
}

func TestRuntimeCyclesShareLimits(t *testing.T) {
	type link struct{ Next *link }
	cyclic := &link{}
	cyclic.Next = cyclic
	if err := genruntime.RejectCycles(cyclic, genruntime.Limits{}); err == nil {
		t.Fatal("pointer cycle accepted")
	}
	if err := genruntime.RejectCycles(&link{}, genruntime.Limits{MaxNodeVisits: 1}); err == nil {
		t.Fatal("visit limit ignored")
	}
	ctx := genruntime.NewCycleContext(genruntime.Limits{MaxDepth: 1, MaxNodeVisits: 10})
	done, err := ctx.Enter(cyclic, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Enter(cyclic, 1); err == nil {
		t.Fatal("active pointer revisited")
	}
	done()
	if _, err := ctx.Enter(cyclic, 2); err == nil {
		t.Fatal("depth limit ignored")
	}
	if err := ctx.CheckDynamic(cyclic, 0); err == nil {
		t.Fatal("dynamic cycle accepted")
	}
	if err := ctx.CheckDynamic(nil, 2); err == nil {
		t.Fatal("dynamic depth limit ignored")
	}
}
