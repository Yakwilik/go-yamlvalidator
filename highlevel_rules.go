package yamlvalidator

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

func isGroupDirective(name string) bool {
	switch name {
	case "require", "exactlyOneOf", "mutuallyExclusive", "anyOfRequired", "oneOfRequired", "forbiddenTogether", "dependentRequired", "when":
		return true
	}
	return false
}

func distinctNames(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			return fmt.Errorf("duplicate YAML field %q in one declaration", name)
		}
		seen[name] = true
	}
	return nil
}

func tagScalar(v tagValue) (string, error) {
	if v.kind != tagAtom && v.kind != tagString {
		return "", fmt.Errorf("expected scalar")
	}
	return v.text, nil
}
func ruleScalar(r tagRule) (string, error) {
	if !r.hasValue {
		return "", fmt.Errorf("%s requires a value", r.key)
	}
	return tagScalar(r.value)
}
func ruleList(r tagRule) ([]tagValue, error) {
	if !r.hasValue || r.value.kind != tagList {
		return nil, fmt.Errorf("%s requires a list", r.key)
	}
	return r.value.list, nil
}
func stringList(v tagValue) ([]string, error) {
	if v.kind != tagList {
		return nil, fmt.Errorf("expected list")
	}
	out := make([]string, len(v.list))
	for i, item := range v.list {
		s, err := tagScalar(item)
		if err != nil {
			return nil, err
		}
		if s == "" {
			return nil, fmt.Errorf("empty list item")
		}
		out[i] = s
	}
	return out, nil
}
func objectRules(v tagValue) ([]tagRule, error) {
	if v.kind != tagObject {
		return nil, fmt.Errorf("expected object")
	}
	return v.rules, nil
}
func tagLiteral(v tagValue) any {
	switch v.kind {
	case tagString:
		return v.text
	case tagList:
		out := make([]any, len(v.list))
		for i, x := range v.list {
			out[i] = tagLiteral(x)
		}
		return out
	case tagObject:
		out := map[string]any{}
		for _, r := range v.rules {
			if r.hasValue {
				out[r.key] = tagLiteral(r.value)
			} else {
				out[r.key] = true
			}
		}
		return out
	default:
		s := v.text
		if s == "null" {
			return nil
		}
		if s == "true" {
			return true
		}
		if s == "false" {
			return false
		}
		if _, ok := newJSONNumber(s); ok {
			return json.Number(s)
		}
		return s
	}
}
func newJSONNumber(s string) (json.Number, bool) {
	var n json.Number
	err := json.Unmarshal([]byte(s), &n)
	return n, err == nil
}
func parseRuleType(s string) (NodeType, error) {
	switch s {
	case "any":
		return TypeAny, nil
	case "null":
		return TypeNull, nil
	case "string":
		return TypeString, nil
	case "int", "integer":
		return TypeInt, nil
	case "float":
		return TypeFloat, nil
	case "bool", "boolean":
		return TypeBool, nil
	case "map", "mapping":
		return TypeMap, nil
	case "sequence", "list":
		return TypeSequence, nil
	}
	return 0, fmt.Errorf("unknown type %q", s)
}

func (c *highLevelCompiler) applyRules(schema *FieldSchema, rules []tagRule, objectMeta bool) error {
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.key] && !isGroupDirective(r.key) {
			return &tagOffsetError{Offset: r.offset, Reason: fmt.Sprintf("duplicate directive %q", r.key)}
		}
		seen[r.key] = true
		if err := c.applyRule(schema, r, objectMeta); err != nil {
			var offset *tagOffsetError
			if errors.As(err, &offset) {
				return err
			}
			return &tagOffsetError{Offset: r.offset, Reason: fmt.Sprintf("%s: %v", r.key, err)}
		}
	}
	if seen["nullable"] && seen["notnull"] {
		return definitionRuleError(rules, "nullable conflicts with notnull", "nullable", "notnull")
	}
	if seen["nullable"] && seen["nonempty"] {
		return definitionRuleError(rules, "nullable conflicts with nonempty", "nullable", "nonempty")
	}
	return nil
}

func (c *highLevelCompiler) applyRule(s *FieldSchema, r tagRule, objectMeta bool) error {
	switch r.key {
	case "required", "nullable", "notnull", "nonempty", "uniqueItems":
		if r.hasValue {
			return fmt.Errorf("takes no argument")
		}
		switch r.key {
		case "required":
			s.Required = true
		case "nullable":
			s.Nullable = true
		case "notnull":
			s.Nullable = false
			s.Validators = append(s.Validators, notNullRule{})
		case "nonempty":
			s.Validators = append(s.Validators, nonemptyRule{})
		case "uniqueItems":
			s.Validators = append(s.Validators, uniqueItemsRule{})
		}
		return nil
	case "type":
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		typ, err := parseRuleType(text)
		if err != nil {
			return err
		}
		if typ != TypeAny && s.Type != TypeAny && typ != s.Type && !(s.Type == TypeFloat && typ == TypeInt) && !(typ == TypeNull && s.Nullable) {
			return fmt.Errorf("type %s is incompatible with inferred %s", typ, s.Type)
		}
		s.Type = typ
		if typ == TypeMap && s.AllowedKeys == nil && s.AdditionalProperties == nil && s.UnknownKeyPolicy == UnknownKeyInherit {
			s.AdditionalProperties = &FieldSchema{Type: TypeAny}
		}
		return nil
	case "types":
		list, err := ruleList(r)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return fmt.Errorf("empty type union")
		}
		for _, v := range list {
			text, err := tagScalar(v)
			if err != nil {
				return err
			}
			typ, err := parseRuleType(text)
			if err != nil {
				return err
			}
			s.AllowedTypes = append(s.AllowedTypes, typ)
		}
		return nil
	case "deprecated":
		if !r.hasValue {
			s.Deprecated = "true"
			return nil
		}
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		if text == "" {
			return fmt.Errorf("empty deprecation message")
		}
		s.Deprecated = text
		return nil
	case "description":
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		s.Description = text
		return nil
	case "default":
		if !r.hasValue {
			return fmt.Errorf("requires a value")
		}
		if err := validateTagLiteral(r.value); err != nil {
			return err
		}
		s.Default = tagLiteral(r.value)
		s.defaultPresent = true
		return nil
	case "minItems", "maxItems":
		n, err := parseTagInt(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires integer")
		}
		if n < 0 {
			return fmt.Errorf("negative item count")
		}
		if r.key == "minItems" {
			s.MinItems = &n
		} else {
			s.MaxItems = &n
		}
		return nil
	case "items":
		rules, err := objectRules(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires object")
		}
		if s.Type != TypeSequence && s.Type != TypeAny {
			return fmt.Errorf("requires sequence")
		}
		if s.ItemSchema == nil {
			s.ItemSchema = &FieldSchema{Type: TypeAny}
		}
		return c.applyNestedRules(s.ItemSchema, rules)
	case "properties":
		props, err := objectRules(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires object")
		}
		if s.Type != TypeMap && s.Type != TypeAny {
			return fmt.Errorf("requires mapping")
		}
		if s.AllowedKeys == nil {
			s.AllowedKeys = map[string]*FieldSchema{}
		}
		for _, entry := range props {
			if !entry.hasValue {
				return fmt.Errorf("property %q requires schema", entry.key)
			}
			nested, err := objectRules(entry.value)
			if err != nil {
				return err
			}
			if _, ok := s.AllowedKeys[entry.key]; ok {
				return fmt.Errorf("duplicate property %q", entry.key)
			}
			child := &FieldSchema{Type: TypeAny}
			if err := c.applyNestedRules(child, nested); err != nil {
				return err
			}
			s.AllowedKeys[entry.key] = child
		}
		return nil
	case "additional":
		if !r.hasValue {
			return fmt.Errorf("requires policy or object")
		}
		if s.Type != TypeMap && s.Type != TypeAny {
			return fmt.Errorf("requires mapping")
		}
		if r.value.kind == tagObject {
			child := &FieldSchema{Type: TypeAny}
			if err := c.applyNestedRules(child, r.value.rules); err != nil {
				return err
			}
			if s.AdditionalProperties != nil {
				s.AdditionalProperties.extraSchemas = append(s.AdditionalProperties.extraSchemas, child)
			} else {
				s.AdditionalProperties = child
			}
			return nil
		}
		text, err := tagScalar(r.value)
		if err != nil {
			return err
		}
		if s.AdditionalProperties != nil {
			if s.ValueSchema == nil {
				s.ValueSchema = s.AdditionalProperties
			} else {
				s.ValueSchema.extraSchemas = append(s.ValueSchema.extraSchemas, s.AdditionalProperties)
			}
		}
		s.AdditionalProperties = nil
		switch text {
		case "forbid":
			s.UnknownKeyPolicy = UnknownKeyError
		case "warn":
			s.UnknownKeyPolicy = UnknownKeyWarn
		case "ignore":
			s.UnknownKeyPolicy = UnknownKeyIgnore
		default:
			return fmt.Errorf("invalid additional policy %q", text)
		}
		return nil
	case "unknown":
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		switch text {
		case "error":
			s.UnknownKeyPolicy = UnknownKeyError
		case "warn":
			s.UnknownKeyPolicy = UnknownKeyWarn
		case "ignore":
			s.UnknownKeyPolicy = UnknownKeyIgnore
		case "inherit":
			s.UnknownKeyPolicy = UnknownKeyInherit
		default:
			return fmt.Errorf("unknown policy %q", text)
		}
		if text == "error" && s.AdditionalProperties != nil && s.AllowedKeys == nil {
			return fmt.Errorf("unknown=error cannot close open map")
		}
		return nil
	case "values":
		rules, err := objectRules(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires object")
		}
		if s.Type != TypeMap && s.Type != TypeAny {
			return fmt.Errorf("requires mapping")
		}
		if s.ValueSchema == nil {
			s.ValueSchema = &FieldSchema{Type: TypeAny}
		}
		if s.Type == TypeAny && s.AdditionalProperties == nil && s.UnknownKeyPolicy == UnknownKeyInherit {
			s.AdditionalProperties = &FieldSchema{Type: TypeAny}
		}
		return c.applyNestedRules(s.ValueSchema, rules)
	case "keys":
		rules, err := objectRules(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires object")
		}
		return c.applyKeyRules(s, rules)
	case "min", "max", "minLength", "maxLength", "minProperties", "maxProperties", "enum", "pattern", "url", "format":
		return c.applyValueRule(s, r)
	case "check", "checks", "ref", "oneOfSchemas", "anyOfSchemas":
		return c.applyExtensionRule(s, r)
	case "require", "anyOfRequired", "exactlyOneOf", "mutuallyExclusive", "oneOfRequired", "forbiddenTogether", "dependentRequired", "when":
		return c.applyGroupRule(s, r)
	default:
		return fmt.Errorf("unknown directive")
	}
}

func (c *highLevelCompiler) applyExtensionRule(s *FieldSchema, r tagRule) error {
	switch r.key {
	case "ref":
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		if c.registry == nil {
			return fmt.Errorf("ref %q requires registry", text)
		}
		child := c.registry.schemas[text]
		if child == nil {
			return fmt.Errorf("unknown schema %q", text)
		}
		s.extraSchemas = append(s.extraSchemas, child)
		return nil
	case "check", "checks":
		if c.registry == nil {
			return fmt.Errorf("check requires registry")
		}
		var values []tagValue
		if r.key == "check" {
			if !r.hasValue {
				return fmt.Errorf("check requires value")
			}
			values = []tagValue{r.value}
		} else {
			if !r.hasValue || r.value.kind != tagList {
				return fmt.Errorf("checks requires list")
			}
			values = r.value.list
		}
		for _, value := range values {
			v, err := c.valueCheck(value)
			if err != nil {
				return err
			}
			s.Validators = append(s.Validators, v)
		}
		return nil
	case "oneOfSchemas", "anyOfSchemas":
		items, err := ruleList(r)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return fmt.Errorf("empty schema alternatives")
		}
		for _, item := range items {
			var child *FieldSchema
			if item.kind == tagObject {
				child = &FieldSchema{Type: TypeAny}
				if err := c.applyNestedRules(child, item.rules); err != nil {
					return err
				}
			} else {
				name, err := tagScalar(item)
				if err != nil {
					return err
				}
				if c.registry == nil || c.registry.schemas[name] == nil {
					return fmt.Errorf("unknown schema %q", name)
				}
				child = c.registry.schemas[name]
			}
			if r.key == "oneOfSchemas" {
				s.OneOfSchemas = append(s.OneOfSchemas, child)
			} else {
				s.AnyOfSchemas = append(s.AnyOfSchemas, child)
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported extension")
}

func (c *highLevelCompiler) applyKeyRules(s *FieldSchema, rules []tagRule) error {
	if s.Type != TypeMap && s.Type != TypeAny {
		return fmt.Errorf("keys requires mapping")
	}
	seen := map[string]bool{}
	var minimum, maximum *int
	for _, r := range rules {
		if seen[r.key] {
			return &tagOffsetError{Offset: r.offset, Reason: fmt.Sprintf("duplicate key rule %q", r.key)}
		}
		seen[r.key] = true
		switch r.key {
		case "format":
			name, err := ruleScalar(r)
			if err != nil {
				return &tagOffsetError{Offset: r.offset, Reason: err.Error()}
			}
			format, err := nativeFormat(name)
			if err != nil {
				return &tagOffsetError{Offset: r.offset, Reason: err.Error()}
			}
			s.KeyValidators = append(s.KeyValidators, nativeKeyFormatRule{format})
		case "pattern":
			text, err := ruleScalar(r)
			if err != nil {
				return &tagOffsetError{Offset: r.offset, Reason: err.Error()}
			}
			re, err := regexp.Compile(text)
			if err != nil {
				return &tagOffsetError{Offset: r.offset, Reason: err.Error()}
			}
			s.KeyValidators = append(s.KeyValidators, keyPatternRule{re})
		case "minLength", "maxLength":
			n, err := parseTagInt(r.value)
			if !r.hasValue || err != nil || n < 0 {
				return &tagOffsetError{Offset: r.offset, Reason: "invalid key length"}
			}
			s.KeyValidators = append(s.KeyValidators, keyLengthRule{bound: n, minimum: r.key == "minLength"})
			if r.key == "minLength" {
				minimum = &n
			} else {
				maximum = &n
			}
		case "check":
			if !r.hasValue {
				return &tagOffsetError{Offset: r.offset, Reason: "key check requires value"}
			}
			validator, err := c.keyCheck(r.value)
			if err != nil {
				return &tagOffsetError{Offset: r.offset, Reason: err.Error()}
			}
			s.KeyValidators = append(s.KeyValidators, validator)
		default:
			return &tagOffsetError{Offset: r.offset, Reason: fmt.Sprintf("unknown key rule %q", r.key)}
		}
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return definitionRuleError(rules, "key minimum length exceeds maximum", "minLength", "maxLength")
	}
	return nil
}

func validateTagLiteral(value tagValue) error {
	switch value.kind {
	case tagList:
		for _, item := range value.list {
			if err := validateTagLiteral(item); err != nil {
				return err
			}
		}
	case tagObject:
		seen := map[string]bool{}
		for _, entry := range value.rules {
			if seen[entry.key] {
				return fmt.Errorf("duplicate object literal key %q", entry.key)
			}
			seen[entry.key] = true
			if entry.hasValue {
				if err := validateTagLiteral(entry.value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *highLevelCompiler) applyGroupRule(s *FieldSchema, r tagRule) error {
	if s.Type != TypeMap && s.Type != TypeAny {
		return fmt.Errorf("object group requires mapping")
	}
	if s.Type == TypeAny {
		if s.AdditionalProperties == nil && s.AllowedKeys == nil && s.UnknownKeyPolicy == UnknownKeyInherit {
			s.AdditionalProperties = &FieldSchema{Type: TypeAny}
		}
		found := false
		for _, validator := range s.Validators {
			if _, ok := validator.(mappingShapeRule); ok {
				found = true
				break
			}
		}
		if !found {
			s.Validators = append(s.Validators, mappingShapeRule{})
		}
	}
	switch r.key {
	case "require", "exactlyOneOf", "mutuallyExclusive":
		list, err := stringList(r.value)
		if !r.hasValue || err != nil {
			return fmt.Errorf("requires list")
		}
		if len(list) == 0 {
			return fmt.Errorf("empty group")
		}
		if err := distinctNames(list); err != nil {
			return err
		}
		switch r.key {
		case "require":
			for _, name := range list {
				if s.AllowedKeys == nil {
					found := false
					for _, existing := range s.requiredNames {
						if existing == name {
							found = true
							break
						}
					}
					if !found {
						s.requiredNames = append(s.requiredNames, name)
					}
					continue
				}
				child := s.AllowedKeys[name]
				if child == nil {
					s.pendingRequired = append(s.pendingRequired, name)
					continue
				}
				child.Required = true
			}
		case "exactlyOneOf":
			s.exactlyGroups = append(s.exactlyGroups, list)
		case "mutuallyExclusive":
			s.mutuallyGroups = append(s.mutuallyGroups, list)
		}
		return nil
	case "anyOfRequired", "oneOfRequired", "forbiddenTogether":
		if !r.hasValue || r.value.kind != tagList {
			return fmt.Errorf("requires nested list")
		}
		if len(r.value.list) == 0 {
			return fmt.Errorf("empty group")
		}
		var groups [][]string
		for _, item := range r.value.list {
			g, err := stringList(item)
			if err != nil || len(g) == 0 {
				return fmt.Errorf("invalid field group")
			}
			if err := distinctNames(g); err != nil {
				return err
			}
			groups = append(groups, g)
		}
		switch r.key {
		case "anyOfRequired":
			s.anyClauses = append(s.anyClauses, groups)
		case "oneOfRequired":
			s.oneClauses = append(s.oneClauses, groups)
		case "forbiddenTogether":
			s.ForbiddenTogether = append(s.ForbiddenTogether, groups...)
		}
		return nil
	case "dependentRequired":
		if !r.hasValue || r.value.kind != tagObject {
			return fmt.Errorf("requires object")
		}
		if s.DependentRequired == nil {
			s.DependentRequired = map[string][]string{}
		}
		seenTriggers := map[string]bool{}
		for _, entry := range r.value.rules {
			if seenTriggers[entry.key] {
				return fmt.Errorf("duplicate dependency %q", entry.key)
			}
			seenTriggers[entry.key] = true
			if !entry.hasValue {
				return fmt.Errorf("dependency requires list")
			}
			list, err := stringList(entry.value)
			if err != nil {
				return err
			}
			if err := distinctNames(list); err != nil {
				return err
			}
			for _, name := range list {
				found := false
				for _, existing := range s.DependentRequired[entry.key] {
					if existing == name {
						found = true
						break
					}
				}
				if !found {
					s.DependentRequired[entry.key] = append(s.DependentRequired[entry.key], name)
				}
			}
		}
		return nil
	case "when":
		if !r.hasValue || r.value.kind != tagObject {
			return fmt.Errorf("requires object")
		}
		cond := ConditionalRule{}
		seen := map[string]bool{}
		for _, entry := range r.value.rules {
			if seen[entry.key] {
				return fmt.Errorf("duplicate when key %q", entry.key)
			}
			seen[entry.key] = true
			switch entry.key {
			case "field":
				text, err := ruleScalar(entry)
				if err != nil {
					return err
				}
				cond.ConditionField = text
			case "eq":
				text, err := ruleScalar(entry)
				if err != nil {
					return err
				}
				cond.ConditionValue = text
			case "require":
				list, err := stringList(entry.value)
				if !entry.hasValue || err != nil {
					return fmt.Errorf("when.require requires list")
				}
				cond.ThenRequired = list
				if err := distinctNames(list); err != nil {
					return err
				}
			case "forbid":
				list, err := stringList(entry.value)
				if !entry.hasValue || err != nil {
					return fmt.Errorf("when.forbid requires list")
				}
				cond.ThenForbidden = list
				if err := distinctNames(list); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown when key %q", entry.key)
			}
		}
		if cond.ConditionField == "" || !seen["eq"] || len(cond.ThenRequired)+len(cond.ThenForbidden) == 0 {
			return fmt.Errorf("incomplete when rule")
		}
		s.Conditions = append(s.Conditions, cond)
		return nil
	}
	return fmt.Errorf("unsupported object group")
}
