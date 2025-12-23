package valuevalidator

import (
	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// NonEmptyValidator validates that a value is not empty.
type NonEmptyValidator struct{}

// Validate implements ValueValidator.
func (NonEmptyValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	isEmpty := false
	switch node.Kind {
	case yaml.ScalarNode:
		isEmpty = node.Value == ""
	case yaml.SequenceNode, yaml.MappingNode:
		isEmpty = len(node.Content) == 0
	}

	if isEmpty {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "value cannot be empty",
		})
	}
}
