package yamlcodec

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type generatedCodec interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncode() (*yaml.Node, error)
	YAMLValidatorDecode(*yaml.Node) error
}

type generatedContextDecoder interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorDecodeWithContext(*yaml.Node, *DecodeContext) error
}

type generatedEncoder interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncode() (*yaml.Node, error)
}

func exactGeneratedType(value any, declared reflect.Type) bool {
	typ := reflect.TypeOf(value)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ != nil && typ == declared
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func ScalarDecode[T any](node *yaml.Node, dst *T) error {
	var err error
	node, err = ResolveNode(node)
	if err != nil {
		return err
	}
	if node.Kind == yaml.ScalarNode {
		switch p := any(dst).(type) {
		case *string:
			if node.Tag == "!!str" {
				*p = node.Value
				return nil
			}
		case *bool:
			if node.Tag == "!!bool" {
				switch node.Value {
				case "true", "True", "TRUE":
					*p = true
					return nil
				case "false", "False", "FALSE":
					*p = false
					return nil
				}
			}
		case *int:
			if node.Tag == "!!int" {
				v, e := strconv.ParseInt(node.Value, 0, strconv.IntSize)
				if e == nil {
					*p = int(v)
					return nil
				}
			}
		case *int8:
			if node.Tag == "!!int" {
				v, e := strconv.ParseInt(node.Value, 0, 8)
				if e == nil {
					*p = int8(v)
					return nil
				}
			}
		case *int16:
			if node.Tag == "!!int" {
				v, e := strconv.ParseInt(node.Value, 0, 16)
				if e == nil {
					*p = int16(v)
					return nil
				}
			}
		case *int32:
			if node.Tag == "!!int" {
				v, e := strconv.ParseInt(node.Value, 0, 32)
				if e == nil {
					*p = int32(v)
					return nil
				}
			}
		case *int64:
			if node.Tag == "!!int" {
				v, e := strconv.ParseInt(node.Value, 0, 64)
				if e == nil {
					*p = v
					return nil
				}
			}
		case *uint:
			if node.Tag == "!!int" {
				v, e := strconv.ParseUint(node.Value, 0, strconv.IntSize)
				if e == nil {
					*p = uint(v)
					return nil
				}
			}
		case *uint8:
			if node.Tag == "!!int" {
				v, e := strconv.ParseUint(node.Value, 0, 8)
				if e == nil {
					*p = uint8(v)
					return nil
				}
			}
		case *uint16:
			if node.Tag == "!!int" {
				v, e := strconv.ParseUint(node.Value, 0, 16)
				if e == nil {
					*p = uint16(v)
					return nil
				}
			}
		case *uint32:
			if node.Tag == "!!int" {
				v, e := strconv.ParseUint(node.Value, 0, 32)
				if e == nil {
					*p = uint32(v)
					return nil
				}
			}
		case *uint64:
			if node.Tag == "!!int" {
				v, e := strconv.ParseUint(node.Value, 0, 64)
				if e == nil {
					*p = v
					return nil
				}
			}
		case *float32:
			if node.Tag == "!!float" || node.Tag == "!!int" {
				v, e := parseFloat(node.Value, 32)
				if e == nil {
					*p = float32(v)
					return nil
				}
			}
		case *float64:
			if node.Tag == "!!float" || node.Tag == "!!int" {
				v, e := parseFloat(node.Value, 64)
				if e == nil {
					*p = v
					return nil
				}
			}
		}
	}
	return FallbackDecode(node, dst)
}

func ResolveNode(node *yaml.Node) (*yaml.Node, error) {
	seen := map[*yaml.Node]bool{}
	for steps := 0; node != nil && node.Kind == yaml.AliasNode; steps++ {
		if seen[node] || steps >= 128 {
			return nil, fmt.Errorf("cyclic or excessive YAML alias chain")
		}
		seen[node] = true
		node = node.Alias
	}
	if node == nil {
		return nil, fmt.Errorf("nil YAML node")
	}
	return node, nil
}

func parseFloat(value string, bits int) (float64, error) {
	switch strings.ToLower(value) {
	case ".nan":
		return math.NaN(), nil
	case ".inf", "+.inf":
		return math.Inf(1), nil
	case "-.inf":
		return math.Inf(-1), nil
	}
	return strconv.ParseFloat(value, bits)
}

func FallbackDecode[T any](node *yaml.Node, dst *T) error {
	return FallbackDecodeWithContext(node, dst, NewDecodeContext(Limits{}, false))
}

func FallbackDecodeWithContext[T any](node *yaml.Node, dst *T, ctx *DecodeContext) error {
	if decoder, ok := any(dst).(generatedContextDecoder); ok && exactGeneratedType(dst, decoder.YAMLValidatorGeneratedType()) {
		return decoder.YAMLValidatorDecodeWithContext(node, ctx)
	}
	if codec, ok := any(dst).(generatedCodec); ok && exactGeneratedType(dst, codec.YAMLValidatorGeneratedType()) {
		return codec.YAMLValidatorDecode(node)
	}
	if err := node.Decode(dst); err != nil {
		return fmt.Errorf("decode YAML: %w", err)
	}
	return nil
}

func ScalarEncode[T any](value T) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.ScalarNode}
	switch v := any(value).(type) {
	case string:
		node.Tag, node.Value = "!!str", v
	case bool:
		node.Tag, node.Value = "!!bool", strconv.FormatBool(v)
	case int:
		node.Tag, node.Value = "!!int", strconv.FormatInt(int64(v), 10)
	case int8:
		node.Tag, node.Value = "!!int", strconv.FormatInt(int64(v), 10)
	case int16:
		node.Tag, node.Value = "!!int", strconv.FormatInt(int64(v), 10)
	case int32:
		node.Tag, node.Value = "!!int", strconv.FormatInt(int64(v), 10)
	case int64:
		node.Tag, node.Value = "!!int", strconv.FormatInt(v, 10)
	case uint:
		node.Tag, node.Value = "!!int", strconv.FormatUint(uint64(v), 10)
	case uint8:
		node.Tag, node.Value = "!!int", strconv.FormatUint(uint64(v), 10)
	case uint16:
		node.Tag, node.Value = "!!int", strconv.FormatUint(uint64(v), 10)
	case uint32:
		node.Tag, node.Value = "!!int", strconv.FormatUint(uint64(v), 10)
	case uint64:
		node.Tag, node.Value = "!!int", strconv.FormatUint(v, 10)
	case float32:
		node.Tag, node.Value = "!!float", formatFloat(float64(v), 32)
	case float64:
		node.Tag, node.Value = "!!float", formatFloat(v, 64)
	default:
		return FallbackEncode(value)
	}
	return node, nil
}

func formatFloat(value float64, bits int) string {
	if math.IsNaN(value) {
		return ".nan"
	}
	if math.IsInf(value, 1) {
		return ".inf"
	}
	if math.IsInf(value, -1) {
		return "-.inf"
	}
	return strconv.FormatFloat(value, 'g', -1, bits)
}

func FallbackEncode[T any](value T) (*yaml.Node, error) {
	if codec, ok := any(value).(generatedEncoder); ok && !nilInterface(value) && exactGeneratedType(value, codec.YAMLValidatorGeneratedType()) {
		return codec.YAMLValidatorEncode()
	}
	if !nilInterface(value) && HasGenerated(reflect.TypeOf(value), map[reflect.Type]bool{}) {
		if err := RejectCycles(value, Limits{}); err != nil {
			return nil, err
		}
		return EncodeMixed(reflect.ValueOf(value))
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, fmt.Errorf("encode YAML: %w", err)
	}
	return &node, nil
}

func AppendInline(parent, child *yaml.Node) error {
	if child == nil || child.Kind == yaml.ScalarNode && child.Tag == "!!null" {
		return nil
	}
	if parent == nil || parent.Kind != yaml.MappingNode || child.Kind != yaml.MappingNode {
		return fmt.Errorf("inline YAML field must encode as a mapping")
	}
	seen := make(map[string]bool, len(parent.Content)/2+len(child.Content)/2)
	for i := 0; i+1 < len(parent.Content); i += 2 {
		seen[parent.Content[i].Value] = true
	}
	for i := 0; i+1 < len(child.Content); i += 2 {
		key := child.Content[i].Value
		if seen[key] {
			return fmt.Errorf("inline YAML key collision: %q", key)
		}
		seen[key] = true
	}
	parent.Content = append(parent.Content, child.Content...)
	return nil
}

func MappingPairs(node *yaml.Node) ([]Pair, error) {
	return NewDecodeContext(Limits{}, false).MappingPairs(node)
}

func IsEmpty(value any) bool {
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return true
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return true
	}
	if z, ok := value.(interface{ IsZero() bool }); ok {
		return z.IsZero()
	}
	switch v.Kind() {
	case reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	case reflect.Struct:
		for i := v.NumField() - 1; i >= 0; i-- {
			if v.Type().Field(i).PkgPath != "" {
				continue
			}
			if !IsEmpty(v.Field(i).Interface()) {
				return false
			}
		}
		return true
	}
	return false
}
