package yamlvalidator

import (
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// generatedValueSymbol records a registry lookup while the generator lowers a
// tag. It is replaced before a generated schema is used for validation.
type generatedValueSymbol struct {
	Name    string
	Args    map[string]any
	Factory bool
}

func (generatedValueSymbol) Validate(*yaml.Node, string, *ValidationContext) {}

type generatedKeySymbol struct {
	Name    string
	Args    map[string]any
	Factory bool
}

func (generatedKeySymbol) ValidateKey(string, *yaml.Node, string, *ValidationContext) {}

type GeneratedValidatorSpec struct {
	Kind       string         `json:"kind"`
	Name       string         `json:"name,omitempty"`
	Text       string         `json:"text,omitempty"`
	Number     int            `json:"number,omitempty"`
	Minimum    bool           `json:"minimum,omitempty"`
	Properties bool           `json:"properties,omitempty"`
	Names      []string       `json:"names,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
	Factory    bool           `json:"factory,omitempty"`
}

type GeneratedNormalizedNode struct {
	Type                 NodeType                 `json:"type"`
	AllowedTypes         []NodeType               `json:"allowedTypes,omitempty"`
	Required             bool                     `json:"required,omitempty"`
	Nullable             bool                     `json:"nullable,omitempty"`
	Deprecated           string                   `json:"deprecated,omitempty"`
	Description          string                   `json:"description,omitempty"`
	Default              any                      `json:"default,omitempty"`
	DefaultPresent       bool                     `json:"defaultPresent,omitempty"`
	SourceIndex          int                      `json:"sourceIndex,omitempty"`
	FieldRules           bool                     `json:"fieldRules,omitempty"`
	Ref                  string                   `json:"ref,omitempty"`
	AllowedKeys          map[string]int           `json:"allowedKeys,omitempty"`
	HasAllowedKeys       bool                     `json:"hasAllowedKeys,omitempty"`
	AdditionalProperties *int                     `json:"additionalProperties,omitempty"`
	ValueSchema          *int                     `json:"valueSchema,omitempty"`
	InlineCapture        *int                     `json:"inlineCapture,omitempty"`
	ItemSchema           *int                     `json:"itemSchema,omitempty"`
	ExtraSchemas         []int                    `json:"extraSchemas,omitempty"`
	OneOfSchemas         []int                    `json:"oneOfSchemas,omitempty"`
	AnyOfSchemas         []int                    `json:"anyOfSchemas,omitempty"`
	UnknownKeyPolicy     UnknownKeyPolicy         `json:"unknownKeyPolicy,omitempty"`
	MinItems             *int                     `json:"minItems,omitempty"`
	MaxItems             *int                     `json:"maxItems,omitempty"`
	Validators           []GeneratedValidatorSpec `json:"validators,omitempty"`
	KeyValidators        []GeneratedValidatorSpec `json:"keyValidators,omitempty"`
	AnyOf                [][]string               `json:"anyOf,omitempty"`
	ExactlyOneOf         []string                 `json:"exactlyOneOf,omitempty"`
	MutuallyExclusive    []string                 `json:"mutuallyExclusive,omitempty"`
	Conditions           []ConditionalRule        `json:"conditions,omitempty"`
	OneOfRequired        [][]string               `json:"oneOfRequired,omitempty"`
	ForbiddenTogether    [][]string               `json:"forbiddenTogether,omitempty"`
	DependentRequired    map[string][]string      `json:"dependentRequired,omitempty"`
	ExactlyGroups        [][]string               `json:"exactlyGroups,omitempty"`
	MutuallyGroups       [][]string               `json:"mutuallyGroups,omitempty"`
	AnyClauses           [][][]string             `json:"anyClauses,omitempty"`
	OneClauses           [][][]string             `json:"oneClauses,omitempty"`
	RequiredNames        []string                 `json:"requiredNames,omitempty"`
}

type GeneratedNormalizedGraph struct {
	Root  int                       `json:"root"`
	Nodes []GeneratedNormalizedNode `json:"nodes"`
}

// LowerGeneratedSchema runs tag normalization during source generation. The
// returned graph contains only native schema properties, edges and validator
// specifications; generated programs never receive tag language declarations.
func LowerGeneratedSchema(graph GeneratedSchemaGraph, root int) (GeneratedNormalizedGraph, error) {
	schema, err := graph.lower(root)
	if err != nil {
		return GeneratedNormalizedGraph{}, err
	}
	result := GeneratedNormalizedGraph{}
	ids := map[*FieldSchema]int{}
	var visit func(*FieldSchema) (int, error)
	visit = func(s *FieldSchema) (int, error) {
		if s == nil {
			return 0, fmt.Errorf("nil generated schema node")
		}
		if id, ok := ids[s]; ok {
			return id, nil
		}
		id := len(result.Nodes)
		ids[s] = id
		result.Nodes = append(result.Nodes, GeneratedNormalizedNode{})
		n := GeneratedNormalizedNode{
			Type: s.Type, AllowedTypes: s.AllowedTypes, Required: s.Required, Nullable: s.Nullable,
			Deprecated: s.Deprecated, Description: s.Description, Default: s.Default,
			DefaultPresent: s.defaultPresent, SourceIndex: s.generatedSourceIndex, FieldRules: s.generatedFieldRules, Ref: s.generatedRef,
			HasAllowedKeys: s.AllowedKeys != nil, UnknownKeyPolicy: s.UnknownKeyPolicy,
			MinItems: s.MinItems, MaxItems: s.MaxItems, AnyOf: s.AnyOf,
			ExactlyOneOf: s.ExactlyOneOf, MutuallyExclusive: s.MutuallyExclusive,
			Conditions: s.Conditions, OneOfRequired: s.OneOfRequired,
			ForbiddenTogether: s.ForbiddenTogether, DependentRequired: s.DependentRequired,
			ExactlyGroups: s.exactlyGroups, MutuallyGroups: s.mutuallyGroups,
			AnyClauses: s.anyClauses, OneClauses: s.oneClauses, RequiredNames: s.requiredNames,
		}
		for _, validator := range s.Validators {
			spec, err := generatedValueSpec(validator)
			if err != nil {
				return 0, err
			}
			n.Validators = append(n.Validators, spec)
		}
		for _, validator := range s.KeyValidators {
			spec, err := generatedKeySpec(validator)
			if err != nil {
				return 0, err
			}
			n.KeyValidators = append(n.KeyValidators, spec)
		}
		if s.AllowedKeys != nil {
			n.AllowedKeys = make(map[string]int, len(s.AllowedKeys))
			keys := make([]string, 0, len(s.AllowedKeys))
			for key := range s.AllowedKeys {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := s.AllowedKeys[key]
				childID, err := visit(child)
				if err != nil {
					return 0, err
				}
				n.AllowedKeys[key] = childID
			}
		}
		for _, edge := range []struct {
			source *FieldSchema
			target **int
		}{
			{s.AdditionalProperties, &n.AdditionalProperties}, {s.ValueSchema, &n.ValueSchema},
			{s.inlineCapture, &n.InlineCapture}, {s.ItemSchema, &n.ItemSchema},
		} {
			if edge.source != nil {
				childID, err := visit(edge.source)
				if err != nil {
					return 0, err
				}
				*edge.target = &childID
			}
		}
		for _, child := range s.extraSchemas {
			childID, err := visit(child)
			if err != nil {
				return 0, err
			}
			n.ExtraSchemas = append(n.ExtraSchemas, childID)
		}
		for _, child := range s.OneOfSchemas {
			childID, err := visit(child)
			if err != nil {
				return 0, err
			}
			n.OneOfSchemas = append(n.OneOfSchemas, childID)
		}
		for _, child := range s.AnyOfSchemas {
			childID, err := visit(child)
			if err != nil {
				return 0, err
			}
			n.AnyOfSchemas = append(n.AnyOfSchemas, childID)
		}
		result.Nodes[id] = n
		return id, nil
	}
	result.Root, err = visit(schema)
	if err != nil {
		return GeneratedNormalizedGraph{}, err
	}
	return result, nil
}

func generatedValueSpec(v ValueValidator) (GeneratedValidatorSpec, error) {
	switch x := v.(type) {
	case representableValidator:
		return GeneratedValidatorSpec{Kind: "representable", Number: x.bits, Text: x.kind.String()}, nil
	case byteRepresentationValidator:
		return GeneratedValidatorSpec{Kind: "bytes"}, nil
	case mappingShapeRule:
		return GeneratedValidatorSpec{Kind: "mapping"}, nil
	case notNullRule:
		return GeneratedValidatorSpec{Kind: "notnull"}, nil
	case nonemptyRule:
		return GeneratedValidatorSpec{Kind: "nonempty"}, nil
	case uniqueItemsRule:
		return GeneratedValidatorSpec{Kind: "uniqueItems"}, nil
	case rangeRule:
		return GeneratedValidatorSpec{Kind: "range", Text: x.bound.RatString(), Minimum: x.minimum}, nil
	case lengthRule:
		return GeneratedValidatorSpec{Kind: "length", Number: x.bound, Minimum: x.minimum, Properties: x.properties}, nil
	case enumRule:
		return GeneratedValidatorSpec{Kind: "enum", Names: []string(x)}, nil
	case patternRule:
		return GeneratedValidatorSpec{Kind: "pattern", Text: x.re.String()}, nil
	case urlRule:
		return GeneratedValidatorSpec{Kind: "url", Minimum: x.requireScheme, Names: x.schemes}, nil
	case nativeFormatRule:
		return GeneratedValidatorSpec{Kind: "format", Name: x.name}, nil
	case generatedValueSymbol:
		return GeneratedValidatorSpec{Kind: "check", Name: x.Name, Args: x.Args, Factory: x.Factory}, nil
	default:
		return GeneratedValidatorSpec{}, fmt.Errorf("unsupported generated value validator %T", v)
	}
}

func generatedKeySpec(v KeyValidator) (GeneratedValidatorSpec, error) {
	switch x := v.(type) {
	case keyPatternRule:
		return GeneratedValidatorSpec{Kind: "pattern", Text: x.re.String()}, nil
	case keyLengthRule:
		return GeneratedValidatorSpec{Kind: "length", Number: x.bound, Minimum: x.minimum}, nil
	case nativeKeyFormatRule:
		return GeneratedValidatorSpec{Kind: "format", Name: x.name}, nil
	case generatedKeySymbol:
		return GeneratedValidatorSpec{Kind: "check", Name: x.Name, Args: x.Args, Factory: x.Factory}, nil
	default:
		return GeneratedValidatorSpec{}, fmt.Errorf("unsupported generated key validator %T", v)
	}
}

// BuildGeneratedSchema allocates a normalized graph, wires its references and
// resolves registry symbols. typeIDs are identity tokens emitted by the source
// generator; they are never inspected for fields or tags.
func BuildGeneratedSchema(graph GeneratedNormalizedGraph, typeIDs []reflect.Type, registry *Registry, encode bool) (*FieldSchema, error) {
	if graph.Root < 0 || graph.Root >= len(graph.Nodes) {
		return nil, fmt.Errorf("invalid generated root")
	}
	nodes := make([]*FieldSchema, len(graph.Nodes))
	for i, n := range graph.Nodes {
		s := &FieldSchema{
			Type: n.Type, AllowedTypes: n.AllowedTypes, Required: n.Required, Nullable: n.Nullable,
			Deprecated: n.Deprecated, Description: n.Description, Default: n.Default,
			defaultPresent: n.DefaultPresent, UnknownKeyPolicy: n.UnknownKeyPolicy,
			MinItems: n.MinItems, MaxItems: n.MaxItems, AnyOf: n.AnyOf,
			ExactlyOneOf: n.ExactlyOneOf, MutuallyExclusive: n.MutuallyExclusive,
			Conditions: n.Conditions, OneOfRequired: n.OneOfRequired,
			ForbiddenTogether: n.ForbiddenTogether, DependentRequired: n.DependentRequired,
			exactlyGroups: n.ExactlyGroups, mutuallyGroups: n.MutuallyGroups,
			anyClauses: n.AnyClauses, oneClauses: n.OneClauses, requiredNames: n.RequiredNames,
		}
		if n.HasAllowedKeys {
			s.AllowedKeys = make(map[string]*FieldSchema, len(n.AllowedKeys))
		}
		for _, v := range n.Validators {
			actual, err := instantiateGeneratedValue(v, registry)
			if err != nil {
				return nil, err
			}
			s.Validators = append(s.Validators, actual)
		}
		for _, v := range n.KeyValidators {
			actual, err := instantiateGeneratedKey(v, registry)
			if err != nil {
				return nil, err
			}
			s.KeyValidators = append(s.KeyValidators, actual)
		}
		nodes[i] = s
	}
	lookup := func(id int) (*FieldSchema, error) {
		if id < 0 || id >= len(nodes) {
			return nil, fmt.Errorf("invalid generated schema edge %d", id)
		}
		return nodes[id], nil
	}
	for i, n := range graph.Nodes {
		s := nodes[i]
		for key, id := range n.AllowedKeys {
			child, err := lookup(id)
			if err != nil {
				return nil, err
			}
			s.AllowedKeys[key] = child
		}
		for _, edge := range []struct {
			id     *int
			target **FieldSchema
		}{{n.AdditionalProperties, &s.AdditionalProperties}, {n.ValueSchema, &s.ValueSchema}, {n.InlineCapture, &s.inlineCapture}, {n.ItemSchema, &s.ItemSchema}} {
			if edge.id != nil {
				child, err := lookup(*edge.id)
				if err != nil {
					return nil, err
				}
				*edge.target = child
			}
		}
		for _, id := range n.ExtraSchemas {
			child, err := lookup(id)
			if err != nil {
				return nil, err
			}
			s.extraSchemas = append(s.extraSchemas, child)
		}
		for _, id := range n.OneOfSchemas {
			child, err := lookup(id)
			if err != nil {
				return nil, err
			}
			s.OneOfSchemas = append(s.OneOfSchemas, child)
		}
		for _, id := range n.AnyOfSchemas {
			child, err := lookup(id)
			if err != nil {
				return nil, err
			}
			s.AnyOfSchemas = append(s.AnyOfSchemas, child)
		}
		if n.Ref != "" {
			if registry == nil || registry.schemas[n.Ref] == nil {
				return nil, fmt.Errorf("unknown schema %q", n.Ref)
			}
			*s = *cloneFieldSchema(registry.schemas[n.Ref], map[*FieldSchema]*FieldSchema{})
		}
	}
	for i, n := range graph.Nodes {
		if n.SourceIndex <= 0 || n.SourceIndex > len(typeIDs) || registry == nil {
			continue
		}
		binding, ok := registry.bindings[typeIDs[n.SourceIndex-1]]
		if !ok {
			continue
		}
		selected := binding.Decode
		if encode {
			selected = binding.Encode
		}
		if selected != nil {
			bound := cloneFieldSchema(selected, map[*FieldSchema]*FieldSchema{})
			if n.FieldRules {
				bound = mergeGeneratedBinding(bound, nodes[i])
			} else {
				bound.Nullable = bound.Nullable || n.Nullable
			}
			*nodes[i] = *bound
		}
	}
	root := nodes[graph.Root]
	if err := ValidateFieldSchema(root); err != nil {
		return nil, err
	}
	return root, nil
}

func mergeGeneratedBinding(bound, overlay *FieldSchema) *FieldSchema {
	bound.Required = bound.Required || overlay.Required
	bound.Nullable = bound.Nullable || overlay.Nullable
	bound.Validators = append(bound.Validators, overlay.Validators...)
	bound.KeyValidators = append(bound.KeyValidators, overlay.KeyValidators...)
	if overlay.MinItems != nil {
		bound.MinItems = overlay.MinItems
	}
	if overlay.MaxItems != nil {
		bound.MaxItems = overlay.MaxItems
	}
	if overlay.UnknownKeyPolicy != UnknownKeyInherit {
		bound.UnknownKeyPolicy = overlay.UnknownKeyPolicy
	}
	if overlay.Description != "" {
		bound.Description = overlay.Description
	}
	if overlay.Deprecated != "" {
		bound.Deprecated = overlay.Deprecated
	}
	if overlay.defaultPresent {
		bound.Default, bound.defaultPresent = overlay.Default, true
	}
	if len(overlay.AllowedTypes) > 0 {
		bound.AllowedTypes = append(bound.AllowedTypes, overlay.AllowedTypes...)
	}
	if overlay.AllowedKeys != nil {
		if bound.AllowedKeys == nil {
			bound.AllowedKeys = make(map[string]*FieldSchema)
		}
		for name, child := range overlay.AllowedKeys {
			if bound.AllowedKeys[name] == nil {
				bound.AllowedKeys[name] = child
			}
		}
	}
	if overlay.ItemSchema != nil && bound.ItemSchema == nil {
		bound.ItemSchema = overlay.ItemSchema
	}
	if overlay.ValueSchema != nil && bound.ValueSchema == nil {
		bound.ValueSchema = overlay.ValueSchema
	}
	bound.extraSchemas = append(bound.extraSchemas, overlay.extraSchemas...)
	bound.AnyOf = append(bound.AnyOf, overlay.AnyOf...)
	bound.ExactlyOneOf = append(bound.ExactlyOneOf, overlay.ExactlyOneOf...)
	bound.MutuallyExclusive = append(bound.MutuallyExclusive, overlay.MutuallyExclusive...)
	bound.Conditions = append(bound.Conditions, overlay.Conditions...)
	bound.OneOfRequired = append(bound.OneOfRequired, overlay.OneOfRequired...)
	bound.ForbiddenTogether = append(bound.ForbiddenTogether, overlay.ForbiddenTogether...)
	bound.exactlyGroups = append(bound.exactlyGroups, overlay.exactlyGroups...)
	bound.mutuallyGroups = append(bound.mutuallyGroups, overlay.mutuallyGroups...)
	bound.anyClauses = append(bound.anyClauses, overlay.anyClauses...)
	bound.oneClauses = append(bound.oneClauses, overlay.oneClauses...)
	if len(overlay.DependentRequired) > 0 {
		if bound.DependentRequired == nil {
			bound.DependentRequired = make(map[string][]string)
		}
		for key, names := range overlay.DependentRequired {
			bound.DependentRequired[key] = append(bound.DependentRequired[key], names...)
		}
	}
	return bound
}

func instantiateGeneratedValue(v GeneratedValidatorSpec, registry *Registry) (ValueValidator, error) {
	switch v.Kind {
	case "representable":
		kind := reflectKindByName(v.Text)
		if kind == reflect.Invalid {
			return nil, fmt.Errorf("invalid representable kind %q", v.Text)
		}
		return representableValidator{kind: kind, bits: v.Number}, nil
	case "bytes":
		return byteRepresentationValidator{}, nil
	case "mapping":
		return mappingShapeRule{}, nil
	case "notnull":
		return notNullRule{}, nil
	case "nonempty":
		return nonemptyRule{}, nil
	case "uniqueItems":
		return uniqueItemsRule{}, nil
	case "range":
		bound, ok := new(big.Rat).SetString(v.Text)
		if !ok {
			return nil, fmt.Errorf("invalid generated bound")
		}
		return rangeRule{bound: bound, minimum: v.Minimum}, nil
	case "length":
		return lengthRule{bound: v.Number, minimum: v.Minimum, properties: v.Properties}, nil
	case "enum":
		return enumRule(v.Names), nil
	case "pattern":
		re, err := regexp.Compile(v.Text)
		if err != nil {
			return nil, err
		}
		return patternRule{re: re}, nil
	case "url":
		return urlRule{requireScheme: v.Minimum, schemes: v.Names}, nil
	case "format":
		return nativeFormat(v.Name)
	case "check":
		if registry == nil {
			return nil, fmt.Errorf("check %q requires registry", v.Name)
		}
		if !v.Factory {
			actual := registry.values[v.Name]
			if actual == nil {
				return nil, fmt.Errorf("unknown value validator %q", v.Name)
			}
			return actual, nil
		}
		factory := registry.valueFactories[v.Name]
		if factory == nil {
			return nil, fmt.Errorf("unknown value factory %q", v.Name)
		}
		actual, err := factory(v.Args)
		if err != nil {
			return nil, err
		}
		if nilInterface(actual) {
			return nil, fmt.Errorf("factory %q returned nil", v.Name)
		}
		if definition, ok := actual.(DefinitionValidator); ok {
			if err := definition.ValidateDefinition(); err != nil {
				return nil, fmt.Errorf("factory %q: %w", v.Name, err)
			}
		}
		return actual, nil
	}
	return nil, fmt.Errorf("unknown generated value validator kind %q", v.Kind)
}

func instantiateGeneratedKey(v GeneratedValidatorSpec, registry *Registry) (KeyValidator, error) {
	switch v.Kind {
	case "pattern":
		re, err := regexp.Compile(v.Text)
		if err != nil {
			return nil, err
		}
		return keyPatternRule{re: re}, nil
	case "length":
		return keyLengthRule{bound: v.Number, minimum: v.Minimum}, nil
	case "format":
		f, err := nativeFormat(v.Name)
		if err != nil {
			return nil, err
		}
		return nativeKeyFormatRule{f}, nil
	case "check":
		if registry == nil {
			return nil, fmt.Errorf("key check %q requires registry", v.Name)
		}
		if !v.Factory {
			actual := registry.keys[v.Name]
			if actual == nil {
				return nil, fmt.Errorf("unknown key validator %q", v.Name)
			}
			return actual, nil
		}
		factory := registry.keyFactories[v.Name]
		if factory == nil {
			return nil, fmt.Errorf("unknown key factory %q", v.Name)
		}
		actual, err := factory(v.Args)
		if err != nil {
			return nil, err
		}
		if nilInterface(actual) {
			return nil, fmt.Errorf("key factory %q returned nil", v.Name)
		}
		if definition, ok := actual.(DefinitionValidator); ok {
			if err := definition.ValidateDefinition(); err != nil {
				return nil, fmt.Errorf("key factory %q: %w", v.Name, err)
			}
		}
		return actual, nil
	}
	return nil, fmt.Errorf("unknown generated key validator kind %q", v.Kind)
}

func reflectKindByName(name string) reflect.Kind {
	for _, kind := range []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64} {
		if kind.String() == name {
			return kind
		}
	}
	return reflect.Invalid
}

// GeneratedInt supplies pointer-valued bounds in generated Go literals.
func GeneratedInt(value int) *int { return &value }
