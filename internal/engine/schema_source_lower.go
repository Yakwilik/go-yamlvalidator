package yamlvalidator

import (
	"fmt"
	"reflect"

	genspec "github.com/Yakwilik/go-yamlvalidator/internal/genspec"
)

func lowerGeneratedSourceGraph(g genspec.SourceGraph, rootID int) (*FieldSchema, error) {
	registry, encode, symbolic := (*Registry)(nil), false, true
	g.Root = rootID
	if g.Root < 0 || g.Root >= len(g.Nodes) {
		return nil, fmt.Errorf("invalid generated schema root")
	}
	reachable := make([]bool, len(g.Nodes))
	var mark func(int)
	mark = func(id int) {
		if id < 0 || id >= len(g.Nodes) || reachable[id] {
			return
		}
		reachable[id] = true
		spec := g.Nodes[id]
		mark(spec.AliasOf)
		mark(spec.Item)
		mark(spec.Value)
		for _, field := range spec.Fields {
			mark(field.Node)
		}
	}
	mark(g.Root)
	nodes := make([]*FieldSchema, len(g.Nodes))
	bound := make([]bool, len(nodes))
	for i, spec := range g.Nodes {
		nodes[i] = &FieldSchema{Type: NodeType(spec.Type), Nullable: spec.Nullable, generatedSourceIndex: i + 1}
		if NodeType(spec.Type) == TypeMap && spec.Fields != nil {
			nodes[i].AllowedKeys = make(map[string]*FieldSchema, len(spec.Fields))
		}
		if spec.NumberKind != reflect.Invalid {
			nodes[i].Validators = append(nodes[i].Validators, representableValidator{kind: spec.NumberKind, bits: spec.NumberBits})
		}
		if spec.ArrayLen >= 0 {
			n := spec.ArrayLen
			nodes[i].MinItems, nodes[i].MaxItems = &n, &n
		}
		if spec.ByteSlice {
			nodes[i].AllowedTypes = []NodeType{TypeSequence, TypeString}
			nodes[i].Validators = append(nodes[i].Validators, byteRepresentationValidator{})
			nodes[i].ItemSchema = &FieldSchema{Type: TypeInt, Validators: []ValueValidator{representableValidator{kind: reflect.Uint8, bits: 8}}}
		}
	}
	fields := make([][]*FieldSchema, len(nodes))
	for i, spec := range g.Nodes {
		if spec.Item >= 0 {
			if spec.Item >= len(nodes) {
				return nil, fmt.Errorf("invalid generated item node")
			}
			nodes[i].ItemSchema = nodes[spec.Item]
		}
		if spec.Value >= 0 {
			if spec.Value >= len(nodes) {
				return nil, fmt.Errorf("invalid generated value node")
			}
			nodes[i].ValueSchema = nodes[spec.Value]
			nodes[i].AdditionalProperties = &FieldSchema{Type: TypeAny}
		}
		fields[i] = make([]*FieldSchema, len(spec.Fields))
		for j, field := range spec.Fields {
			if field.Node < 0 || field.Node >= len(nodes) {
				return nil, fmt.Errorf("invalid generated field node")
			}
			fields[i][j] = &FieldSchema{}
			if !field.Inline {
				if _, exists := nodes[i].AllowedKeys[field.Key]; exists {
					return nil, fmt.Errorf("duplicate YAML key %q", field.Key)
				}
				nodes[i].AllowedKeys[field.Key] = fields[i][j]
			}
		}
	}
	// Bindings select a supplied schema by type identity. No Go field metadata
	// or source tag is inspected here.
	for i, spec := range g.Nodes {
		if !reachable[i] {
			continue
		}
		if registry == nil || spec.GoType == nil {
			continue
		}
		binding, ok := registry.bindings[spec.GoType]
		if !ok {
			continue
		}
		selected := binding.Decode
		if encode {
			selected = binding.Encode
		}
		if selected != nil {
			*nodes[i] = *cloneFieldSchema(selected, make(map[*FieldSchema]*FieldSchema))
			bound[i] = true
		}
	}
	for i, spec := range g.Nodes {
		if spec.AliasOf < 0 || bound[i] {
			continue
		}
		if spec.AliasOf >= len(nodes) {
			return nil, fmt.Errorf("invalid generated alias node")
		}
		*nodes[i] = *nodes[spec.AliasOf]
		nodes[i].Nullable = true
		nodes[i].generatedSourceIndex = i + 1
	}
	for i, spec := range g.Nodes {
		for j, field := range spec.Fields {
			*fields[i][j] = *nodes[field.Node]
		}
	}
	compiler := &highLevelCompiler{registry: registry, encode: encode, symbolic: symbolic}
	// Descendants are emitted after their parents. Apply their field rules first
	// so inline flattening sees the completed child schema.
	for i := len(g.Nodes) - 1; i >= 0; i-- {
		if !reachable[i] {
			continue
		}
		spec := g.Nodes[i]
		if bound[i] {
			continue
		}
		for j, field := range spec.Fields {
			child := fields[i][j]
			if len(field.Rules) > 0 {
				*child = *cloneFieldSchema(child, make(map[*FieldSchema]*FieldSchema))
				child.generatedFieldRules = true
				rules := generatedTagRules(field.Rules)
				value, _, err := normalizeFieldRules(rules)
				if err != nil {
					return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
				}
				if err := compiler.applyRules(child, value, false); err != nil {
					return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
				}
				if err := validateHighLevelObjectReferences(child, value); err != nil {
					return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
				}
				if err := resolveHighLevelRequired(child); err != nil {
					return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
				}
				deduplicateHighLevelGroups(child)
			}
			if !field.Inline {
				continue
			}
			if child.Type != TypeMap {
				return nil, fmt.Errorf("field %s: inline field must be a mapping", field.FieldName)
			}
			if child.AllowedKeys == nil {
				if nodes[i].inlineCapture != nil {
					return nil, fmt.Errorf("multiple inline maps are unsupported")
				}
				nodes[i].inlineCapture = child
				nodes[i].AdditionalProperties = &FieldSchema{Type: TypeAny}
				continue
			}
			for key, value := range child.AllowedKeys {
				if _, exists := nodes[i].AllowedKeys[key]; exists {
					return nil, fmt.Errorf("inline YAML key collision: %s", key)
				}
				nodes[i].AllowedKeys[key] = value
			}
			mergeInlineObjectRules(nodes[i], child)
		}
		for _, field := range spec.Fields {
			if len(field.Rules) == 0 {
				continue
			}
			_, parent, err := normalizeFieldRules(generatedTagRules(field.Rules))
			if err != nil {
				return nil, err
			}
			if len(parent) > 0 {
				nodes[i].generatedFieldRules = true
			}
			if err := compiler.applyRules(nodes[i], parent, true); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
			}
			if err := validateHighLevelObjectReferences(nodes[i], parent); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.FieldName, err)
			}
		}
		if err := resolveHighLevelRequired(nodes[i]); err != nil {
			return nil, err
		}
		deduplicateHighLevelGroups(nodes[i])
	}
	if err := ValidateFieldSchema(nodes[g.Root]); err != nil {
		return nil, err
	}
	return nodes[g.Root], nil
}

func generatedTagRules(rules []genspec.Rule) []tagRule {
	out := make([]tagRule, len(rules))
	for i, rule := range rules {
		out[i] = tagRule{key: rule.Key, value: generatedTagValue(rule.Value), hasValue: rule.HasValue, offset: rule.Offset}
	}
	return out
}

func generatedTagValue(value genspec.RuleValue) tagValue {
	out := tagValue{kind: value.Kind, text: value.Text, rules: generatedTagRules(value.Rules)}
	for _, item := range value.List {
		out.list = append(out.list, generatedTagValue(item))
	}
	return out
}
