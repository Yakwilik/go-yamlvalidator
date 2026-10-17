package yamlcodec

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
	Key     string
	KeyNode *yaml.Node
	Value   *yaml.Node
}

type DecodeContext struct {
	FieldNaming FieldNaming
	limits      Limits
	enforce     bool
	visits      int
	depth       int
	activeMixed map[*yaml.Node]bool
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

// EnterMixedNode rejects cycles on the active reflective composite path. A
// later use of the same alias is allowed after the returned function runs.
// This guard is independent of depth accounting and pointer dereferences.
func (c *DecodeContext) EnterMixedNode(node *yaml.Node) (func(), error) {
	if c == nil || node == nil {
		return nil, fmt.Errorf("nil YAML mixed decode context or node")
	}
	if c.activeMixed[node] {
		return nil, fmt.Errorf("cyclic YAML value")
	}
	if c.activeMixed == nil {
		c.activeMixed = make(map[*yaml.Node]bool)
	}
	c.activeMixed[node] = true
	return func() { delete(c.activeMixed, node) }, nil
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
	return c.expandMapping(node, nil)
}

// MappingPairsWithKeyDecoder preserves custom map-key codecs during merge expansion.
func (c *DecodeContext) MappingPairsWithKeyDecoder(node *yaml.Node, decodeKey func(*yaml.Node) (string, error)) ([]Pair, error) {
	if c == nil {
		return nil, fmt.Errorf("nil generated decode context")
	}
	resolved, err := c.Resolve(node)
	if err != nil {
		return nil, err
	}
	return c.expandMapping(resolved, decodeKey)
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

func (c *DecodeContext) expandMapping(root *yaml.Node, decodeKey func(*yaml.Node) (string, error)) ([]Pair, error) {
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
		var err error
		value, err = c.Resolve(value)
		if err != nil {
			return nil, err
		}
		switch value.Kind {
		case yaml.MappingNode:
			return expandMap(value)
		case yaml.SequenceNode:
			if active[value] {
				return nil, fmt.Errorf("cyclic YAML merge")
			}
			active[value] = true
			defer delete(active, value)
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
		// yaml.v3 compares the original node kind and spelling for duplicate
		// keys. Run this for every source mapping before merge precedence can
		// hide a malformed entry, including aliased and overridden sources.
		type keyIdentity struct {
			kind  yaml.Kind
			value string
		}
		explicitSeen := map[keyIdentity]bool{}
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key, value := mapping.Content[i], mapping.Content[i+1]
			if err := c.bump(); err != nil {
				return nil, err
			}
			if key == nil {
				return nil, fmt.Errorf("nil YAML mapping key")
			}
			identity := keyIdentity{kind: key.Kind, value: key.Value}
			if explicitSeen[identity] {
				return nil, fmt.Errorf("duplicate YAML key %q", key.Value)
			}
			explicitSeen[identity] = true
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
			var name string
			if decodeKey != nil {
				decoded, err := decodeKey(key)
				if err != nil {
					return nil, fmt.Errorf("decode YAML mapping key: %w", err)
				}
				name = decoded
			} else {
				resolvedKey, err := c.Resolve(key)
				if err != nil {
					return nil, err
				}
				if resolvedKey.Kind != yaml.ScalarNode {
					return nil, fmt.Errorf("YAML mapping key must be a scalar")
				}
				if err := resolvedKey.Decode(&name); err != nil {
					return nil, fmt.Errorf("decode YAML mapping key: %w", err)
				}
			}
			explicit = append(explicit, Pair{Key: name, KeyNode: key, Value: value})
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
