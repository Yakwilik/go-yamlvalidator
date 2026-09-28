package yamlvalidator

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Yakwilik/go-yamlvalidator/internal/taglang"
	"gopkg.in/yaml.v3"
)

type highLevelPlan struct{ schema *FieldSchema }

var defaultPlanCache planCache
var yamlNodeType = reflect.TypeFor[yaml.Node]()
var timeType = reflect.TypeFor[time.Time]()
var durationType = reflect.TypeFor[time.Duration]()
var textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
var yamlMarshalerType = reflect.TypeFor[yaml.Marshaler]()
var yamlUnmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()
var reflectSchemaCompileCalls atomic.Uint64
var generatedStructReflectionCalls atomic.Uint64

func compileHighLevel(typ reflect.Type, encode bool, registry *Registry) (*highLevelPlan, error) {
	reflectSchemaCompileCalls.Add(1)
	key := planKey{typ, encode}
	cache := &defaultPlanCache
	if registry != nil {
		cache = &registry.cache
	}
	return cache.getOrCompile(key, func() (*highLevelPlan, error) {
		compiler := &highLevelCompiler{encode: encode, registry: registry, visiting: make(map[reflect.Type]bool), schemas: make(map[reflect.Type]*FieldSchema)}
		schema, err := compiler.infer(typ)
		if err != nil {
			return nil, err
		}
		if err := ValidateFieldSchema(schema); err != nil {
			return nil, &SchemaError{Type: typ, Reason: err.Error()}
		}
		return &highLevelPlan{schema: schema}, nil
	})
}

type highLevelCompiler struct {
	encode        bool
	registry      *Registry
	symbolic      bool
	visiting      map[reflect.Type]bool
	schemas       map[reflect.Type]*FieldSchema
	inlineTargets map[reflect.Type]int
}

type highLevelRuleOrigin struct {
	typ        reflect.Type
	field, tag string
	rules      []tagRule
}

func (c *highLevelCompiler) infer(typ reflect.Type) (*FieldSchema, error) {
	if typ == nil {
		return &FieldSchema{Type: TypeAny, Nullable: true}, nil
	}
	original := typ
	nullable := false
	for typ.Kind() == reflect.Pointer {
		nullable = true
		typ = typ.Elem()
	}
	if typ == yamlNodeType {
		return &FieldSchema{Type: TypeAny, Nullable: true}, nil
	}
	if c.registry != nil {
		if binding, ok := c.registry.bindings[original]; ok {
			selected := binding.Decode
			if c.encode {
				selected = binding.Encode
			}
			if selected != nil {
				return cloneFieldSchema(selected, make(map[*FieldSchema]*FieldSchema)), nil
			}
		}
		if binding, ok := c.registry.bindings[typ]; ok {
			selected := binding.Decode
			if c.encode {
				selected = binding.Encode
			}
			if selected != nil {
				schema := cloneFieldSchema(selected, make(map[*FieldSchema]*FieldSchema))
				schema.Nullable = schema.Nullable || nullable
				return schema, nil
			}
		}
	}
	if typ.Kind() != reflect.Interface {
		candidate := reflect.New(typ).Interface()
		if provider, ok := candidate.(generatedSchemaProvider); ok {
			if marker, ok := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok && marker.YAMLValidatorGeneratedType() == typ {
				schema, err := provider.YAMLValidatorSchema(c.registry, c.encode)
				if err != nil {
					return nil, err
				}
				schema.Nullable = schema.Nullable || nullable
				return schema, nil
			}
		}
	}
	if c.visiting[typ] {
		schema := c.schemas[typ]
		if nullable {
			copy := *schema
			copy.Nullable = true
			return &copy, nil
		}
		return schema, nil
	}
	if typ == timeType {
		return &FieldSchema{Type: TypeString, Nullable: nullable}, nil
	}
	if typ == durationType {
		return &FieldSchema{Type: TypeString, Nullable: nullable}, nil
	}
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
		item := &FieldSchema{Type: TypeInt, Validators: []ValueValidator{representableValidator{kind: reflect.Uint8, bits: 8}}}
		return &FieldSchema{Type: TypeSequence, AllowedTypes: []NodeType{TypeSequence, TypeString}, Nullable: true, ItemSchema: item, Validators: []ValueValidator{byteRepresentationValidator{}}}, nil
	}
	if isCustomCodec(typ, c.encode) {
		return nil, &SchemaError{Type: typ, Reason: "custom YAML/text codec requires a type binding or explicit type=any"}
	}
	schema := &FieldSchema{Nullable: nullable}
	if c.schemas == nil {
		c.schemas = make(map[reflect.Type]*FieldSchema)
	}
	c.schemas[typ] = schema
	switch typ.Kind() {
	case reflect.String:
		schema.Type = TypeString
	case reflect.Bool:
		schema.Type = TypeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema.Type = TypeInt
		schema.Validators = append(schema.Validators, representableValidator{kind: typ.Kind(), bits: typ.Bits()})
	case reflect.Float32, reflect.Float64:
		schema.Type = TypeFloat
		schema.Validators = append(schema.Validators, representableValidator{kind: typ.Kind(), bits: typ.Bits()})
	case reflect.Interface:
		if typ.NumMethod() > 0 {
			return nil, &SchemaError{Type: typ, Reason: "nonempty interface requires explicit type binding"}
		}
		schema.Type = TypeAny
		schema.Nullable = true
	case reflect.Struct:
		schema.Type = TypeMap
		c.visiting[typ] = true
		defer delete(c.visiting, typ)
		if err := c.compileStruct(typ, schema); err != nil {
			return nil, err
		}
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			return nil, &SchemaError{Type: typ, Reason: "map key type must be string or named string"}
		}
		schema.Type = TypeMap
		schema.Nullable = true
		c.visiting[typ] = true
		defer delete(c.visiting, typ)
		child, err := c.infer(typ.Elem())
		if err != nil {
			return nil, err
		}
		schema.ValueSchema = child
		schema.AdditionalProperties = &FieldSchema{Type: TypeAny}
	case reflect.Slice, reflect.Array:
		schema.Type = TypeSequence
		schema.Nullable = nullable || typ.Kind() == reflect.Slice
		c.visiting[typ] = true
		defer delete(c.visiting, typ)
		child, err := c.infer(typ.Elem())
		if err != nil {
			return nil, err
		}
		schema.ItemSchema = child
		if typ.Kind() == reflect.Array {
			n := typ.Len()
			schema.MinItems = &n
			schema.MaxItems = &n
		}
	default:
		return nil, &SchemaError{Type: typ, Reason: "unsupported Go kind " + typ.Kind().String()}
	}
	return schema, nil
}

func isCustomCodec(typ reflect.Type, encode bool) bool {
	if typ.Kind() != reflect.Interface {
		candidate := reflect.New(typ).Interface()
		if _, ok := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok {
			return false
		}
	}
	if encode {
		return typ.Implements(yamlMarshalerType) || reflect.PointerTo(typ).Implements(yamlMarshalerType) || typ.Implements(textMarshalerType) || reflect.PointerTo(typ).Implements(textMarshalerType)
	}
	return typ.Implements(yamlUnmarshalerType) || reflect.PointerTo(typ).Implements(yamlUnmarshalerType) || typ.Implements(textUnmarshalerType) || reflect.PointerTo(typ).Implements(textUnmarshalerType)
}

func (c *highLevelCompiler) compileStruct(typ reflect.Type, schema *FieldSchema) error {
	if candidate := reflect.New(typ).Interface(); candidate != nil {
		if marker, ok := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok && marker.YAMLValidatorGeneratedType() == typ {
			generatedStructReflectionCalls.Add(1)
		}
	}
	schema.AllowedKeys = make(map[string]*FieldSchema)
	var fields []reflect.StructField
	if marker, ok := reflect.New(typ).Interface().(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok && marker.YAMLValidatorGeneratedType() == typ {
		if provider, ok := reflect.New(typ).Interface().(interface{ YAMLValidatorTypePlan() SourceTypePlan }); ok {
			plan := provider.YAMLValidatorTypePlan()
			if plan.Type != typ || len(plan.Fields) != typ.NumField() {
				return &SchemaError{Type: typ, Reason: "generated source type plan does not match Go type"}
			}
			fields = plan.Fields
		}
	}
	if fields == nil {
		for i := 0; i < typ.NumField(); i++ {
			fields = append(fields, typ.Field(i))
		}
	}
	for _, field := range fields {
		tags, err := parseOuterStructTag(string(field.Tag))
		if err != nil {
			return &SchemaError{Type: typ, Field: field.Name, Tag: string(field.Tag), Offset: schemaErrorOffset(err), Reason: err.Error()}
		}
		validationText, hasValidation := tags["yamlvalidate"]
		if _, hasJSON := tags["yamljsonschema"]; hasJSON {
			return &SchemaError{Type: typ, Field: field.Name, Reason: "yamljsonschema is unsupported"}
		}
		if field.Name == "_" && hasValidation {
			return &SchemaError{Type: typ, Field: field.Name, Tag: validationText, Reason: "validation tag on blank field"}
		}
		inlineEmbedded := field.Anonymous && strings.Contains(","+tags["yaml"]+",", ",inline,")
		if field.PkgPath != "" && !inlineEmbedded {
			if hasValidation {
				return &SchemaError{Type: typ, Field: field.Name, Reason: "validation tag on unexported field"}
			}
			continue
		}
		name, inline, excluded, err := parseYAMLFieldTag(field, tags["yaml"])
		if err != nil {
			return &SchemaError{Type: typ, Field: field.Name, Tag: tags["yaml"], Reason: err.Error()}
		}
		if excluded {
			if hasValidation {
				return &SchemaError{Type: typ, Field: field.Name, Reason: "validation tag on YAML-excluded field"}
			}
			continue
		}
		var valueRules, parentRules []tagRule
		if hasValidation {
			rules, err := parseTagRules(validationText)
			if err != nil {
				return wrapFieldError(typ, field, validationText, err)
			}
			valueRules, parentRules, err = normalizeFieldRules(rules)
			if err != nil {
				return wrapFieldError(typ, field, validationText, err)
			}
		}
		if len(parentRules) > 0 {
			schema.objectOrigins = append(schema.objectOrigins, highLevelRuleOrigin{typ, field.Name, validationText, parentRules})
		}
		if inline {
			base := derefType(field.Type)
			if base.Kind() == reflect.Struct {
				if c.inlineTargets == nil {
					c.inlineTargets = make(map[reflect.Type]int)
				}
				c.inlineTargets[base]++
			}
			child, err := c.inferField(field.Type, valueRules)
			if base.Kind() == reflect.Struct {
				c.inlineTargets[base]--
			}
			if err != nil {
				return wrapFieldError(typ, field, validationText, err)
			}
			if child.Type != TypeMap {
				return &SchemaError{Type: typ, Field: field.Name, Reason: "inline field must be a struct or string-keyed map"}
			}
			if child.AllowedKeys != nil {
				if err := validateHighLevelObjectReferences(child, valueRules); err != nil {
					return wrapFieldError(typ, field, validationText, err)
				}
				if child.UnknownKeyPolicy != UnknownKeyInherit || child.MinItems != nil || child.MaxItems != nil || child.AdditionalProperties != nil || child.ValueSchema != nil || hasPropertyLengthRule(child) {
					return &SchemaError{Type: typ, Field: field.Name, Reason: "inline struct cannot set local closedness or size constraints"}
				}
				for key, value := range child.AllowedKeys {
					if _, exists := schema.AllowedKeys[key]; exists {
						return &SchemaError{Type: typ, Field: field.Name, Reason: "inline YAML key collision: " + key}
					}
					schema.AllowedKeys[key] = value
				}
				mergeInlineObjectRules(schema, child)
				continue
			}
			if err := validateHighLevelObjectReferences(child, valueRules); err != nil {
				return wrapFieldError(typ, field, validationText, err)
			}
			if schema.inlineCapture != nil {
				return &SchemaError{Type: typ, Field: field.Name, Reason: "multiple inline maps are unsupported"}
			}
			schema.inlineCapture = child
			schema.AdditionalProperties = &FieldSchema{Type: TypeAny}
			continue
		}
		if _, exists := schema.AllowedKeys[name]; exists {
			return &SchemaError{Type: typ, Field: field.Name, Reason: "duplicate YAML key " + name}
		}
		child, err := c.inferField(field.Type, valueRules)
		if err != nil {
			return wrapFieldError(typ, field, validationText, err)
		}
		if err := validateHighLevelObjectReferences(child, valueRules); err != nil {
			return wrapFieldError(typ, field, validationText, err)
		}
		if err := resolveHighLevelRequired(child); err != nil {
			return wrapFieldError(typ, field, validationText, err)
		}
		schema.AllowedKeys[name] = child
	}
	if c.inlineTargets[typ] == 0 {
		for _, origin := range schema.objectOrigins {
			if err := c.applyRules(schema, origin.rules, true); err != nil {
				return &SchemaError{Type: origin.typ, Field: origin.field, Tag: origin.tag, Offset: schemaErrorOffset(err), Reason: err.Error()}
			}
			if err := validateHighLevelObjectReferences(schema, origin.rules); err != nil {
				return &SchemaError{Type: origin.typ, Field: origin.field, Tag: origin.tag, Offset: schemaErrorOffset(err), Reason: err.Error()}
			}
		}
		if err := resolveHighLevelRequired(schema); err != nil {
			return &SchemaError{Type: typ, Reason: err.Error()}
		}
		deduplicateHighLevelGroups(schema)
		schema.objectOrigins = nil
	}
	return nil
}

func normalizeFieldRules(rules []tagRule) (value, parent []tagRule, err error) {
	v, p, err := taglang.Normalize(toSharedRules(rules), taglang.FieldScope)
	if err != nil {
		return nil, nil, err
	}
	return fromSharedRules(v), fromSharedRules(p), nil
}

func normalizeValueRules(rules []tagRule) ([]tagRule, error) {
	v, _, err := taglang.Normalize(toSharedRules(rules), taglang.ValueScope)
	if err != nil {
		return nil, err
	}
	return fromSharedRules(v), nil
}

func (c *highLevelCompiler) applyNestedRules(schema *FieldSchema, rules []tagRule) error {
	value, err := normalizeValueRules(rules)
	if err != nil {
		return err
	}
	if err := c.applyRules(schema, value, false); err != nil {
		return err
	}
	deduplicateHighLevelGroups(schema)
	return nil
}

func hasPropertyLengthRule(schema *FieldSchema) bool {
	for _, validator := range schema.Validators {
		if v, ok := validator.(lengthRule); ok && v.properties {
			return true
		}
	}
	return false
}

func deduplicateHighLevelGroups(schema *FieldSchema) {
	unique := func(groups [][]string) [][]string {
		seen := make(map[string]bool, len(groups))
		out := groups[:0]
		for _, group := range groups {
			members := append([]string(nil), group...)
			sort.Strings(members)
			key := fmt.Sprintf("%q", members)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, group)
		}
		return out
	}
	schema.exactlyGroups = unique(schema.exactlyGroups)
	schema.mutuallyGroups = unique(schema.mutuallyGroups)
	schema.ForbiddenTogether = unique(schema.ForbiddenTogether)
	seenRequired := map[string]bool{}
	required := schema.requiredNames[:0]
	for _, name := range schema.requiredNames {
		if seenRequired[name] {
			continue
		}
		seenRequired[name] = true
		required = append(required, name)
	}
	schema.requiredNames = required
	uniqueClauses := func(clauses [][][]string) [][][]string {
		seen := make(map[string]bool, len(clauses))
		out := clauses[:0]
		for _, clause := range clauses {
			var parts []string
			for _, group := range clause {
				members := append([]string(nil), group...)
				sort.Strings(members)
				parts = append(parts, fmt.Sprintf("%q", members))
			}
			sort.Strings(parts)
			key := fmt.Sprintf("%q", parts)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, clause)
		}
		return out
	}
	schema.anyClauses = uniqueClauses(schema.anyClauses)
	schema.oneClauses = uniqueClauses(schema.oneClauses)
	seenConditions := make(map[string]bool, len(schema.Conditions))
	conditions := schema.Conditions[:0]
	for _, condition := range schema.Conditions {
		required := append([]string(nil), condition.ThenRequired...)
		forbidden := append([]string(nil), condition.ThenForbidden...)
		sort.Strings(required)
		sort.Strings(forbidden)
		key := fmt.Sprintf("%q|%q|%q|%q", condition.ConditionField, condition.ConditionValue, required, forbidden)
		if seenConditions[key] {
			continue
		}
		seenConditions[key] = true
		conditions = append(conditions, condition)
	}
	schema.Conditions = conditions
}
func mergeInlineObjectRules(parent, child *FieldSchema) {
	parent.Validators = append(parent.Validators, child.Validators...)
	parent.KeyValidators = append(parent.KeyValidators, child.KeyValidators...)
	parent.AnyOf = append(parent.AnyOf, child.AnyOf...)
	parent.ExactlyOneOf = append(parent.ExactlyOneOf, child.ExactlyOneOf...)
	parent.MutuallyExclusive = append(parent.MutuallyExclusive, child.MutuallyExclusive...)
	parent.OneOfRequired = append(parent.OneOfRequired, child.OneOfRequired...)
	parent.ForbiddenTogether = append(parent.ForbiddenTogether, child.ForbiddenTogether...)
	parent.Conditions = append(parent.Conditions, child.Conditions...)
	parent.exactlyGroups = append(parent.exactlyGroups, child.exactlyGroups...)
	parent.mutuallyGroups = append(parent.mutuallyGroups, child.mutuallyGroups...)
	parent.anyClauses = append(parent.anyClauses, child.anyClauses...)
	parent.oneClauses = append(parent.oneClauses, child.oneClauses...)
	parent.extraSchemas = append(parent.extraSchemas, child.extraSchemas...)
	parent.pendingRequired = append(parent.pendingRequired, child.pendingRequired...)
	parent.requiredNames = append(parent.requiredNames, child.requiredNames...)
	parent.objectOrigins = append(parent.objectOrigins, child.objectOrigins...)
	if len(child.DependentRequired) > 0 {
		if parent.DependentRequired == nil {
			parent.DependentRequired = make(map[string][]string)
		}
		for key, values := range child.DependentRequired {
			parent.DependentRequired[key] = append(parent.DependentRequired[key], values...)
		}
	}
}

func (c *highLevelCompiler) inferField(typ reflect.Type, rules []tagRule) (*FieldSchema, error) {
	// Explicit type=any deliberately makes an otherwise opaque custom codec safe
	// to use without invoking it during compilation.
	if explicitAny(rules) && isCustomCodec(derefType(typ), c.encode) {
		schema := &FieldSchema{Type: TypeAny, Nullable: isNilCapable(typ)}
		if err := c.applyRules(schema, rules, false); err != nil {
			return nil, err
		}
		deduplicateHighLevelGroups(schema)
		return schema, nil
	}
	schema, err := c.infer(typ)
	if err != nil {
		return nil, err
	}
	inferred := schema.Type
	shapeAllowed := map[NodeType]bool{inferred: true}
	for _, allowed := range schema.AllowedTypes {
		shapeAllowed[allowed] = true
	}
	if isNilCapable(typ) {
		shapeAllowed[TypeNull] = true
	}
	if err := c.applyRules(schema, rules, false); err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.key == "nullable" && !isNilCapable(typ) {
			return nil, &tagOffsetError{Offset: rule.offset, Reason: "nullable requires a nil-capable Go type or binding"}
		}
	}
	if inferred != TypeAny {
		for _, rule := range rules {
			if rule.key == "type" && schema.Type == TypeAny {
				return nil, &tagOffsetError{Offset: rule.offset, Reason: fmt.Sprintf("type=any would widen inferred %s", inferred)}
			}
			if rule.key != "types" {
				continue
			}
			for _, allowed := range schema.AllowedTypes {
				if shapeAllowed[allowed] || inferred == TypeFloat && allowed == TypeInt {
					continue
				}
				return nil, &tagOffsetError{Offset: rule.offset, Reason: fmt.Sprintf("allowed type %s is incompatible with inferred %s", allowed, inferred)}
			}
		}
	}
	if base := derefType(typ); base.Kind() == reflect.Array {
		n := base.Len()
		if schema.MinItems != nil && *schema.MinItems > n || schema.MaxItems != nil && *schema.MaxItems < n {
			offset := 0
			for _, rule := range rules {
				if rule.key == "minItems" || rule.key == "maxItems" {
					offset = rule.offset
				}
			}
			return nil, &tagOffsetError{Offset: offset, Reason: fmt.Sprintf("array length %d conflicts with item bounds", n)}
		}
		schema.MinItems = &n
		schema.MaxItems = &n
	}
	if err := validateHighLevelFieldDefinition(schema, typ, rules); err != nil {
		return nil, err
	}
	deduplicateHighLevelGroups(schema)
	return schema, nil
}

func explicitAny(rules []tagRule) bool {
	for _, r := range rules {
		if r.key == "type" && r.hasValue && r.value.kind == tagAtom && r.value.text == "any" {
			return true
		}
	}
	return false
}
func derefType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}
func isNilCapable(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return true
	}
	return false
}
func wrapFieldError(typ reflect.Type, field reflect.StructField, tag string, err error) error {
	return &SchemaError{Type: typ, Field: field.Name, Tag: tag, Offset: schemaErrorOffset(err), Reason: err.Error()}
}
func schemaErrorOffset(err error) int {
	var offset *tagOffsetError
	if errors.As(err, &offset) {
		return offset.Offset
	}
	return 0
}

func parseYAMLFieldTag(field reflect.StructField, text string) (name string, inline, excluded bool, err error) {
	parts := strings.Split(text, ",")
	name = parts[0]
	if name == "" {
		name = strings.ToLower(field.Name)
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

func parseOuterStructTag(tag string) (map[string]string, error) {
	return taglang.ParseStructTag(tag)
}

func parseTagInt(value tagValue) (int, error) {
	if value.kind != tagAtom {
		return 0, fmt.Errorf("expected integer")
	}
	n, err := strconv.Atoi(value.text)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", value.text, err)
	}
	return n, nil
}
