package keyvalidator

import (
	"fmt"
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// RegexKeyValidator validates that key names match a pattern.
type RegexKeyValidator struct {
	Pattern *regexp.Regexp
	Message string // Custom error message (optional)
}

func (vld RegexKeyValidator) ValidateDefinition() error {
	if vld.Pattern == nil {
		return fmt.Errorf("regex pattern must not be nil")
	}
	return nil
}

// ValidateKey implements KeyValidator.
func (vld RegexKeyValidator) ValidateKey(key string, keyNode *yaml.Node, path string, ctx *v.ValidationContext) {
	if vld.Pattern.MatchString(key) {
		return
	}
	msg := vld.Message
	if msg == "" {
		msg = fmt.Sprintf("key does not match pattern %s", vld.Pattern.String())
	}
	ctx.AddError(v.ValidationError{
		Level:   v.LevelError,
		Code:    "key_pattern",
		Path:    path,
		Line:    keyNode.Line,
		Column:  keyNode.Column,
		Message: msg,
		Got:     key,
	})
}
