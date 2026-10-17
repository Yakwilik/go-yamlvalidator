package yamlcodec

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FieldNaming selects the default YAML name of an exported Go field.
type FieldNaming uint8

const (
	FieldNamingYAML FieldNaming = iota
	FieldNamingGo
)

// FieldName preserves explicit YAML names in either mode.
func FieldName(goName, yamlName string, naming FieldNaming) string {
	if yamlName != "" {
		return yamlName
	}
	if naming == FieldNamingGo {
		return goName
	}
	return strings.ToLower(goName)
}

func ValidateFieldNaming(naming FieldNaming) error {
	if naming != FieldNamingYAML && naming != FieldNamingGo {
		return fmt.Errorf("invalid field naming mode %d", naming)
	}
	return nil
}

type generatedNamingEncoder interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorEncodeWithNaming(FieldNaming) (*yaml.Node, error)
}

type generatedNamingValidator interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorValidateFieldNaming(FieldNaming, ...bool) error
}

var nodeType = reflect.TypeFor[yaml.Node]()
var timestampType = reflect.TypeFor[time.Time]()

func specialRepresentation(typ reflect.Type) bool {
	return typ == nodeType || typ == timestampType || typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8
}

// ValidateFieldNames checks codec metadata before decoding can mutate a value.
// It deliberately does not compile validation rules or invoke user codecs.
func ValidateFieldNames(typ reflect.Type, naming FieldNaming, encode bool) error {
	return (&FieldNamingContext{Naming: naming, Encode: encode}).Validate(typ)
}

// FieldNamingContext shares the active type path through generated fallbacks.
// Each operation owns its context; recursive types never restart the traversal.
type FieldNamingContext struct {
	Naming   FieldNaming
	Encode   bool
	visiting map[reflect.Type]bool
}

func (c *FieldNamingContext) Validate(typ reflect.Type) error {
	if c == nil {
		return fmt.Errorf("nil field naming context")
	}
	naming, encode := c.Naming, c.Encode
	if err := ValidateFieldNaming(naming); err != nil {
		return err
	}
	if typ == nil {
		return nil
	}
	if c.visiting == nil {
		c.visiting = map[reflect.Type]bool{}
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if c.visiting[typ] || specialRepresentation(typ) {
		return nil
	}
	c.visiting[typ] = true
	defer delete(c.visiting, typ)
	if typ.Kind() != reflect.Interface {
		candidate := reflect.New(typ).Interface()
		if validator, ok := candidate.(generatedNamingContextValidator); ok && validator.YAMLValidatorGeneratedType() == typ {
			return validator.YAMLValidatorValidateFieldNamingWithContext(c)
		}
		if validator, ok := candidate.(generatedNamingValidator); ok && validator.YAMLValidatorGeneratedType() == typ {
			return validator.YAMLValidatorValidateFieldNaming(naming, encode)
		}
		_, generated := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type })
		if !generated {
			custom := IsCustomUnmarshaler(typ)
			if encode {
				custom = typ.Implements(yamlMarshalerType) || reflect.PointerTo(typ).Implements(yamlMarshalerType) || typ.Implements(textMarshalerType) || reflect.PointerTo(typ).Implements(textMarshalerType)
			}
			if custom {
				return nil
			}
		}
	}
	switch typ.Kind() {
	case reflect.Struct:
		if _, err := structFields(typ, naming, map[reflect.Type]bool{}, encode); err != nil {
			return err
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !includeField(field) {
				continue
			}
			_, _, excluded, err := parseYAMLFieldTag(field, field.Tag.Get("yaml"), naming)
			if err != nil {
				return err
			}
			if excluded {
				continue
			}
			if err := c.Validate(field.Type); err != nil {
				return err
			}
		}
	case reflect.Map, reflect.Array, reflect.Slice:
		return c.Validate(typ.Elem())
	}
	return nil
}

type mixedField struct {
	name  string
	index []int
}

type mixedFields struct {
	fields       []mixedField
	inlineMap    []int
	inlineCodecs [][]int
}

func includeField(field reflect.StructField) bool {
	return field.IsExported() || field.Anonymous && strings.Contains(","+field.Tag.Get("yaml")+",", ",inline,")
}

// structFields flattens inline structs once for both decoding and conflict checks.
func structFields(typ reflect.Type, naming FieldNaming, active map[reflect.Type]bool, encode bool) (mixedFields, error) {
	if active[typ] {
		return mixedFields{}, fmt.Errorf("recursive inline YAML struct %s", typ)
	}
	active[typ] = true
	defer delete(active, typ)
	var result mixedFields
	known := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !includeField(field) {
			continue
		}
		name, inline, excluded, err := parseYAMLFieldTag(field, field.Tag.Get("yaml"), naming)
		if err != nil {
			return result, err
		}
		if excluded {
			continue
		}
		if !inline {
			if known[name] {
				return result, fmt.Errorf("duplicate YAML key %q in %s", name, typ)
			}
			known[name] = true
			result.fields = append(result.fields, mixedField{name, []int{i}})
			continue
		}
		base := field.Type
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		if base.Kind() != reflect.Interface {
			candidate := reflect.New(base).Interface()
			if names, ok := candidate.(generatedFieldNames); ok && names.YAMLValidatorGeneratedType() == base {
				for _, key := range names.YAMLValidatorFieldNamesWithNaming(naming) {
					if known[key] {
						return result, fmt.Errorf("inline YAML key collision: %q in %s", key, typ)
					}
					known[key] = true
					result.fields = append(result.fields, mixedField{name: key})
				}
				result.inlineCodecs = append(result.inlineCodecs, []int{i})
				continue
			}
			_, generated := candidate.(interface{ YAMLValidatorGeneratedType() reflect.Type })
			custom := IsCustomUnmarshaler(base)
			if encode {
				custom = base.Implements(yamlMarshalerType) || reflect.PointerTo(base).Implements(yamlMarshalerType) || base.Implements(textMarshalerType) || reflect.PointerTo(base).Implements(textMarshalerType)
			}
			if !generated && custom {
				result.inlineCodecs = append(result.inlineCodecs, []int{i})
				continue
			}
		}
		switch base.Kind() {
		case reflect.Struct:
			child, err := structFields(base, naming, active, encode)
			if err != nil {
				return result, err
			}
			for _, index := range child.inlineCodecs {
				result.inlineCodecs = append(result.inlineCodecs, append([]int{i}, index...))
			}
			for _, entry := range child.fields {
				if known[entry.name] {
					return result, fmt.Errorf("inline YAML key collision: %q in %s", entry.name, typ)
				}
				known[entry.name] = true
				index := entry.index
				if len(index) > 0 {
					index = append([]int{i}, index...)
				}
				result.fields = append(result.fields, mixedField{entry.name, index})
			}
			if len(child.inlineMap) > 0 {
				if len(result.inlineMap) > 0 {
					return result, fmt.Errorf("multiple inline YAML maps in %s", typ)
				}
				result.inlineMap = append([]int{i}, child.inlineMap...)
			}
		case reflect.Map:
			if base.Key().Kind() != reflect.String {
				return result, fmt.Errorf("inline YAML map key must be string")
			}
			if len(result.inlineMap) > 0 {
				return result, fmt.Errorf("multiple inline YAML maps in %s", typ)
			}
			result.inlineMap = []int{i}
		default:
			return result, fmt.Errorf("inline field must be a struct or string-keyed map")
		}
	}
	return result, nil
}

func fieldByIndex(value reflect.Value, index []int, allocate bool) (reflect.Value, error) {
	for _, i := range index {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				if !allocate {
					return reflect.Value{}, nil
				}
				if !value.CanSet() {
					return reflect.Value{}, fmt.Errorf("cannot allocate unexported inline pointer %s", value.Type())
				}
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}
		value = value.Field(i)
	}
	return value, nil
}

var obsoleteUnmarshalerType = reflect.TypeFor[interface{ UnmarshalYAML(func(any) error) error }]()

// IsCustomUnmarshaler recognizes both yaml.v3 hook signatures and text codecs.
func IsCustomUnmarshaler(typ reflect.Type) bool {
	for _, candidate := range []reflect.Type{typ, reflect.PointerTo(typ)} {
		if candidate.Implements(yamlUnmarshalerType) || candidate.Implements(obsoleteUnmarshalerType) || candidate.Implements(textUnmarshalerType) {
			return true
		}
	}
	return false
}

type generatedFieldNames interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorFieldNamesWithNaming(FieldNaming) []string
}

type generatedNamingContextValidator interface {
	YAMLValidatorGeneratedType() reflect.Type
	YAMLValidatorValidateFieldNamingWithContext(*FieldNamingContext) error
}
