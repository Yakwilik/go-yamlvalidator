package yamlvalidator

import "gopkg.in/yaml.v3"

type notNullRule struct{}

func (notNullRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		addHighLevelError(ctx, node, path, "notnull", "value must not be null")
	}
}
