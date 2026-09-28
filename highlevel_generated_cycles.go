package yamlvalidator

import (
	"fmt"
	"reflect"
)

type generatedCycleIdentity struct {
	typ reflect.Type
	ptr uintptr
	len int
}

// GeneratedCycleContext bounds a generated typed traversal. Reflection here
// obtains container identity only; generated functions visit known fields.
type GeneratedCycleContext struct {
	limits Limits
	visits int
	active map[generatedCycleIdentity]bool
}

func NewGeneratedCycleContext(limits Limits) *GeneratedCycleContext {
	normalized, _ := normalizeLimits(limits)
	return &GeneratedCycleContext{limits: normalized, active: make(map[generatedCycleIdentity]bool)}
}

func (c *GeneratedCycleContext) Enter(value any, depth int) (func(), error) {
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
	identity := generatedCycleIdentity{typ: v.Type(), ptr: ptr}
	if v.Kind() == reflect.Slice {
		identity.len = v.Len()
	}
	if c.active[identity] {
		return nil, fmt.Errorf("cyclic Go value cannot be encoded as YAML")
	}
	c.active[identity] = true
	return func() { delete(c.active, identity) }, nil
}

func (c *GeneratedCycleContext) CheckDynamic(value any, depth int) error {
	if depth > c.limits.MaxDepth {
		return fmt.Errorf("Go value exceeds depth %d", c.limits.MaxDepth)
	}
	return rejectGoCycles(reflect.ValueOf(value), c.limits)
}
