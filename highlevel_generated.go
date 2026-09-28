package yamlvalidator

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// GeneratedCodec is implemented by yamlvalidator-gen. Its encode and decode
// operations are unchecked; the enclosing operation validates the node once.
type GeneratedCodec interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncode() (*yaml.Node, error)
	YAMLValidatorDecode(*yaml.Node) error
}

type generatedEncoder interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncode() (*yaml.Node, error)
}

type generatedSchemaProvider interface {
	YAMLValidatorSchema(*Registry, bool) (*FieldSchema, error)
}

func generatedPlan(value any, registry *Registry, encode bool) (*highLevelPlan, bool, error) {
	provider, ok := value.(generatedSchemaProvider)
	marker, hasMarker := value.(interface{ YAMLValidatorGeneratedType() reflect.Type })
	if !ok || !hasMarker || !exactGeneratedType(value, marker.YAMLValidatorGeneratedType()) {
		return nil, false, nil
	}
	schema, err := provider.YAMLValidatorSchema(registry, encode)
	if err != nil {
		return nil, true, err
	}
	return &highLevelPlan{schema: schema}, true, nil
}

func exactGeneratedType(value any, declared reflect.Type) bool {
	typ := reflect.TypeOf(value)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ != nil && typ == declared
}

// MarshalGenerated supports the standard yaml.v3 hook, which has no access to
// source bytes or per-call Options. Coordinates come directly from the node.
func MarshalGenerated(value generatedEncoder) (*yaml.Node, error) {
	if nilInterface(value) {
		return GeneratedFallbackEncode[any](nil)
	}
	if !exactGeneratedType(value, value.YAMLValidatorGeneratedType()) {
		return nil, fmt.Errorf("promoted generated YAML method on a different type")
	}
	limits, _ := normalizeLimits(Limits{})
	if err := checkGeneratedCycles(value, limits); err != nil {
		return nil, err
	}
	node, err := value.YAMLValidatorEncode()
	if err != nil {
		return nil, err
	}
	plan, _, err := generatedPlan(value, nil, true)
	if err != nil {
		return nil, err
	}
	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{node}}
	if err := validateHighLevelDocument(root, plan, nil, highLevelRunOptions{limits: limits}); err != nil {
		return nil, err
	}
	return node, nil
}

func checkGeneratedCycles(value any, limits Limits) error {
	if !nilInterface(value) {
		if marker, ok := value.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok && exactGeneratedType(value, marker.YAMLValidatorGeneratedType()) {
			if checker, ok := value.(interface{ YAMLValidatorCheckCycles(Limits) error }); ok {
				return checker.YAMLValidatorCheckCycles(limits)
			}
		}
	}
	return rejectGoCycles(reflect.ValueOf(value), limits)
}

// UnmarshalGenerated supports the standard yaml.v3 hook on a parsed node.
func UnmarshalGenerated(node *yaml.Node, dst GeneratedCodec) error {
	if !exactGeneratedType(dst, dst.YAMLValidatorGeneratedType()) {
		return fmt.Errorf("promoted generated YAML method on a different type")
	}
	plan, _, err := generatedPlan(dst, nil, false)
	if err != nil {
		return err
	}
	root := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{node}}
	limits, _ := normalizeLimits(Limits{})
	if err := validateHighLevelDocument(root, plan, nil, highLevelRunOptions{limits: limits}); err != nil {
		return err
	}
	ctx := NewGeneratedDecodeContext(Limits{}, false)
	if withContext, ok := dst.(interface {
		YAMLValidatorDecodeWithContext(*yaml.Node, *GeneratedDecodeContext) error
	}); ok {
		return withContext.YAMLValidatorDecodeWithContext(node, ctx)
	}
	return dst.YAMLValidatorDecode(node)
}

// GeneratedScalarDecode decodes a scalar or an explicitly opaque custom codec.
func GeneratedScalarDecode[T any](node *yaml.Node, dst *T) error {
	var err error
	node, err = GeneratedResolveNode(node)
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
				v, e := parseGeneratedFloat(node.Value, 32)
				if e == nil {
					*p = float32(v)
					return nil
				}
			}
		case *float64:
			if node.Tag == "!!float" || node.Tag == "!!int" {
				v, e := parseGeneratedFloat(node.Value, 64)
				if e == nil {
					*p = v
					return nil
				}
			}
		}
	}
	return GeneratedFallbackDecode(node, dst)
}

// GeneratedResolveNode follows aliases with a cycle and depth bound.
func GeneratedResolveNode(node *yaml.Node) (*yaml.Node, error) {
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

func parseGeneratedFloat(value string, bits int) (float64, error) {
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

// GeneratedFallbackDecode is for opaque, custom, time, and YAML special types.
func GeneratedFallbackDecode[T any](node *yaml.Node, dst *T) error {
	return GeneratedFallbackDecodeWithContext(node, dst, NewGeneratedDecodeContext(Limits{}, false))
}

func GeneratedFallbackDecodeWithContext[T any](node *yaml.Node, dst *T, ctx *GeneratedDecodeContext) error {
	if codec, ok := any(dst).(GeneratedCodec); ok && exactGeneratedType(dst, codec.YAMLValidatorGeneratedType()) {
		if withContext, ok := any(dst).(interface {
			YAMLValidatorDecodeWithContext(*yaml.Node, *GeneratedDecodeContext) error
		}); ok {
			return withContext.YAMLValidatorDecodeWithContext(node, ctx)
		}
		return codec.YAMLValidatorDecode(node)
	}
	if err := node.Decode(dst); err != nil {
		return fmt.Errorf("decode YAML: %w", err)
	}
	return nil
}

// GeneratedScalarEncode encodes a scalar or an explicitly opaque custom codec.
func GeneratedScalarEncode[T any](value T) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.ScalarNode}
	switch v := any(value).(type) {
	case string:
		node.Tag = "!!str"
		node.Value = v
	case bool:
		node.Tag = "!!bool"
		node.Value = strconv.FormatBool(v)
	case int:
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(int64(v), 10)
	case int8:
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(int64(v), 10)
	case int16:
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(int64(v), 10)
	case int32:
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(int64(v), 10)
	case int64:
		node.Tag = "!!int"
		node.Value = strconv.FormatInt(v, 10)
	case uint:
		node.Tag = "!!int"
		node.Value = strconv.FormatUint(uint64(v), 10)
	case uint8:
		node.Tag = "!!int"
		node.Value = strconv.FormatUint(uint64(v), 10)
	case uint16:
		node.Tag = "!!int"
		node.Value = strconv.FormatUint(uint64(v), 10)
	case uint32:
		node.Tag = "!!int"
		node.Value = strconv.FormatUint(uint64(v), 10)
	case uint64:
		node.Tag = "!!int"
		node.Value = strconv.FormatUint(v, 10)
	case float32:
		node.Tag = "!!float"
		node.Value = generatedFloat(float64(v), 32)
	case float64:
		node.Tag = "!!float"
		node.Value = generatedFloat(v, 64)
	default:
		return GeneratedFallbackEncode(value)
	}
	return node, nil
}

func generatedFloat(value float64, bits int) string {
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

// GeneratedFallbackEncode is for opaque, custom, time, and YAML special types.
func GeneratedFallbackEncode[T any](value T) (*yaml.Node, error) {
	if codec, ok := any(value).(generatedEncoder); ok && !nilInterface(value) && exactGeneratedType(value, codec.YAMLValidatorGeneratedType()) {
		return codec.YAMLValidatorEncode()
	}
	if !nilInterface(value) && mixedHasGenerated(reflect.TypeOf(value), map[reflect.Type]bool{}) {
		limits, _ := normalizeLimits(Limits{})
		if err := rejectGoCycles(reflect.ValueOf(value), limits); err != nil {
			return nil, err
		}
		return encodeMixedNode(reflect.ValueOf(value))
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, fmt.Errorf("encode YAML: %w", err)
	}
	return &node, nil
}

// GeneratedPair is a key and value from a mapping after YAML merge expansion.
type GeneratedPair struct {
	Key   string
	Value *yaml.Node
}

// GeneratedAppendInline flattens a mapping while rejecting key collisions.
func GeneratedAppendInline(parent, child *yaml.Node) error {
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

// GeneratedMappingPairs applies YAML merge precedence and rejects explicit
// duplicate keys, matching yaml.v3's mapping decode behavior.
func GeneratedMappingPairs(node *yaml.Node) ([]GeneratedPair, error) {
	return NewGeneratedDecodeContext(Limits{}, false).MappingPairs(node)
}

// GeneratedIsEmpty implements yaml.v3's omitempty check for a field value.
func GeneratedIsEmpty(value any) bool {
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
	}
	if v.Kind() == reflect.Struct {
		for i := v.NumField() - 1; i >= 0; i-- {
			if v.Type().Field(i).PkgPath != "" {
				continue
			}
			if !GeneratedIsEmpty(v.Field(i).Interface()) {
				return false
			}
		}
		return true
	}
	return false
}
