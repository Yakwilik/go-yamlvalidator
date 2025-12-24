package valuevalidator

import (
	"fmt"
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// RegexValidator validates that a string matches a pattern.
type RegexValidator struct {
	Pattern *regexp.Regexp
	Message string // Custom error message (optional)
}

// Validate implements ValueValidator.
func (vld RegexValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	if vld.Pattern.MatchString(node.Value) {
		return
	}
	msg := vld.Message
	if msg == "" {
		msg = fmt.Sprintf("value does not match pattern %s", vld.Pattern.String())
	}
	ctx.AddError(v.ValidationError{
		Level:   v.LevelError,
		Path:    path,
		Line:    node.Line,
		Column:  node.Column,
		Message: msg,
		Got:     node.Value,
	})
}
