package yamlvalidator

import "gopkg.in/yaml.v3"

type skipNullValueValidator struct{ inner ValueValidator }

func (v skipNullValueValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	v.inner.Validate(node, path, ctx)
}
