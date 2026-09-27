package yamlvalidator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// mixedHasGenerated only inspects types. Mixed trees use reflection because the
// enclosing type has no generated codec; generated children still run unchecked.
func mixedHasGenerated(typ reflect.Type, visiting map[reflect.Type]bool) bool {
	if typ == nil || visiting[typ] {
		return false
	}
	visiting[typ] = true
	defer delete(visiting, typ)
	if typ.Kind() == reflect.Pointer {
		return mixedHasGenerated(typ.Elem(), visiting)
	}
	if typ.Kind() != reflect.Interface {
		candidate := reflect.New(typ).Interface()
		_, promotedGenerated := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type })
		if marker, ok := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok && marker.YAMLValidatorGeneratedType() == typ {
			return true
		}
		if !promotedGenerated && (reflect.TypeOf(candidate).Implements(yamlUnmarshalerType) || reflect.TypeOf(candidate).Implements(yamlMarshalerType)) {
			return false
		}
	}
	switch typ.Kind() {
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).IsExported() && mixedHasGenerated(typ.Field(i).Type, visiting) {
				return true
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return mixedHasGenerated(typ.Elem(), visiting)
	}
	return false
}

func mixedYAMLKeys(typ reflect.Type, visiting map[reflect.Type]bool) []string {
	if typ == nil || visiting[typ] {
		return nil
	}
	visiting[typ] = true
	defer delete(visiting, typ)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name, inline, excluded, err := parseYAMLFieldTag(field, field.Tag.Get("yaml"))
		if err != nil || excluded {
			continue
		}
		if inline {
			out = append(out, mixedYAMLKeys(field.Type, visiting)...)
		} else {
			out = append(out, name)
		}
	}
	return out
}

func decodeMixedNode(node *yaml.Node, dst reflect.Value) error {
	return decodeMixedNodeContext(node, dst, NewGeneratedDecodeContext(Limits{}, false))
}

func decodeMixedNodeContext(node *yaml.Node, dst reflect.Value, ctx *GeneratedDecodeContext) error {
	if err := ctx.Enter(); err != nil {
		return err
	}
	defer ctx.Leave()
	if !dst.IsValid() || !dst.CanSet() {
		return fmt.Errorf("invalid generated decode destination")
	}
	if node == nil {
		return fmt.Errorf("nil YAML node")
	}
	if dst.Kind() == reflect.Pointer {
		if node.Tag == "!!null" {
			dst.SetZero()
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		if codec, ok := dst.Interface().(GeneratedCodec); ok && exactGeneratedType(dst.Interface(), codec.YAMLValidatorGeneratedType()) {
			if withContext, ok := dst.Interface().(interface {
				YAMLValidatorDecodeWithContext(*yaml.Node, *GeneratedDecodeContext) error
			}); ok {
				return withContext.YAMLValidatorDecodeWithContext(node, ctx)
			}
			return codec.YAMLValidatorDecode(node)
		}
		if dst.Type().Implements(yamlUnmarshalerType) || dst.Type().Implements(textUnmarshalerType) {
			return node.Decode(dst.Interface())
		}
		return decodeMixedNodeContext(node, dst.Elem(), ctx)
	}
	if dst.CanAddr() {
		if codec, ok := dst.Addr().Interface().(GeneratedCodec); ok && exactGeneratedType(dst.Addr().Interface(), codec.YAMLValidatorGeneratedType()) {
			if withContext, ok := dst.Addr().Interface().(interface {
				YAMLValidatorDecodeWithContext(*yaml.Node, *GeneratedDecodeContext) error
			}); ok {
				return withContext.YAMLValidatorDecodeWithContext(node, ctx)
			}
			return codec.YAMLValidatorDecode(node)
		}
	}
	if !mixedHasGenerated(dst.Type(), map[reflect.Type]bool{}) {
		return node.Decode(dst.Addr().Interface())
	}
	for node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return fmt.Errorf("invalid YAML alias")
		}
		node = node.Alias
	}
	switch dst.Kind() {
	case reflect.Struct:
		pairs, err := ctx.MappingPairs(node)
		if err != nil {
			return err
		}
		fields := map[string]int{}
		inline := []int{}
		known := map[string]bool{}
		for i := 0; i < dst.NumField(); i++ {
			f := dst.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			name, isInline, excluded, err := parseYAMLFieldTag(f, f.Tag.Get("yaml"))
			if err != nil {
				return err
			}
			if excluded {
				continue
			}
			if isInline {
				inline = append(inline, i)
				for _, key := range mixedYAMLKeys(f.Type, map[reflect.Type]bool{}) {
					known[key] = true
				}
			} else {
				fields[name] = i
				known[name] = true
			}
		}
		for _, pair := range pairs {
			if i, ok := fields[pair.Key]; ok {
				if err := decodeMixedNodeContext(pair.Value, dst.Field(i), ctx); err != nil {
					return fmt.Errorf("field %s: %w", dst.Type().Field(i).Name, err)
				}
			}
		}
		for _, i := range inline {
			f := dst.Field(i)
			mapNode := node
			base := f.Type()
			for base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if base.Kind() == reflect.Map {
				mapNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				for _, pair := range pairs {
					if known[pair.Key] {
						continue
					}
					mapNode.Content = append(mapNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pair.Key}, pair.Value)
				}
			}
			if err := decodeMixedNodeContext(mapNode, f, ctx); err != nil {
				return fmt.Errorf("inline field %s: %w", dst.Type().Field(i).Name, err)
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		if node.Tag == "!!null" && dst.Kind() == reflect.Slice {
			dst.SetZero()
			return nil
		}
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("expected YAML sequence")
		}
		if dst.Kind() == reflect.Array && dst.Len() != len(node.Content) {
			return fmt.Errorf("expected %d sequence items", dst.Len())
		}
		if dst.Kind() == reflect.Slice {
			dst.Set(reflect.MakeSlice(dst.Type(), len(node.Content), len(node.Content)))
		}
		for i, child := range node.Content {
			if err := decodeMixedNodeContext(child, dst.Index(i), ctx); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
		}
		return nil
	case reflect.Map:
		if node.Tag == "!!null" {
			dst.SetZero()
			return nil
		}
		pairs, err := ctx.MappingPairs(node)
		if err != nil {
			return err
		}
		if dst.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map key must be string")
		}
		result := dst
		if result.IsNil() {
			result.Set(reflect.MakeMapWithSize(dst.Type(), len(pairs)))
		}
		for _, pair := range pairs {
			item := reflect.New(dst.Type().Elem()).Elem()
			if err := decodeMixedNodeContext(pair.Value, item, ctx); err != nil {
				return fmt.Errorf("key %q: %w", pair.Key, err)
			}
			key := reflect.New(dst.Type().Key()).Elem()
			key.SetString(pair.Key)
			result.SetMapIndex(key, item)
		}
		return nil
	}
	return node.Decode(dst.Addr().Interface())
}

func encodeMixedNode(value reflect.Value) (*yaml.Node, error) {
	if !value.IsValid() {
		return GeneratedFallbackEncode[any](nil)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return GeneratedFallbackEncode[any](nil)
		}
		return encodeMixedNode(value.Elem())
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return GeneratedFallbackEncode[any](nil)
		}
		if codec, ok := value.Interface().(generatedEncoder); ok && exactGeneratedType(value.Interface(), codec.YAMLValidatorGeneratedType()) {
			return codec.YAMLValidatorEncode()
		}
		if value.Type().Implements(yamlMarshalerType) || value.Type().Implements(textMarshalerType) {
			var node yaml.Node
			if err := node.Encode(value.Interface()); err != nil {
				return nil, err
			}
			return &node, nil
		}
		return encodeMixedNode(value.Elem())
	}
	if value.CanInterface() {
		if codec, ok := value.Interface().(generatedEncoder); ok && exactGeneratedType(value.Interface(), codec.YAMLValidatorGeneratedType()) {
			return codec.YAMLValidatorEncode()
		}
	}
	if !mixedHasGenerated(value.Type(), map[reflect.Type]bool{}) {
		var node yaml.Node
		if err := node.Encode(value.Interface()); err != nil {
			return nil, err
		}
		return &node, nil
	}
	switch value.Kind() {
	case reflect.Struct:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i := 0; i < value.NumField(); i++ {
			f := value.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			name, inline, excluded, err := parseYAMLFieldTag(f, f.Tag.Get("yaml"))
			if err != nil {
				return nil, err
			}
			if excluded {
				continue
			}
			tag := f.Tag.Get("yaml")
			if strings.Contains(","+tag+",", ",omitempty,") && GeneratedIsEmpty(value.Field(i).Interface()) {
				continue
			}
			child, err := encodeMixedNode(value.Field(i))
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", f.Name, err)
			}
			if strings.Contains(","+tag+",", ",flow,") {
				child.Style |= yaml.FlowStyle
			}
			if inline {
				if err := GeneratedAppendInline(node, child); err != nil {
					return nil, err
				}
				continue
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, child)
		}
		return node, nil
	case reflect.Slice, reflect.Array:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for i := 0; i < value.Len(); i++ {
			child, err := encodeMixedNode(value.Index(i))
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key must be string")
		}
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, key := range keys {
			child, err := encodeMixedNode(value.MapIndex(key))
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.String()}, child)
		}
		return node, nil
	}
	var node yaml.Node
	if err := node.Encode(value.Interface()); err != nil {
		return nil, err
	}
	return &node, nil
}
