package yamlvalidator

import "gopkg.in/yaml.v3"

// expandMappingWithMergesBounded expands only as much work as the run budget
// permits. A nil result with ctx.limitReached set is an incomplete validation.
func expandMappingWithMergesBounded(node *yaml.Node, ctx *ValidationContext) []kvPair {
	if ctx == nil || ctx.MaxNodeVisits == 0 {
		return expandMappingWithMerges(node)
	}
	active := make(map[*yaml.Node]bool)
	var expandMap func(*yaml.Node) []kvPair
	var expandValue func(*yaml.Node) []kvPair
	expandValue = func(value *yaml.Node) []kvPair {
		if value == nil || ctx.IsStopped() {
			return nil
		}
		if !ctx.visitNode(value, "") {
			return nil
		}
		switch value.Kind {
		case yaml.AliasNode:
			return expandValue(value.Alias)
		case yaml.MappingNode:
			return expandMap(value)
		case yaml.SequenceNode:
			out := []kvPair{}
			seen := map[string]bool{}
			for _, item := range value.Content {
				for _, pair := range expandValue(item) {
					if !ctx.visitNode(pair.key, "") {
						return nil
					}
					if !seen[pair.key.Value] {
						seen[pair.key.Value] = true
						out = append(out, pair)
					}
				}
				if ctx.IsStopped() {
					return nil
				}
			}
			return out
		}
		return nil
	}
	expandMap = func(mapping *yaml.Node) []kvPair {
		if mapping == nil || mapping.Kind != yaml.MappingNode || ctx.IsStopped() {
			return nil
		}
		if active[mapping] {
			ctx.limitReached = true
			ctx.stopped = true
			return nil
		}
		active[mapping] = true
		defer delete(active, mapping)
		merged := []kvPair{}
		mergedSeen := map[string]bool{}
		explicit := []kvPair{}
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key, value := mapping.Content[i], mapping.Content[i+1]
			if !ctx.visitNode(key, "") {
				return nil
			}
			if key.Tag == "!!merge" {
				for _, pair := range expandValue(value) {
					if !ctx.visitNode(pair.key, "") {
						return nil
					}
					if !mergedSeen[pair.key.Value] {
						mergedSeen[pair.key.Value] = true
						merged = append(merged, pair)
					}
				}
			} else {
				explicit = append(explicit, kvPair{key: key, value: value})
			}
			if ctx.IsStopped() {
				return nil
			}
		}
		explicit = dedupePairsKeepLast(explicit)
		explicitKeys := make(map[string]bool, len(explicit))
		for _, pair := range explicit {
			explicitKeys[pair.key.Value] = true
		}
		out := make([]kvPair, 0, len(merged)+len(explicit))
		for _, pair := range merged {
			if !explicitKeys[pair.key.Value] {
				out = append(out, pair)
			}
		}
		return append(out, explicit...)
	}
	return expandMap(node)
}

func validMergeValueBounded(node *yaml.Node, ctx *ValidationContext, visiting map[*yaml.Node]bool) bool {
	if node == nil || ctx.IsStopped() || !ctx.visitNode(node, "") {
		return false
	}
	if visiting[node] {
		return false
	}
	visiting[node] = true
	defer delete(visiting, node)
	switch node.Kind {
	case yaml.AliasNode:
		return node.Alias != nil && validMergeValueBounded(node.Alias, ctx, visiting)
	case yaml.MappingNode:
		return true
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if !validMergeValueBounded(item, ctx, visiting) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
