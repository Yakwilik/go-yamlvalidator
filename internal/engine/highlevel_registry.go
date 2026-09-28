package yamlvalidator

import (
	"fmt"
	"reflect"
	"sync"
)

// ValueValidatorFactory builds a validator from structured tag arguments.
// Numeric arguments are json.Number values, preserving their exact spelling.
type ValueValidatorFactory func(map[string]any) (ValueValidator, error)

// KeyValidatorFactory builds a key validator from structured tag arguments.
type KeyValidatorFactory func(map[string]any) (KeyValidator, error)

// TypeBinding supplies a schema for a type whose YAML representation cannot
// be inferred safely. Encode and Decode may differ.
type TypeBinding struct {
	Encode *FieldSchema
	Decode *FieldSchema
}

// RegistryConfig declares named validation extensions. The constructor takes
// a snapshot of its maps, slices, and cloneable validators.
type RegistryConfig struct {
	ValueValidators map[string]ValueValidator
	KeyValidators   map[string]KeyValidator
	ValueFactories  map[string]ValueValidatorFactory
	KeyFactories    map[string]KeyValidatorFactory
	Schemas         map[string]*FieldSchema
	TypeBindings    map[reflect.Type]TypeBinding
}

// Registry is an immutable collection of high-level schema extensions.
type Registry struct {
	values         map[string]ValueValidator
	keys           map[string]KeyValidator
	valueFactories map[string]ValueValidatorFactory
	keyFactories   map[string]KeyValidatorFactory
	schemas        map[string]*FieldSchema
	bindings       map[reflect.Type]TypeBinding
	cache          planCache
}

// NewRegistry snapshots and validates the supplied registry configuration.
func NewRegistry(config RegistryConfig) (*Registry, error) {
	r := &Registry{
		values:         make(map[string]ValueValidator, len(config.ValueValidators)),
		keys:           make(map[string]KeyValidator, len(config.KeyValidators)),
		valueFactories: make(map[string]ValueValidatorFactory, len(config.ValueFactories)),
		keyFactories:   make(map[string]KeyValidatorFactory, len(config.KeyFactories)),
		schemas:        make(map[string]*FieldSchema, len(config.Schemas)),
		bindings:       make(map[reflect.Type]TypeBinding, len(config.TypeBindings)),
	}
	for name, validator := range config.ValueValidators {
		if name == "" || nilInterface(validator) {
			return nil, fmt.Errorf("invalid value validator %q", name)
		}
		if d, ok := validator.(DefinitionValidator); ok {
			if err := d.ValidateDefinition(); err != nil {
				return nil, fmt.Errorf("value validator %q: %w", name, err)
			}
		}
		if c, ok := validator.(ValueValidatorCloner); ok {
			validator = c.CloneValueValidator()
			if nilInterface(validator) {
				return nil, fmt.Errorf("value validator %q cloner returned nil", name)
			}
			if d, ok := validator.(DefinitionValidator); ok {
				if err := d.ValidateDefinition(); err != nil {
					return nil, fmt.Errorf("cloned value validator %q: %w", name, err)
				}
			}
		}
		r.values[name] = validator
	}
	for name, validator := range config.KeyValidators {
		if name == "" || nilInterface(validator) {
			return nil, fmt.Errorf("invalid key validator %q", name)
		}
		if d, ok := validator.(DefinitionValidator); ok {
			if err := d.ValidateDefinition(); err != nil {
				return nil, fmt.Errorf("key validator %q: %w", name, err)
			}
		}
		if c, ok := validator.(KeyValidatorCloner); ok {
			validator = c.CloneKeyValidator()
			if nilInterface(validator) {
				return nil, fmt.Errorf("key validator %q cloner returned nil", name)
			}
			if d, ok := validator.(DefinitionValidator); ok {
				if err := d.ValidateDefinition(); err != nil {
					return nil, fmt.Errorf("cloned key validator %q: %w", name, err)
				}
			}
		}
		r.keys[name] = validator
	}
	for name, factory := range config.ValueFactories {
		if name == "" || factory == nil {
			return nil, fmt.Errorf("invalid value factory %q", name)
		}
		r.valueFactories[name] = factory
	}
	for name, factory := range config.KeyFactories {
		if name == "" || factory == nil {
			return nil, fmt.Errorf("invalid key factory %q", name)
		}
		r.keyFactories[name] = factory
	}
	for name, schema := range config.Schemas {
		if name == "" {
			return nil, fmt.Errorf("empty registered schema name")
		}
		if err := ValidateFieldSchema(schema); err != nil {
			return nil, fmt.Errorf("schema %q: %w", name, err)
		}
		r.schemas[name] = cloneFieldSchema(schema, make(map[*FieldSchema]*FieldSchema))
	}
	for typ, binding := range config.TypeBindings {
		if typ == nil {
			return nil, fmt.Errorf("nil type binding")
		}
		if binding.Encode != nil {
			if err := ValidateFieldSchema(binding.Encode); err != nil {
				return nil, fmt.Errorf("encode binding %s: %w", typ, err)
			}
		}
		if binding.Decode != nil {
			if err := ValidateFieldSchema(binding.Decode); err != nil {
				return nil, fmt.Errorf("decode binding %s: %w", typ, err)
			}
		}
		r.bindings[typ] = TypeBinding{Encode: cloneFieldSchema(binding.Encode, make(map[*FieldSchema]*FieldSchema)), Decode: cloneFieldSchema(binding.Decode, make(map[*FieldSchema]*FieldSchema))}
	}
	return r, nil
}

type planKey struct {
	typ    reflect.Type
	encode bool
}

// planCache stores successful plans only. Cold calls may compile the same type
// concurrently; user factories run outside the lock. The bounded active count
// catches recursive factory compilation without goroutine identity assumptions.
type planCache struct {
	mu      sync.Mutex
	entries map[planKey]*highLevelPlan
	order   []planKey
	active  map[planKey]int
}

func (c *planCache) getOrCompile(key planKey, compile func() (*highLevelPlan, error)) (*highLevelPlan, error) {
	c.mu.Lock()
	if plan := c.entries[key]; plan != nil {
		c.mu.Unlock()
		return plan, nil
	}
	if c.active == nil {
		c.active = make(map[planKey]int)
	}
	if c.active[key] >= 128 {
		c.mu.Unlock()
		return nil, &SchemaError{Type: key.typ, Reason: "recursive or excessive concurrent high-level schema compilation"}
	}
	c.active[key]++
	c.mu.Unlock()
	plan, err := safeCompilePlan(compile)
	c.mu.Lock()
	c.active[key]--
	if c.active[key] == 0 {
		delete(c.active, key)
	}
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if existing := c.entries[key]; existing != nil {
		c.mu.Unlock()
		return existing, nil
	}
	if c.entries == nil {
		c.entries = make(map[planKey]*highLevelPlan)
	}
	if len(c.order) >= 512 {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
	c.entries[key] = plan
	c.order = append(c.order, key)
	c.mu.Unlock()
	return plan, nil
}

func safeCompilePlan(compile func() (*highLevelPlan, error)) (plan *highLevelPlan, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			plan = nil
			err = fmt.Errorf("schema compilation panic: %v", recovered)
		}
	}()
	return compile()
}
