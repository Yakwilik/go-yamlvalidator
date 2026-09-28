package yamlvalidator

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
)

func definitionRuleError(rules []tagRule, reason string, keys ...string) error {
	offset := 0
	for _, rule := range rules {
		for _, key := range keys {
			if rule.key == key {
				offset = rule.offset
			}
		}
	}
	return &tagOffsetError{Offset: offset, Reason: reason}
}

func validateHighLevelObjectReferences(schema *FieldSchema, rules []tagRule) error {
	if schema.AllowedKeys == nil {
		return nil
	}
	for _, rule := range rules {
		var names []string
		switch rule.key {
		case "require", "exactlyOneOf", "mutuallyExclusive":
			names, _ = stringList(rule.value)
		case "anyOfRequired", "oneOfRequired", "forbiddenTogether":
			for _, group := range rule.value.list {
				members, _ := stringList(group)
				names = append(names, members...)
			}
		case "dependentRequired":
			for _, entry := range rule.value.rules {
				names = append(names, entry.key)
				members, _ := stringList(entry.value)
				names = append(names, members...)
			}
		case "when":
			for _, entry := range rule.value.rules {
				switch entry.key {
				case "field":
					name, _ := tagScalar(entry.value)
					names = append(names, name)
				case "require", "forbid":
					members, _ := stringList(entry.value)
					names = append(names, members...)
				}
			}
		}
		for _, name := range names {
			if schema.AllowedKeys[name] == nil {
				return &tagOffsetError{Offset: rule.offset, Reason: fmt.Sprintf("unknown YAML field %q in %s", name, rule.key)}
			}
		}
	}
	return nil
}

func resolveHighLevelRequired(schema *FieldSchema) error {
	for _, name := range schema.pendingRequired {
		child := schema.AllowedKeys[name]
		if child == nil {
			return fmt.Errorf("unknown YAML field %q in require", name)
		}
		child.Required = true
	}
	schema.pendingRequired = nil
	return nil
}

func validateHighLevelFieldDefinition(schema *FieldSchema, typ reflect.Type, rules []tagRule) error {
	base := derefType(typ)
	for _, rule := range rules {
		if isGroupDirective(rule.key) && schema.Type != TypeMap && schema.Type != TypeAny {
			return &tagOffsetError{Offset: rule.offset, Reason: "object group requires mapping"}
		}
		switch rule.key {
		case "nonempty":
			if schema.Type != TypeString && schema.Type != TypeMap && schema.Type != TypeSequence && schema.Type != TypeAny {
				return &tagOffsetError{Offset: rule.offset, Reason: "nonempty requires string, map, or sequence"}
			}
		case "uniqueItems":
			if schema.Type != TypeSequence && schema.Type != TypeAny {
				return &tagOffsetError{Offset: rule.offset, Reason: "uniqueItems requires sequence"}
			}
		case "minItems", "maxItems":
			if schema.Type != TypeSequence && schema.Type != TypeAny {
				return &tagOffsetError{Offset: rule.offset, Reason: "item bound requires sequence"}
			}
		}
	}
	var minNum, maxNum *big.Rat
	var minLen, maxLen, minProps, maxProps *int
	for _, validator := range schema.Validators {
		switch v := validator.(type) {
		case rangeRule:
			if v.minimum {
				if minNum != nil {
					return definitionRuleError(rules, "duplicate minimum", "min")
				}
				minNum = v.bound
			} else {
				if maxNum != nil {
					return definitionRuleError(rules, "duplicate maximum", "max")
				}
				maxNum = v.bound
			}
		case lengthRule:
			n := v.bound
			if v.properties {
				if v.minimum {
					minProps = &n
				} else {
					maxProps = &n
				}
			} else {
				if v.minimum {
					minLen = &n
				} else {
					maxLen = &n
				}
			}
		}
	}
	if minNum != nil && maxNum != nil && minNum.Cmp(maxNum) > 0 {
		return definitionRuleError(rules, "minimum exceeds maximum", "min", "max")
	}
	if minLen != nil && maxLen != nil && *minLen > *maxLen {
		return definitionRuleError(rules, "minimum length exceeds maximum", "minLength", "maxLength")
	}
	if minProps != nil && maxProps != nil && *minProps > *maxProps {
		return definitionRuleError(rules, "minimum properties exceeds maximum", "minProperties", "maxProperties")
	}
	if schema.MinItems != nil && schema.MaxItems != nil && *schema.MinItems > *schema.MaxItems {
		return definitionRuleError(rules, "minItems exceeds maxItems", "minItems", "maxItems")
	}
	if minNum == nil && maxNum == nil {
		return nil
	}
	if base.Kind() >= reflect.Int && base.Kind() <= reflect.Uint64 {
		bits := base.Bits()
		low := new(big.Int)
		high := new(big.Int)
		if base.Kind() >= reflect.Uint {
			high.Sub(new(big.Int).Lsh(big.NewInt(1), uint(bits)), big.NewInt(1))
		} else {
			high.Sub(new(big.Int).Lsh(big.NewInt(1), uint(bits-1)), big.NewInt(1))
			low.Neg(new(big.Int).Lsh(big.NewInt(1), uint(bits-1)))
		}
		if minNum != nil && minNum.Cmp(new(big.Rat).SetInt(high)) > 0 || maxNum != nil && maxNum.Cmp(new(big.Rat).SetInt(low)) < 0 {
			key := "min"
			if maxNum != nil && maxNum.Cmp(new(big.Rat).SetInt(low)) < 0 {
				key = "max"
			}
			return definitionRuleError(rules, fmt.Sprintf("numeric bound cannot be satisfied by %s", base), key)
		}
	}
	if base.Kind() == reflect.Float32 || base.Kind() == reflect.Float64 {
		maxFloat := math.MaxFloat64
		if base.Kind() == reflect.Float32 {
			maxFloat = math.MaxFloat32
		}
		bound := new(big.Rat).SetFloat64(maxFloat)
		neg := new(big.Rat).Neg(bound)
		if minNum != nil && minNum.Cmp(bound) > 0 || maxNum != nil && maxNum.Cmp(neg) < 0 {
			key := "min"
			if maxNum != nil && maxNum.Cmp(neg) < 0 {
				key = "max"
			}
			return definitionRuleError(rules, fmt.Sprintf("numeric bound cannot be satisfied by %s", base), key)
		}
	}
	return nil
}
