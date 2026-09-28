package main

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
	genspec "github.com/Yakwilik/go-yamlvalidator/genruntime/spec"
	"github.com/Yakwilik/go-yamlvalidator/internal/taglang"
)

type schemaField struct {
	field field
	tag   string
}

func (g *generator) schemaGraphSource() (string, error) {
	var b strings.Builder
	graph := genspec.SourceGraph{Nodes: make([]genspec.SourceNode, len(g.types))}
	for id, typ := range g.types {
		spec, err := g.schemaNodeSpec(id, typ)
		if err != nil {
			return "", err
		}
		graph.Nodes[id] = spec
	}
	b.WriteString("var yamlvalidatorGeneratedTypes = []reflect.Type{")
	for _, typ := range g.types {
		fmt.Fprintf(&b, "reflect.TypeFor[%s](),", g.typeText(typ))
	}
	b.WriteString("}\n")
	for _, root := range g.roots {
		lowered, err := yamlvalidator.LowerGeneratedSchema(graph, root.id)
		if err != nil {
			return "", fmt.Errorf("type %s: lower schema: %w", root.name, err)
		}
		fmt.Fprintf(&b, "func yamlvalidatorGeneratedSchema%d(registry *yamlvalidator.Registry, encode bool) (*yamlvalidator.FieldSchema,error) { return yamlvalidator.BuildGeneratedSchema(%s,yamlvalidatorGeneratedTypes,registry,encode) }\n", root.id, renderGeneratedLiteral(reflect.ValueOf(lowered)))
	}
	return b.String(), nil
}

func (g *generator) schemaNodeSpec(id int, typ types.Type) (genspec.SourceNode, error) {
	s := genspec.SourceNode{AliasOf: -1, Item: -1, Value: -1, ArrayLen: -1}
	base := types.Unalias(typ)
	if named, ok := base.(*types.Named); ok {
		if named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && (named.Obj().Name() == "Time" || named.Obj().Name() == "Duration") {
			s.Type = genspec.NodeType(yamlvalidator.TypeString)
			s.Opaque = true
			return s, nil
		}
		if named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "gopkg.in/yaml.v3" && named.Obj().Name() == "Node" {
			s.Type = genspec.NodeType(yamlvalidator.TypeAny)
			s.Nullable = true
			s.Opaque = true
			return s, nil
		}
		if customCodec(typ) && !g.rootType(typ) {
			s.Type = genspec.NodeType(yamlvalidator.TypeAny)
			s.Opaque = true
			return s, nil
		}
		base = named.Underlying()
	}
	switch u := base.(type) {
	case *types.Pointer:
		s.Type = genspec.NodeType(yamlvalidator.TypeAny)
		s.Nullable = true
		s.AliasOf = g.schemaID(u.Elem())
	case *types.Basic:
		switch u.Kind() {
		case types.String:
			s.Type = genspec.NodeType(yamlvalidator.TypeString)
		case types.Bool:
			s.Type = genspec.NodeType(yamlvalidator.TypeBool)
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			s.Type = genspec.NodeType(yamlvalidator.TypeInt)
			s.NumberKind = basicReflectKindValue(u.Kind())
			s.NumberBits = basicBitsValue(u.Kind())
		case types.Float32, types.Float64:
			s.Type = genspec.NodeType(yamlvalidator.TypeFloat)
			s.NumberKind = basicReflectKindValue(u.Kind())
			s.NumberBits = basicBitsValue(u.Kind())
		default:
			s.Type = genspec.NodeType(yamlvalidator.TypeAny)
			s.Opaque = true
		}
	case *types.Struct:
		s.Type = genspec.NodeType(yamlvalidator.TypeMap)
		s.Fields = []genspec.SourceField{}
		for _, entry := range g.schemaFields[id] {
			f := entry.field
			field := genspec.SourceField{Key: f.key, Node: f.id, Inline: f.inline, FieldName: f.name}
			if entry.tag != "" {
				rules, err := taglang.Parse(entry.tag)
				if err != nil {
					return s, fmt.Errorf("field %s: %w", f.name, err)
				}
				field.Rules = convertGeneratedRules(rules)
			}
			s.Fields = append(s.Fields, field)
		}
	case *types.Map:
		s.Type = genspec.NodeType(yamlvalidator.TypeMap)
		s.Nullable = true
		s.Value = g.schemaID(u.Elem())
	case *types.Slice:
		s.Type = genspec.NodeType(yamlvalidator.TypeSequence)
		s.Nullable = true
		if basic, ok := types.Unalias(u.Elem()).(*types.Basic); ok && basic.Kind() == types.Byte {
			s.ByteSlice = true
		} else {
			s.Item = g.schemaID(u.Elem())
		}
	case *types.Array:
		s.Type = genspec.NodeType(yamlvalidator.TypeSequence)
		s.Item = g.schemaID(u.Elem())
		s.ArrayLen = int(u.Len())
	case *types.Interface:
		s.Type = genspec.NodeType(yamlvalidator.TypeAny)
		s.Nullable = true
	default:
		s.Type = genspec.NodeType(yamlvalidator.TypeAny)
		s.Opaque = true
	}
	return s, nil
}

func convertGeneratedRules(rules []taglang.Rule) []genspec.Rule {
	out := make([]genspec.Rule, len(rules))
	for i, r := range rules {
		out[i] = genspec.Rule{Key: r.Key, HasValue: r.HasValue, Offset: r.Offset, Value: convertGeneratedValue(r.Value)}
	}
	return out
}

func convertGeneratedValue(value taglang.Value) genspec.RuleValue {
	out := genspec.RuleValue{Kind: byte(value.Kind), Text: value.Text, Rules: convertGeneratedRules(value.Rules)}
	for _, item := range value.List {
		out.List = append(out.List, convertGeneratedValue(item))
	}
	return out
}

func basicReflectKindValue(kind types.BasicKind) reflect.Kind {
	switch kind {
	case types.Int:
		return reflect.Int
	case types.Int8:
		return reflect.Int8
	case types.Int16:
		return reflect.Int16
	case types.Int32:
		return reflect.Int32
	case types.Int64:
		return reflect.Int64
	case types.Uint:
		return reflect.Uint
	case types.Uint8:
		return reflect.Uint8
	case types.Uint16:
		return reflect.Uint16
	case types.Uint32:
		return reflect.Uint32
	case types.Uint64:
		return reflect.Uint64
	case types.Float32:
		return reflect.Float32
	case types.Float64:
		return reflect.Float64
	default:
		return reflect.Invalid
	}
}

func basicBitsValue(kind types.BasicKind) int {
	switch kind {
	case types.Int, types.Uint:
		return strconv.IntSize
	case types.Int8, types.Uint8:
		return 8
	case types.Int16, types.Uint16:
		return 16
	case types.Int32, types.Uint32, types.Float32:
		return 32
	default:
		return 64
	}
}

func (g *generator) schemaID(t types.Type) int {
	key := types.TypeString(t, func(p *types.Package) string { return p.Path() })
	return g.ids[key]
}
