package valuevalidator

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// OneOfTypeValidator validates that a node matches one of the allowed types.
type OneOfTypeValidator struct {
	Types []v.NodeType
}

// Validate implements ValueValidator.
func (vld OneOfTypeValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	validator := &v.Validator{}                       // reuse inferType
	actual := validator.InferTypeForPublic(node, ctx) // helper we will expose

	for _, t := range vld.Types {
		if actual == t || (t == v.TypeFloat && actual == v.TypeInt) {
			return
		}
	}

	var typeNames []string
	for _, t := range vld.Types {
		typeNames = append(typeNames, t.String())
	}

	ctx.AddError(v.ValidationError{
		Level:    v.LevelError,
		Path:     path,
		Line:     node.Line,
		Column:   node.Column,
		Message:  "type not allowed",
		Got:      actual.String(),
		Expected: fmt.Sprintf("one of %v", typeNames),
	})
}
