package yamlvalidator

import "fmt"

func parseFactoryDeclaration(value tagValue) (string, map[string]any, error) {
	if value.kind != tagObject {
		return "", nil, fmt.Errorf("factory declaration requires object")
	}
	var name string
	args := map[string]any{}
	seen := map[string]bool{}
	for _, rule := range value.rules {
		if seen[rule.key] {
			return "", nil, fmt.Errorf("duplicate factory option %q", rule.key)
		}
		seen[rule.key] = true
		switch rule.key {
		case "name":
			text, err := ruleScalar(rule)
			if err != nil {
				return "", nil, err
			}
			if text == "" {
				return "", nil, fmt.Errorf("factory name is empty")
			}
			name = text
		case "args":
			if !rule.hasValue || rule.value.kind != tagObject {
				return "", nil, fmt.Errorf("factory args require object")
			}
			if err := validateFactoryArgs(rule.value); err != nil {
				return "", nil, err
			}
			args = tagLiteral(rule.value).(map[string]any)
		default:
			return "", nil, fmt.Errorf("unknown factory option %q", rule.key)
		}
	}
	if name == "" {
		return "", nil, fmt.Errorf("factory name is required")
	}
	return name, args, nil
}

func validateFactoryArgs(value tagValue) error {
	if value.kind == tagObject {
		seen := map[string]bool{}
		for _, entry := range value.rules {
			if seen[entry.key] {
				return fmt.Errorf("duplicate factory argument %q", entry.key)
			}
			seen[entry.key] = true
			if !entry.hasValue {
				return fmt.Errorf("factory argument %q requires value", entry.key)
			}
			if err := validateFactoryArgs(entry.value); err != nil {
				return err
			}
		}
	}
	if value.kind == tagList {
		for _, item := range value.list {
			if err := validateFactoryArgs(item); err != nil {
				return err
			}
		}
	}
	return nil
}
