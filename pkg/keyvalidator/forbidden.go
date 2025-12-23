package keyvalidator

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// ForbiddenKeyValidator validates that certain key names are not used.
type ForbiddenKeyValidator struct {
	Forbidden []string
	Message   string // Custom error message (optional)
}

// ValidateKey implements KeyValidator.
func (vld ForbiddenKeyValidator) ValidateKey(key string, keyNode *yaml.Node, path string, ctx *v.ValidationContext) {
	for _, forbidden := range vld.Forbidden {
		if key == forbidden {
			msg := vld.Message
			if msg == "" {
				msg = fmt.Sprintf("key %q is forbidden", key)
			}
			ctx.AddError(v.ValidationError{
				Level:   v.LevelError,
				Path:    path,
				Line:    keyNode.Line,
				Column:  keyNode.Column,
				Message: msg,
				Got:     key,
			})
			return
		}
	}
}
