package valuevalidator

import (
	"fmt"
	"unicode/utf8"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// LengthValidator validates the length of a string, sequence, or map.
type LengthValidator struct {
	Min *int // Minimum length (nil = no minimum)
	Max *int // Maximum length (nil = no maximum)
}

// Validate implements ValueValidator.
func (vld LengthValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	var length int
	switch node.Kind {
	case yaml.ScalarNode:
		length = utf8.RuneCountInString(node.Value)
	case yaml.SequenceNode:
		length = len(node.Content)
	case yaml.MappingNode:
		length = len(node.Content) / 2
	}

	if vld.Min != nil && length < *vld.Min {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     node.Line,
			Column:   node.Column,
			Message:  "length below minimum",
			Got:      fmt.Sprintf("%d", length),
			Expected: fmt.Sprintf(">= %d", *vld.Min),
		})
	}

	if vld.Max != nil && length > *vld.Max {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     node.Line,
			Column:   node.Column,
			Message:  "length above maximum",
			Got:      fmt.Sprintf("%d", length),
			Expected: fmt.Sprintf("<= %d", *vld.Max),
		})
	}
}
