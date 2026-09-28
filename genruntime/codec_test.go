package genruntime

import (
	"math"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestGeneratedScalarPaths(t *testing.T) {
	encodes := []struct {
		value     any
		tag, text string
	}{
		{"x", "!!str", "x"}, {true, "!!bool", "true"}, {int(-4), "!!int", "-4"}, {int8(-8), "!!int", "-8"}, {int16(-16), "!!int", "-16"}, {int32(-32), "!!int", "-32"}, {int64(-64), "!!int", "-64"},
		{uint(4), "!!int", "4"}, {uint8(8), "!!int", "8"}, {uint16(16), "!!int", "16"}, {uint32(32), "!!int", "32"}, {uint64(64), "!!int", "64"},
		{float32(1.5), "!!float", "1.5"}, {float64(2.5), "!!float", "2.5"}, {math.Inf(1), "!!float", ".inf"}, {math.Inf(-1), "!!float", "-.inf"},
	}
	for _, tc := range encodes {
		node, err := ScalarEncode(tc.value)
		if err != nil || node.Tag != tc.tag || node.Value != tc.text {
			t.Fatalf("%T: %+v %v", tc.value, node, err)
		}
	}
	nan, err := ScalarEncode(math.NaN())
	if err != nil || nan.Value != ".nan" {
		t.Fatalf("nan %+v %v", nan, err)
	}
	decode := func(node *yaml.Node, dst any) error {
		switch p := dst.(type) {
		case *string:
			return ScalarDecode(node, p)
		case *bool:
			return ScalarDecode(node, p)
		case *int:
			return ScalarDecode(node, p)
		case *int8:
			return ScalarDecode(node, p)
		case *int16:
			return ScalarDecode(node, p)
		case *int32:
			return ScalarDecode(node, p)
		case *int64:
			return ScalarDecode(node, p)
		case *uint:
			return ScalarDecode(node, p)
		case *uint8:
			return ScalarDecode(node, p)
		case *uint16:
			return ScalarDecode(node, p)
		case *uint32:
			return ScalarDecode(node, p)
		case *uint64:
			return ScalarDecode(node, p)
		case *float32:
			return ScalarDecode(node, p)
		case *float64:
			return ScalarDecode(node, p)
		}
		return nil
	}
	var s string
	var b bool
	var i int
	var i8 int8
	var i16 int16
	var i32 int32
	var i64 int64
	var u uint
	var u8 uint8
	var u16 uint16
	var u32 uint32
	var u64 uint64
	var f32 float32
	var f64 float64
	for _, tc := range []struct {
		node yaml.Node
		dst  any
	}{
		{yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "abc"}, &s}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}, &b},
		{yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &i}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &i8}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &i16}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &i32}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &i64},
		{yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &u}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &u8}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &u16}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &u32}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "12"}, &u64},
		{yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: ".inf"}, &f32}, {yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: "-.inf"}, &f64},
	} {
		if err := decode(&tc.node, tc.dst); err != nil {
			t.Fatal(err)
		}
	}
	if s != "abc" || !b || i != 12 || i8 != 12 || i16 != 12 || i32 != 12 || i64 != 12 || u != 12 || u8 != 12 || u16 != 12 || u32 != 12 || u64 != 12 || !math.IsInf(float64(f32), 1) || !math.IsInf(f64, -1) {
		t.Fatal("primitive decode mismatch")
	}
	if f, err := parseFloat(".nan", 64); err != nil || !math.IsNaN(f) {
		t.Fatalf("nan parse %v %v", f, err)
	}
	if f, err := parseFloat("1.25", 64); err != nil || f != 1.25 {
		t.Fatalf("float parse %v %v", f, err)
	}
	for _, bad := range []string{"nope", "TrUe"} {
		var value bool
		if err := ScalarDecode(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: bad}, &value); err == nil {
			t.Fatalf("malformed bool %q accepted", bad)
		}
	}
	var timestamp time.Time
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: "2020-01-02T03:04:05Z"}
	if err := FallbackDecode(node, &timestamp); err != nil || timestamp.Year() != 2020 {
		t.Fatalf("time %v %v", timestamp, err)
	}
	if _, err := FallbackEncode(timestamp); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedNodeSafetyAndOmission(t *testing.T) {
	a := &yaml.Node{Kind: yaml.AliasNode}
	a.Alias = a
	if _, err := ResolveNode(a); err == nil {
		t.Fatal("alias cycle accepted")
	}
	if _, err := MappingPairs(a); err == nil {
		t.Fatal("mapping alias cycle accepted")
	}
	if _, err := ResolveNode(nil); err == nil {
		t.Fatal("nil node accepted")
	}
	if _, err := MappingPairs(&yaml.Node{Kind: yaml.ScalarNode}); err == nil {
		t.Fatal("scalar accepted as map")
	}
	parent := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Value: "a"}, {Value: "one"}}}
	child := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Value: "b"}, {Value: "two"}}}
	if err := AppendInline(parent, child); err != nil || len(parent.Content) != 4 {
		t.Fatalf("append %v", err)
	}
	if err := AppendInline(parent, child); err == nil {
		t.Fatal("inline collision accepted")
	}
	if err := AppendInline(parent, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}); err == nil {
		t.Fatal("inline scalar accepted")
	}
	if err := AppendInline(parent, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}); err != nil {
		t.Fatal(err)
	}
	if !IsEmpty(struct{ A int }{}) || IsEmpty([1]int{}) || !IsEmpty([]string{}) || !IsEmpty(map[string]int{}) || !IsEmpty(false) || !IsEmpty(0) || !IsEmpty("") {
		t.Fatal("omitempty mismatch")
	}
	var ptr *struct{ A int }
	if !IsEmpty(ptr) {
		t.Fatal("nil pointer is nonempty")
	}
}
