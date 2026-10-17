package main

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
	genspec "github.com/Yakwilik/go-yamlvalidator/internal/genspec"
	"github.com/Yakwilik/go-yamlvalidator/internal/schemacompiler"
	"github.com/Yakwilik/go-yamlvalidator/internal/taglang"
)

type schemaField struct {
	field field
	tag   string
}

func (g *generator) schemaGraphSource() (string, error) {
	var b strings.Builder
	graphs := [2]genspec.SourceGraph{}
	for mode := range graphs {
		graphs[mode].Nodes = make([]genspec.SourceNode, len(g.types))
		for id, typ := range g.types {
			spec, err := g.schemaNodeSpec(id, typ, mode == 1)
			if err != nil {
				return "", err
			}
			graphs[mode].Nodes[id] = spec
		}
	}
	b.WriteString("var yamlvalidatorGeneratedTypes = []reflect.Type{")
	for _, typ := range g.types {
		fmt.Fprintf(&b, "reflect.TypeFor[%s](),", g.typeText(typ))
	}
	b.WriteString("}\n")
	for _, root := range g.roots {
		var lowered [2]genspec.Graph
		var errors [2]error
		for mode := range graphs {
			lowered[mode], errors[mode] = schemacompiler.Lower(graphs[mode], root.id)
			if errors[mode] != nil {
				lowered[mode] = genspec.Graph{Error: errors[mode].Error()}
			}
		}
		if errors[0] != nil && errors[1] != nil {
			return "", fmt.Errorf("type %s: lower schema: %w", root.name, errors[0])
		}
		fmt.Fprintf(&b, "func yamlvalidatorGeneratedSchema%d() (spec.Graph,[]reflect.Type) { return yamlvalidatorGeneratedSchema%dWithNaming(genruntime.FieldNamingYAML) }\n", root.id, root.id)
		fmt.Fprintf(&b, "func yamlvalidatorGeneratedSchema%dWithNaming(naming genruntime.FieldNaming) (spec.Graph,[]reflect.Type) { if naming==genruntime.FieldNamingGo {return %s,yamlvalidatorGeneratedTypes};return %s,yamlvalidatorGeneratedTypes }\n", root.id, renderGeneratedLiteral(reflect.ValueOf(lowered[1])), renderGeneratedLiteral(reflect.ValueOf(lowered[0])))
		fmt.Fprintf(&b, "func yamlvalidatorGeneratedNaming%d(ctx *genruntime.FieldNamingContext) error {", root.id)
		for mode, graph := range graphs {
			if err := sourceFieldNamingError(graph, root.id); err != nil {
				comparison := "!="
				if mode == 1 {
					comparison = "=="
				}
				fmt.Fprintf(&b, "if ctx.Naming%sgenruntime.FieldNamingGo{return fmt.Errorf(\"%%s\",%q)};", comparison, err.Error())
			}
		}
		if opaque := sourceOpaqueNodes(graphs[0], root.id); len(opaque) > 0 {
			for _, id := range opaque {
				fmt.Fprintf(&b, "if err:=ctx.Validate(yamlvalidatorGeneratedTypes[%d]);err!=nil{return err};", id)
			}
		}
		b.WriteString("return nil}\n")
	}
	return b.String(), nil
}

// sourceOpaqueNodes identifies fallback boundaries that must retain the
// directional priority of user codecs during naming preflight.
func sourceOpaqueNodes(graph genspec.SourceGraph, root int) []int {
	var out []int
	visited := map[int]bool{}
	var visit func(int)
	visit = func(id int) {
		if id < 0 || visited[id] {
			return
		}
		visited[id] = true
		node := graph.Nodes[id]
		if node.Opaque {
			out = append(out, id)
			return
		}
		for _, child := range []int{node.AliasOf, node.Item, node.Value} {
			visit(child)
		}
		for _, field := range node.Fields {
			visit(field.Node)
		}
	}
	visit(root)
	return out
}

// sourceFieldNamingError checks only name metadata, so SkipValidation never
// evaluates validation rules or registry bindings while rejecting collisions.
func sourceFieldNamingError(graph genspec.SourceGraph, root int) error {
	visited := map[int]bool{}
	var keys func(int, map[int]bool) ([]string, int, error)
	keys = func(id int, active map[int]bool) ([]string, int, error) {
		if active[id] {
			return nil, 0, nil
		}
		active[id] = true
		defer delete(active, id)
		node := graph.Nodes[id]
		if node.AliasOf >= 0 {
			return keys(node.AliasOf, active)
		}
		if node.Fields == nil {
			if node.Value >= 0 {
				return nil, 1, nil
			}
			return nil, 0, nil
		}
		var out []string
		maps := 0
		for _, field := range node.Fields {
			if !field.Inline {
				out = append(out, field.Key)
				continue
			}
			child, count, err := keys(field.Node, active)
			if err != nil {
				return nil, 0, err
			}
			out = append(out, child...)
			maps += count
		}
		seen := map[string]bool{}
		for _, key := range out {
			if seen[key] {
				return nil, 0, fmt.Errorf("duplicate YAML key %q", key)
			}
			seen[key] = true
		}
		if maps > 1 {
			return nil, 0, fmt.Errorf("multiple inline maps are unsupported")
		}
		return out, maps, nil
	}
	var visit func(int) error
	visit = func(id int) error {
		if id < 0 || visited[id] {
			return nil
		}
		visited[id] = true
		node := graph.Nodes[id]
		if node.Fields != nil {
			if _, _, err := keys(id, map[int]bool{}); err != nil {
				return err
			}
		}
		for _, child := range []int{node.AliasOf, node.Item, node.Value} {
			if err := visit(child); err != nil {
				return err
			}
		}
		for _, field := range node.Fields {
			if err := visit(field.Node); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(root)
}

func (g *generator) schemaNodeSpec(id int, typ types.Type, goNaming bool) (genspec.SourceNode, error) {
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
			if goNaming && f.yamlName == "" {
				field.Key = f.name
			}
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
