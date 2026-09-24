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

func (vld LengthValidator) ValidateDefinition() error {
	if vld.Min != nil && *vld.Min < 0 {
		return fmt.Errorf("minimum length must be non-negative")
	}
	if vld.Max != nil && *vld.Max < 0 {
		return fmt.Errorf("maximum length must be non-negative")
	}
	if vld.Min != nil && vld.Max != nil && *vld.Min > *vld.Max {
		return fmt.Errorf("minimum length must not exceed maximum length")
	}
	return nil
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
			Code:     "min_length",
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
			Code:     "max_length",
			Path:     path,
			Line:     node.Line,
			Column:   node.Column,
			Message:  "length above maximum",
			Got:      fmt.Sprintf("%d", length),
			Expected: fmt.Sprintf("<= %d", *vld.Max),
		})
	}
}
