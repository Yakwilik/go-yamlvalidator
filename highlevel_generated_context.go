package yamlvalidator

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// GeneratedDecodeContext carries one decode operation's limits through nested
// generated codecs. The outer validator enforces limits when enabled; unchecked
// SkipValidation decoding uses this context to bound node and merge work.
type GeneratedDecodeContext struct {
	limits  Limits
	enforce bool
	visits  int
	depth   int
}

func NewGeneratedDecodeContext(limits Limits, enforce bool) *GeneratedDecodeContext {
	normalized, err := normalizeLimits(limits)
	if err != nil {
		normalized, _ = normalizeLimits(Limits{})
	}
	return &GeneratedDecodeContext{limits: normalized, enforce: enforce}
}

func (c *GeneratedDecodeContext) Enter() error {
	if c == nil {
		return fmt.Errorf("nil generated decode context")
	}
	c.depth++
	if c.enforce {
		c.visits++
		if c.depth > c.limits.MaxDepth || c.visits > c.limits.MaxNodeVisits {
			return fmt.Errorf("generated YAML decode exceeds limits")
		}
	}
	return nil
}
func (c *GeneratedDecodeContext) Leave() {
	if c != nil {
		c.depth--
	}
}

func (c *GeneratedDecodeContext) Resolve(node *yaml.Node) (*yaml.Node, error) {
	resolved, err := GeneratedResolveNode(node)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (c *GeneratedDecodeContext) MappingPairs(node *yaml.Node) ([]GeneratedPair, error) {
	if c == nil {
		return nil, fmt.Errorf("nil generated decode context")
	}
	node, err := c.Resolve(node)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML mapping")
	}
	seen := make(map[string]bool)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Tag == "!!merge" {
			continue
		}
		if seen[key.Value] {
			return nil, fmt.Errorf("duplicate YAML key %q", key.Value)
		}
		seen[key.Value] = true
	}
	ctx := NewValidationContext()
	ctx.MaxDepth = c.limits.MaxDepth
	ctx.MaxNodeVisits = c.limits.MaxNodeVisits
	if c.enforce {
		ctx.MaxNodeVisits -= c.visits
		if ctx.MaxNodeVisits <= 0 {
			return nil, fmt.Errorf("generated YAML decode exceeds visit limit")
		}
	}
	pairs := expandMappingWithMergesBounded(node, ctx)
	if ctx.limitReached || ctx.IsStopped() {
		return nil, fmt.Errorf("YAML merge expansion exceeded limits")
	}
	if c.enforce {
		c.visits += ctx.nodeVisits
	}
	out := make([]GeneratedPair, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, GeneratedPair{Key: pair.key.Value, Value: pair.value})
	}
	return out, nil
}
