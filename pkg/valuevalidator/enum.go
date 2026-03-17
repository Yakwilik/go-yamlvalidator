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
		Path:     path,
		Line:     node.Line,
		Column:   node.Column,
		Message:  msg,
		Got:      node.Value,
		Expected: fmt.Sprintf("one of %v", vld.Allowed),
	})
}
