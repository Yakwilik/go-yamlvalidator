package taglang

import "fmt"

// Scope identifies where a declaration occurs. Parent directives are only
// meaningful on a serializable struct field, which supplies the containing map.
type Scope uint8

const (
	ValueScope Scope = iota
	FieldScope
)

var groupNames = map[string]bool{
	"exactlyOneOf": true, "mutuallyExclusive": true,
	"anyOfRequired": true, "oneOfRequired": true,
	"forbiddenTogether": true, "dependentRequired": true,
	"when": true, "require": true,
}

// Normalize gives Keys directives the current value scope and bare directives
// the containing mapping scope. A future source compiler uses this same rule.
func Normalize(rules []Rule, scope Scope) (value, parent []Rule, err error) {
	for _, rule := range rules {
		name := rule.Key
		if len(name) > 4 && name[len(name)-4:] == "Keys" && groupNames[name[:len(name)-4]] {
			rule.Key = name[:len(name)-4]
			value = append(value, rule)
			continue
		}
		if groupNames[name] {
			if scope != FieldScope {
				return nil, nil, &OffsetError{Offset: rule.Offset, Reason: fmt.Sprintf("%s requires a parent field binding; use %sKeys for the current mapping", name, name)}
			}
			parent = append(parent, rule)
			continue
		}
		value = append(value, rule)
	}
	return value, parent, nil
}
