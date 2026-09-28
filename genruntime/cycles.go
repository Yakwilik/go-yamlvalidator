package genruntime

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

type cycleIdentity struct {
	typ reflect.Type
	ptr uintptr
	len int
}

type CycleContext struct {
	limits Limits
	visits int
	active map[cycleIdentity]bool
}

func NewCycleContext(limits Limits) *CycleContext {
	return &CycleContext{limits: normalizeLimits(limits), active: make(map[cycleIdentity]bool)}
}

func (c *CycleContext) Enter(value any, depth int) (func(), error) {
	c.visits++
	if c.visits > c.limits.MaxNodeVisits {
		return nil, fmt.Errorf("Go value exceeds %d visits", c.limits.MaxNodeVisits)
	}
	if depth > c.limits.MaxDepth {
		return nil, fmt.Errorf("Go value exceeds depth %d", c.limits.MaxDepth)
	}
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return func() {}, nil
	}
	var ptr uintptr
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			ptr = v.Pointer()
		}
	case reflect.Slice:
		if !v.IsNil() && v.Len() > 0 {
			ptr = v.Pointer()
		}
	}
	if ptr == 0 {
		return func() {}, nil
	}
	id := cycleIdentity{typ: v.Type(), ptr: ptr}
	if v.Kind() == reflect.Slice {
		id.len = v.Len()
	}
	if c.active[id] {
		return nil, fmt.Errorf("cyclic Go value cannot be encoded as YAML")
	}
	c.active[id] = true
	return func() { delete(c.active, id) }, nil
}

func (c *CycleContext) CheckDynamic(value any, depth int) error {
	if depth > c.limits.MaxDepth {
		return fmt.Errorf("Go value exceeds depth %d", c.limits.MaxDepth)
	}
	return RejectCycles(value, c.limits)
}

var yamlMarshalerType = reflect.TypeFor[yaml.Marshaler]()
var textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()

func RejectCycles(value any, limits Limits) error {
	limits = normalizeLimits(limits)
	type identity struct {
		typ     reflect.Type
		pointer uintptr
		length  int
	}
	seen := map[identity]bool{}
	visits := 0
	var walk func(reflect.Value, int) error
	walk = func(v reflect.Value, depth int) error {
		if !v.IsValid() {
			return nil
		}
		visits++
		if visits > limits.MaxNodeVisits {
			return fmt.Errorf("Go value exceeds %d visits", limits.MaxNodeVisits)
		}
		if depth > limits.MaxDepth {
			return fmt.Errorf("Go value exceeds depth %d", limits.MaxDepth)
		}
		for v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
			return nil
		}
		if v.Type().Implements(yamlMarshalerType) || v.Type().Implements(textMarshalerType) {
			generated := false
			if v.CanInterface() {
				if _, ok := v.Interface().(interface{ YAMLValidatorGeneratedType() reflect.Type }); ok {
					generated = true
				}
			}
			if !generated {
				return nil
			}
		}
		var pointer uintptr
		switch v.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
			if v.IsNil() {
				return nil
			}
			if v.Kind() == reflect.Slice && v.Len() == 0 {
				return nil
			}
			pointer = v.Pointer()
		}
		if pointer != 0 {
			length := 0
			if v.Kind() == reflect.Slice {
				length = v.Len()
			}
			key := identity{typ: v.Type(), pointer: pointer, length: length}
			if seen[key] {
				return fmt.Errorf("cyclic Go value cannot be encoded as YAML")
			}
			seen[key] = true
			defer delete(seen, key)
		}
		switch v.Kind() {
		case reflect.Pointer:
			return walk(v.Elem(), depth+1)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				field := v.Type().Field(i)
				if field.PkgPath != "" || strings.Split(field.Tag.Get("yaml"), ",")[0] == "-" {
					continue
				}
				if strings.Contains(","+field.Tag.Get("yaml")+",", ",omitempty,") && v.Field(i).IsZero() {
					continue
				}
				if err := walk(v.Field(i), depth+1); err != nil {
					return err
				}
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				if err := walk(iter.Value(), depth+1); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if err := walk(v.Index(i), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(reflect.ValueOf(value), 0)
}
