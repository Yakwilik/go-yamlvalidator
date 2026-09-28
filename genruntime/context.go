package genruntime

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Limits struct {
	MaxDepth      int
	MaxNodeVisits int
}

func normalizeLimits(limits Limits) Limits {
	if limits.MaxDepth <= 0 {
		limits.MaxDepth = 128
	}
	if limits.MaxNodeVisits <= 0 {
		limits.MaxNodeVisits = 1_000_000
	}
	return limits
}

type Pair struct {
	Key   string
	Value *yaml.Node
}

type DecodeContext struct {
	limits  Limits
	enforce bool
	visits  int
	depth   int
}

func NewDecodeContext(limits Limits, enforce bool) *DecodeContext {
	return &DecodeContext{limits: normalizeLimits(limits), enforce: enforce}
}

func (c *DecodeContext) Enter() error {
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

func (c *DecodeContext) Leave() {
	if c != nil {
		c.depth--
	}
}

func (c *DecodeContext) Resolve(node *yaml.Node) (*yaml.Node, error) {
	return ResolveNode(node)
}

func (c *DecodeContext) MappingPairs(node *yaml.Node) ([]Pair, error) {
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
	return c.expandMapping(node)
}

func (c *DecodeContext) bump() error {
	if !c.enforce {
		return nil
	}
	c.visits++
	if c.visits > c.limits.MaxNodeVisits {
		return fmt.Errorf("generated YAML decode exceeds visit limit")
	}
	return nil
}

func (c *DecodeContext) expandMapping(root *yaml.Node) ([]Pair, error) {
	active := make(map[*yaml.Node]bool)
	var expandValue func(*yaml.Node) ([]Pair, error)
	var expandMap func(*yaml.Node) ([]Pair, error)

	expandValue = func(value *yaml.Node) ([]Pair, error) {
		if value == nil {
			return nil, nil
		}
		if err := c.bump(); err != nil {
			return nil, err
		}
		switch value.Kind {
		case yaml.AliasNode:
			if value.Alias == nil {
				return nil, fmt.Errorf("invalid YAML alias")
			}
			return expandValue(value.Alias)
		case yaml.MappingNode:
			return expandMap(value)
		case yaml.SequenceNode:
			var out []Pair
			seen := map[string]bool{}
			for _, item := range value.Content {
				pairs, err := expandValue(item)
				if err != nil {
					return nil, err
				}
				for _, pair := range pairs {
					if err := c.bump(); err != nil {
						return nil, err
					}
					if !seen[pair.Key] {
						seen[pair.Key] = true
						out = append(out, pair)
					}
				}
			}
			return out, nil
		default:
			return nil, fmt.Errorf("invalid YAML merge value")
		}
	}

	expandMap = func(mapping *yaml.Node) ([]Pair, error) {
		if mapping == nil || mapping.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("expected YAML mapping")
		}
		if active[mapping] {
			return nil, fmt.Errorf("cyclic YAML merge")
		}
		active[mapping] = true
		defer delete(active, mapping)

		var merged, explicit []Pair
		mergedSeen := map[string]bool{}
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key, value := mapping.Content[i], mapping.Content[i+1]
			if err := c.bump(); err != nil {
				return nil, err
			}
			if key.Tag == "!!merge" {
				pairs, err := expandValue(value)
				if err != nil {
					return nil, err
				}
				for _, pair := range pairs {
					if err := c.bump(); err != nil {
						return nil, err
					}
					if !mergedSeen[pair.Key] {
						mergedSeen[pair.Key] = true
						merged = append(merged, pair)
					}
				}
				continue
			}
			explicit = append(explicit, Pair{Key: key.Value, Value: value})
		}
		explicit = dedupePairsKeepLast(explicit)
		explicitKeys := make(map[string]bool, len(explicit))
		for _, pair := range explicit {
			explicitKeys[pair.Key] = true
		}
		out := make([]Pair, 0, len(merged)+len(explicit))
		for _, pair := range merged {
			if !explicitKeys[pair.Key] {
				out = append(out, pair)
			}
		}
		return append(out, explicit...), nil
	}
	return expandMap(root)
}

func dedupePairsKeepLast(pairs []Pair) []Pair {
	last := make(map[string]int, len(pairs))
	for i, pair := range pairs {
		last[pair.Key] = i
	}
	out := make([]Pair, 0, len(last))
	for i, pair := range pairs {
		if last[pair.Key] == i {
			out = append(out, pair)
		}
	}
	return out
}
