package yamlvalidator

import (
	"fmt"
	"regexp"
	"strings"
)

func (c *highLevelCompiler) applyValueRule(s *FieldSchema, r tagRule) error {
	switch r.key {
	case "min", "max":
		if !r.hasValue {
			return fmt.Errorf("numeric bound requires a value")
		}
		if s.Type != TypeInt && s.Type != TypeFloat && s.Type != TypeAny {
			return fmt.Errorf("numeric bound requires numeric type")
		}
		bound, err := parseExactBound(r.value)
		if err != nil {
			return err
		}
		s.Validators = append(s.Validators, rangeRule{bound: bound, minimum: r.key == "min"})
	case "minLength", "maxLength", "minProperties", "maxProperties":
		if !r.hasValue {
			return fmt.Errorf("length bound requires a value")
		}
		n, err := parseTagInt(r.value)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid length bound")
		}
		properties := strings.HasSuffix(r.key, "Properties")
		if properties && s.Type != TypeMap && s.Type != TypeAny {
			return fmt.Errorf("property bound requires mapping")
		}
		if !properties && s.Type != TypeString && s.Type != TypeAny {
			return fmt.Errorf("length bound requires string")
		}
		s.Validators = append(s.Validators, lengthRule{bound: n, minimum: strings.HasPrefix(r.key, "min"), properties: properties})
	case "enum":
		if !r.hasValue {
			return fmt.Errorf("enum requires list")
		}
		if r.value.kind != tagList || len(r.value.list) == 0 {
			return fmt.Errorf("enum requires nonempty scalar list")
		}
		values := make([]string, len(r.value.list))
		for i, item := range r.value.list {
			text, err := tagScalar(item)
			if err != nil {
				return fmt.Errorf("enum requires scalar members")
			}
			values[i] = text
		}
		s.Validators = append(s.Validators, enumRule(values))
	case "pattern":
		if s.Type != TypeString && s.Type != TypeAny {
			return fmt.Errorf("pattern requires string")
		}
		text, err := ruleScalar(r)
		if err != nil {
			return err
		}
		re, err := regexp.Compile(text)
		if err != nil {
			return err
		}
		s.Validators = append(s.Validators, patternRule{re: re})
	case "url":
		if s.Type != TypeString && s.Type != TypeAny {
			return fmt.Errorf("url requires string")
		}
		v, err := parseURLRule(r.value, r.hasValue)
		if err != nil {
			return err
		}
		s.Validators = append(s.Validators, v)
	case "format":
		if s.Type != TypeString && s.Type != TypeAny {
			return fmt.Errorf("format requires string")
		}
		format, err := ruleScalar(r)
		if err != nil {
			return err
		}
		validator, err := nativeFormat(format)
		if err != nil {
			return err
		}
		s.Validators = append(s.Validators, validator)
	default:
		return fmt.Errorf("unknown value directive %q", r.key)
	}
	return nil
}
