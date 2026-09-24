package valuevalidator

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// EnumValidator validates that a value is one of the allowed values.
type EnumValidator struct {
	Allowed []string
	Message string // Custom error message (optional)
}

func (vld EnumValidator) ValidateDefinition() error {
	if len(vld.Allowed) == 0 {
		return fmt.Errorf("enum must contain at least one allowed value")
	}
	seen := make(map[string]bool, len(vld.Allowed))
	for _, allowed := range vld.Allowed {
		if seen[allowed] {
			return fmt.Errorf("duplicate enum value %q", allowed)
		}
		seen[allowed] = true
	}
	return nil
}

// Validate implements ValueValidator.
func (vld EnumValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	for _, allowed := range vld.Allowed {
		if node.Value == allowed {
			return
		}
	}
	msg := vld.Message
	if msg == "" {
		msg = fmt.Sprintf("invalid value %q", node.Value)
	}
	ctx.AddError(v.ValidationError{
		Level:    v.LevelError,
		Code:     "enum",
		Path:     path,
		Line:     node.Line,
		Column:   node.Column,
		Message:  msg,
		Got:      node.Value,
		Expected: fmt.Sprintf("one of %v", vld.Allowed),
	})
}
