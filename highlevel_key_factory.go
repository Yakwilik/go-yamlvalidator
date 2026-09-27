package yamlvalidator

import (
	"fmt"
)

func (c *highLevelCompiler) keyCheck(value tagValue) (KeyValidator, error) {
	if c.registry == nil {
		return nil, fmt.Errorf("key check requires registry")
	}
	if value.kind != tagObject {
		name, err := tagScalar(value)
		if err != nil {
			return nil, err
		}
		validator := c.registry.keys[name]
		if validator == nil {
			return nil, fmt.Errorf("unknown key validator %q", name)
		}
		return validator, nil
	}
	name, args, err := parseFactoryDeclaration(value)
	if err != nil {
		return nil, err
	}
	factory := c.registry.keyFactories[name]
	if factory == nil {
		return nil, fmt.Errorf("unknown key factory %q", name)
	}
	validator, err := factory(args)
	if err != nil {
		return nil, err
	}
	if nilInterface(validator) {
		return nil, fmt.Errorf("key factory %q returned nil", name)
	}
	if d, ok := validator.(DefinitionValidator); ok {
		if err := d.ValidateDefinition(); err != nil {
			return nil, fmt.Errorf("key factory %q: %w", name, err)
		}
	}
	return validator, nil
}
