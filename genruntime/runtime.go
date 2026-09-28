package genruntime

import (
	engine "github.com/Yakwilik/go-yamlvalidator/internal/engine"
	"github.com/Yakwilik/go-yamlvalidator/internal/yamlcodec"
	"gopkg.in/yaml.v3"
	"reflect"
)

// Limits bounds an unchecked generated codec operation.
type Limits = yamlcodec.Limits

// Pair is an effective YAML mapping key and value.
type Pair = yamlcodec.Pair

// DecodeContext carries shared limits through a generated object tree.
type DecodeContext = yamlcodec.DecodeContext

// CycleContext tracks the active typed traversal path during encoding.
type CycleContext = yamlcodec.CycleContext

// MarshalYAML adapts a generated encoder to yaml.Marshaler with default validation.
func MarshalYAML(value any) (*yaml.Node, error) { return engine.MarshalGeneratedNode(value) }

// UnmarshalYAML adapts a generated decoder to yaml.Unmarshaler with default validation.
func UnmarshalYAML(node *yaml.Node, dst any) error { return engine.UnmarshalGeneratedNode(node, dst) }

func NewDecodeContext(l Limits, enforce bool) *DecodeContext {
	return yamlcodec.NewDecodeContext(l, enforce)
}
func NewCycleContext(l Limits) *CycleContext            { return yamlcodec.NewCycleContext(l) }
func ResolveNode(n *yaml.Node) (*yaml.Node, error)      { return yamlcodec.ResolveNode(n) }
func MappingPairs(n *yaml.Node) ([]Pair, error)         { return yamlcodec.MappingPairs(n) }
func AppendInline(parent, child *yaml.Node) error       { return yamlcodec.AppendInline(parent, child) }
func IsEmpty(value any) bool                            { return yamlcodec.IsEmpty(value) }
func RejectCycles(value any, l Limits) error            { return yamlcodec.RejectCycles(value, l) }
func ScalarEncode[T any](value T) (*yaml.Node, error)   { return yamlcodec.ScalarEncode(value) }
func ScalarDecode[T any](n *yaml.Node, dst *T) error    { return yamlcodec.ScalarDecode(n, dst) }
func FallbackEncode[T any](value T) (*yaml.Node, error) { return yamlcodec.FallbackEncode(value) }
func FallbackDecode[T any](n *yaml.Node, dst *T) error  { return yamlcodec.FallbackDecode(n, dst) }
func FallbackDecodeWithContext[T any](n *yaml.Node, dst *T, c *DecodeContext) error {
	return yamlcodec.FallbackDecodeWithContext(n, dst, c)
}
func HasGenerated(t reflect.Type, visiting map[reflect.Type]bool) bool {
	return yamlcodec.HasGenerated(t, visiting)
}
func DecodeMixed(n *yaml.Node, dst reflect.Value) error { return yamlcodec.DecodeMixed(n, dst) }
func DecodeMixedWithContext(n *yaml.Node, dst reflect.Value, c *DecodeContext) error {
	return yamlcodec.DecodeMixedWithContext(n, dst, c)
}
func EncodeMixed(value reflect.Value) (*yaml.Node, error) { return yamlcodec.EncodeMixed(value) }
