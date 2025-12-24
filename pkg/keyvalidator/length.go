package keyvalidator

import (
	"fmt"
	"unicode/utf8"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// LengthKeyValidator validates key name length.
type LengthKeyValidator struct {
	Min *int
	Max *int
}

// ValidateKey implements KeyValidator.
func (vld LengthKeyValidator) ValidateKey(key string, keyNode *yaml.Node, path string, ctx *v.ValidationContext) {
	length := utf8.RuneCountInString(key)

	if vld.Min != nil && length < *vld.Min {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     keyNode.Line,
			Column:   keyNode.Column,
			Message:  "key too short",
			Got:      fmt.Sprintf("%d characters", length),
			Expected: fmt.Sprintf(">= %d characters", *vld.Min),
		})
	}

	if vld.Max != nil && length > *vld.Max {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     keyNode.Line,
			Column:   keyNode.Column,
			Message:  "key too long",
			Got:      fmt.Sprintf("%d characters", length),
			Expected: fmt.Sprintf("<= %d characters", *vld.Max),
		})
	}
}
