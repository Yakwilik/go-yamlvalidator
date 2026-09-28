package yamlvalidator

import (
	"fmt"
	"reflect"

	"github.com/Yakwilik/go-yamlvalidator/genruntime"
	"gopkg.in/yaml.v3"
)

type generatedCodec interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncode() (*yaml.Node, error)
	YAMLValidatorDecode(*yaml.Node) error
}

type generatedContextDecoder interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorDecodeWithContext(*yaml.Node, *genruntime.DecodeContext) error
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
	if !ok || !hasMarker {
		return nil, false, nil
	}
	typ := marker.YAMLValidatorGeneratedType()
	if !exactGeneratedType(value, typ) {
		return nil, false, nil
	}
	cache := &defaultPlanCache
	if registry != nil {
		cache = &registry.cache
	}
	plan, err := cache.getOrCompile(planKey{typ: typ, encode: encode}, func() (*highLevelPlan, error) {
		schema, err := provider.YAMLValidatorSchema(registry, encode)
		if err != nil {
			return nil, err
		}
		return &highLevelPlan{schema: schema}, nil
	})
	return plan, true, err
}

func exactGeneratedType(value any, declared reflect.Type) bool {
	typ := reflect.TypeOf(value)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ != nil && typ == declared
}

// MarshalGenerated is the small bridge used by generated yaml.v3 hooks.
// Codec mechanics live in genruntime; validation remains owned by this package.
func MarshalGenerated(value any) (*yaml.Node, error) {
	if nilInterface(value) {
		return genruntime.FallbackEncode[any](nil)
	}
	codec, ok := value.(generatedEncoder)
	if !ok {
		return nil, fmt.Errorf("value does not implement generated YAML encoder")
	}
	if !exactGeneratedType(value, codec.YAMLValidatorGeneratedType()) {
		return nil, fmt.Errorf("promoted generated YAML method on a different type")
	}
	limits, _ := normalizeLimits(Limits{})
	if err := checkGeneratedCycles(value, limits); err != nil {
		return nil, err
	}
	node, err := codec.YAMLValidatorEncode()
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
		if marker, ok := value.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok &&
			exactGeneratedType(value, marker.YAMLValidatorGeneratedType()) {
			if checker, ok := value.(interface{ YAMLValidatorCheckCycles(Limits) error }); ok {
				return checker.YAMLValidatorCheckCycles(limits)
			}
		}
	}
	return rejectGoCycles(reflect.ValueOf(value), limits)
}

// UnmarshalGenerated is the bridge used by generated yaml.v3 hooks.
func UnmarshalGenerated(node *yaml.Node, dst any) error {
	codec, ok := dst.(generatedCodec)
	if !ok {
		return fmt.Errorf("destination does not implement generated YAML codec")
	}
	if !exactGeneratedType(dst, codec.YAMLValidatorGeneratedType()) {
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
	ctx := genruntime.NewDecodeContext(toGeneratedLimits(Limits{}), false)
	if withContext, ok := dst.(generatedContextDecoder); ok {
		return withContext.YAMLValidatorDecodeWithContext(node, ctx)
	}
	return codec.YAMLValidatorDecode(node)
}

func toGeneratedLimits(limits Limits) genruntime.Limits {
	return genruntime.Limits{
		MaxDepth:      limits.MaxDepth,
		MaxNodeVisits: limits.MaxNodeVisits,
	}
}
