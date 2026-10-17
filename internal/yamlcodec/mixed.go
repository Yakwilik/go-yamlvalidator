package yamlcodec

import (
	"encoding"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var yamlUnmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()
var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()

func HasGenerated(typ reflect.Type, visiting map[reflect.Type]bool) bool {
	if typ == nil || visiting[typ] {
		return false
	}
	visiting[typ] = true
	defer delete(visiting, typ)
	if typ.Kind() == reflect.Pointer {
		return HasGenerated(typ.Elem(), visiting)
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
			if typ.Field(i).IsExported() && HasGenerated(typ.Field(i).Type, visiting) {
				return true
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return HasGenerated(typ.Elem(), visiting)
	}
	return false
}

func DecodeMixed(node *yaml.Node, dst reflect.Value) error {
	return DecodeMixedWithContext(node, dst, NewDecodeContext(Limits{}, false))
}

func DecodeMixedWithContext(node *yaml.Node, dst reflect.Value, ctx *DecodeContext) error {
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
	if dst.Type() == nodeType {
		return node.Decode(dst.Addr().Interface())
	}
	resolved, err := ctx.Resolve(node)
	if err != nil {
		return err
	}
	if resolved.Tag == "!!null" {
		switch dst.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			dst.SetZero()
		}
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		if handled, err := decodeGenerated(node, dst.Interface(), ctx); handled {
			return err
		}
		if !hasGeneratedMarker(dst.Interface()) && IsCustomUnmarshaler(dst.Type()) {
			return node.Decode(dst.Interface())
		}
		return DecodeMixedWithContext(node, dst.Elem(), ctx)
	}
	if dst.CanAddr() {
		candidate := dst.Addr().Interface()
		if handled, err := decodeGenerated(node, candidate, ctx); handled {
			return err
		}
		if !hasGeneratedMarker(candidate) && IsCustomUnmarshaler(dst.Type()) {
			return node.Decode(candidate)
		}
	}
	if specialRepresentation(dst.Type()) || ctx.FieldNaming == FieldNamingYAML && !HasGenerated(dst.Type(), map[reflect.Type]bool{}) {
		return node.Decode(dst.Addr().Interface())
	}
	node = resolved
	switch dst.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		leave, err := ctx.EnterMixedNode(node)
		if err != nil {
			return err
		}
		defer leave()
	}
	switch dst.Kind() {
	case reflect.Struct:
		pairs, err := ctx.MappingPairs(node)
		if err != nil {
			return err
		}
		metadata, err := structFields(dst.Type(), ctx.FieldNaming, map[reflect.Type]bool{}, false)
		if err != nil {
			return err
		}
		fields := map[string][]int{}
		for _, field := range metadata.fields {
			fields[field.name] = field.index
		}
		for _, index := range metadata.inlineCodecs {
			field, err := fieldByIndex(dst, index, true)
			if err != nil {
				return err
			}
			if field.Kind() == reflect.Pointer && field.IsNil() {
				if !field.CanSet() {
					return fmt.Errorf("cannot set inline YAML pointer")
				}
				field.Set(reflect.New(field.Type().Elem()))
			}
			inlineNode := node
			candidate := interfaceValue(field)
			if field.Kind() != reflect.Pointer && field.CanAddr() {
				candidate = interfaceValue(field.Addr())
			}
			if generated, ok := candidate.(generatedFieldNames); ok && exactGeneratedType(candidate, generated.YAMLValidatorGeneratedType()) {
				own := map[string]bool{}
				for _, key := range generated.YAMLValidatorFieldNamesWithNaming(ctx.FieldNaming) {
					own[key] = true
				}
				// Exclude sibling fields before an inline generated map captures extras.
				// Keep every original node untouched, including key positions and styles.
				inlineNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Line: node.Line, Column: node.Column}
				for _, pair := range pairs {
					if _, known := fields[pair.Key]; known && !own[pair.Key] {
						continue
					}
					inlineNode.Content = append(inlineNode.Content, pair.KeyNode, pair.Value)
				}
			}
			if err := DecodeMixedWithContext(inlineNode, field, ctx); err != nil {
				return err
			}
		}
		var extra []*yaml.Node
		for _, pair := range pairs {
			if index, ok := fields[pair.Key]; ok {
				if len(index) == 0 {
					continue
				}
				field, err := fieldByIndex(dst, index, true)
				if err != nil {
					return err
				}
				if err := DecodeMixedWithContext(pair.Value, field, ctx); err != nil {
					return fmt.Errorf("field %s: %w", pair.Key, err)
				}
				continue
			}
			if len(metadata.inlineMap) > 0 {
				extra = append(extra, pair.KeyNode, pair.Value)
			}
		}
		if len(extra) > 0 {
			field, err := fieldByIndex(dst, metadata.inlineMap, true)
			if err != nil {
				return err
			}
			return DecodeMixedWithContext(&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: extra}, field, ctx)
		}
		return nil
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("expected YAML sequence")
		}
		if dst.Kind() == reflect.Array && dst.Len() != len(node.Content) {
			return fmt.Errorf("expected %d sequence items", dst.Len())
		}
		if dst.Kind() == reflect.Slice {
			dst.Set(reflect.MakeSlice(dst.Type(), len(node.Content), len(node.Content)))
		}
		count := 0
		for i, child := range node.Content {
			resolved, err := ctx.Resolve(child)
			if err != nil {
				return err
			}
			if resolved.Tag == "!!null" && !keepsNull(dst.Type().Elem()) {
				err := ctx.Enter()
				ctx.Leave()
				if err != nil {
					return err
				}
				continue
			}
			if err := DecodeMixedWithContext(child, dst.Index(count), ctx); err != nil {
				return fmt.Errorf("item %d: %w", i, err)
			}
			count++
		}
		if dst.Kind() == reflect.Slice {
			dst.SetLen(count)
		}
		return nil
	case reflect.Map:
		var pairs []Pair
		var err error
		if IsCustomUnmarshaler(dst.Type().Key()) {
			pairs, err = ctx.MappingPairsWithKeyDecoder(node, func(keyNode *yaml.Node) (string, error) {
				key := reflect.New(dst.Type().Key()).Elem()
				if err := keyNode.Decode(key.Addr().Interface()); err != nil {
					return "", err
				}
				return key.String(), nil
			})
		} else {
			pairs, err = ctx.MappingPairs(node)
		}
		if err != nil {
			return err
		}
		if dst.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map key must be string")
		}
		newMap := dst.IsNil()
		if newMap {
			dst.Set(reflect.MakeMapWithSize(dst.Type(), len(pairs)))
		}
		for _, pair := range pairs {
			resolved, err := ctx.Resolve(pair.Value)
			if err != nil {
				return err
			}
			key := reflect.New(dst.Type().Key()).Elem()
			key.SetString(pair.Key)
			kind := dst.Type().Elem().Kind()
			if resolved.Tag == "!!null" && !newMap && dst.MapIndex(key).IsValid() && kind != reflect.Pointer && kind != reflect.Map && kind != reflect.Slice && kind != reflect.Interface {
				err := ctx.Enter()
				ctx.Leave()
				if err != nil {
					return err
				}
				continue
			}
			item := reflect.New(dst.Type().Elem()).Elem()
			if err := DecodeMixedWithContext(pair.Value, item, ctx); err != nil {
				return fmt.Errorf("key %q: %w", pair.Key, err)
			}
			dst.SetMapIndex(key, item)
		}
		return nil
	}
	return node.Decode(dst.Addr().Interface())
}

func hasGeneratedMarker(value any) bool {
	_, ok := value.(interface{ YAMLValidatorGeneratedType() reflect.Type })
	return ok
}

func decodeGenerated(node *yaml.Node, value any, ctx *DecodeContext) (bool, error) {
	marker, ok := value.(interface{ YAMLValidatorGeneratedType() reflect.Type })
	if !ok || !exactGeneratedType(value, marker.YAMLValidatorGeneratedType()) {
		return false, nil
	}
	if ctx.FieldNaming == FieldNamingGo {
		// Old generated decoders have a context but hard-code YAML names.
		if _, ok := value.(generatedNamingEncoder); !ok {
			return false, nil
		}
	}
	if decoder, ok := value.(generatedContextDecoder); ok {
		return true, decoder.YAMLValidatorDecodeWithContext(node, ctx)
	}
	if ctx.FieldNaming == FieldNamingYAML {
		if codec, ok := value.(generatedCodec); ok {
			return true, codec.YAMLValidatorDecode(node)
		}
	}
	return false, nil
}

func EncodeMixed(value reflect.Value) (*yaml.Node, error) {
	return EncodeMixedWithNaming(value, FieldNamingYAML)
}

func EncodeMixedWithNaming(value reflect.Value, naming FieldNaming) (*yaml.Node, error) {
	if !value.IsValid() {
		return FallbackEncode[any](nil)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return FallbackEncode[any](nil)
		}
		return EncodeMixedWithNaming(value.Elem(), naming)
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return FallbackEncode[any](nil)
		}
		if codec, ok := interfaceValue(value).(generatedEncoder); ok && exactGeneratedType(interfaceValue(value), codec.YAMLValidatorGeneratedType()) {
			if naming == FieldNamingYAML {
				return codec.YAMLValidatorEncode()
			}
			if named, ok := value.Interface().(generatedNamingEncoder); ok {
				return named.YAMLValidatorEncodeWithNaming(naming)
			}
		}
		if !hasGeneratedMarker(interfaceValue(value)) && (value.Type().Implements(yamlMarshalerType) || value.Type().Implements(textMarshalerType)) {
			var node yaml.Node
			if err := node.Encode(value.Interface()); err != nil {
				return nil, err
			}
			return &node, nil
		}
		return EncodeMixedWithNaming(value.Elem(), naming)
	}
	if value.CanInterface() {
		if codec, ok := interfaceValue(value).(generatedEncoder); ok && exactGeneratedType(interfaceValue(value), codec.YAMLValidatorGeneratedType()) {
			if naming == FieldNamingYAML {
				return codec.YAMLValidatorEncode()
			}
			if named, ok := value.Interface().(generatedNamingEncoder); ok {
				return named.YAMLValidatorEncodeWithNaming(naming)
			}
		}
	}
	if value.CanInterface() && (specialRepresentation(value.Type()) || naming == FieldNamingYAML && !HasGenerated(value.Type(), map[reflect.Type]bool{}) || !hasGeneratedMarker(value.Interface()) && (value.Type().Implements(yamlMarshalerType) || value.Type().Implements(textMarshalerType))) {
		var node yaml.Node
		if err := node.Encode(value.Interface()); err != nil {
			return nil, err
		}
		return &node, nil
	}
	switch value.Kind() {
	case reflect.Struct:
		if _, err := structFields(value.Type(), naming, map[reflect.Type]bool{}, true); err != nil {
			return nil, err
		}
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i := 0; i < value.NumField(); i++ {
			f := value.Type().Field(i)
			if !includeField(f) {
				continue
			}
			name, inline, excluded, err := parseYAMLFieldTag(f, f.Tag.Get("yaml"), naming)
			if err != nil {
				return nil, err
			}
			if excluded {
				continue
			}
			tag := f.Tag.Get("yaml")
			if strings.Contains(","+tag+",", ",omitempty,") && isEmptyValue(value.Field(i)) {
				continue
			}
			child, err := EncodeMixedWithNaming(value.Field(i), naming)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", f.Name, err)
			}
			if strings.Contains(","+tag+",", ",flow,") {
				child.Style |= yaml.FlowStyle
			}
			if inline {
				if err := AppendInline(node, child); err != nil {
					return nil, err
				}
				continue
			}
			if err := AppendInline(node, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, child}}); err != nil {
				return nil, err
			}
		}
		return node, nil
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return FallbackEncode[any](nil)
		}
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for i := 0; i < value.Len(); i++ {
			child, err := EncodeMixedWithNaming(value.Index(i), naming)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case reflect.Map:
		if value.IsNil() {
			return FallbackEncode[any](nil)
		}
		if value.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key must be string")
		}
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, key := range keys {
			child, err := EncodeMixedWithNaming(value.MapIndex(key), naming)
			if err != nil {
				return nil, err
			}
			keyNode, err := FallbackEncode(key.Interface())
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, keyNode, child)
		}
		return node, nil
	}
	var node yaml.Node
	if err := node.Encode(value.Interface()); err != nil {
		return nil, err
	}
	return &node, nil
}

func parseYAMLFieldTag(field reflect.StructField, text string, modes ...FieldNaming) (name string, inline, excluded bool, err error) {
	parts := strings.Split(text, ",")
	name = parts[0]
	if name == "" {
		naming := FieldNamingYAML
		if len(modes) > 0 {
			naming = modes[0]
		}
		name = FieldName(field.Name, "", naming)
	}
	if name == "-" {
		return name, false, true, nil
	}
	seen := map[string]bool{}
	for _, flag := range parts[1:] {
		if seen[flag] {
			return "", false, false, fmt.Errorf("duplicate yaml flag %q", flag)
		}
		seen[flag] = true
		switch flag {
		case "omitempty", "flow":
		case "inline":
			inline = true
		default:
			return "", false, false, fmt.Errorf("unsupported yaml flag %q", flag)
		}
	}
	if inline && parts[0] != "" {
		return "", false, false, fmt.Errorf("inline field cannot have a YAML name")
	}
	return name, inline, false, nil
}

func interfaceValue(value reflect.Value) any {
	if value.CanInterface() {
		return value.Interface()
	}
	return nil
}

func keepsNull(typ reflect.Type) bool {
	if typ == nodeType {
		return true
	}
	switch typ.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return true
	}
	return false
}
