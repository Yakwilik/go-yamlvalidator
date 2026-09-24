package valuevalidator

import (
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func (validator EnumValidator) CloneValueValidator() v.ValueValidator {
	validator.Allowed = append([]string(nil), validator.Allowed...)
	return validator
}

func (validator RegexValidator) CloneValueValidator() v.ValueValidator {
	if validator.Pattern != nil {
		validator.Pattern = regexp.MustCompile(validator.Pattern.String())
	}
	return validator
}

func (validator RangeValidator) CloneValueValidator() v.ValueValidator {
	if validator.Min != nil {
		value := *validator.Min
		validator.Min = &value
	}
	if validator.Max != nil {
		value := *validator.Max
		validator.Max = &value
	}
	validator.MinExact = cloneExactNumber(validator.MinExact)
	validator.MaxExact = cloneExactNumber(validator.MaxExact)
	return validator
}

func (validator LengthValidator) CloneValueValidator() v.ValueValidator {
	if validator.Min != nil {
		value := *validator.Min
		validator.Min = &value
	}
	if validator.Max != nil {
		value := *validator.Max
		validator.Max = &value
	}
	return validator
}

func (validator URLValidator) CloneValueValidator() v.ValueValidator {
	validator.AllowedSchemes = append([]string(nil), validator.AllowedSchemes...)
	return validator
}

func (validator OneOfTypeValidator) CloneValueValidator() v.ValueValidator {
	validator.Types = append([]v.NodeType(nil), validator.Types...)
	return validator
}

func (validator NonEmptyValidator) CloneValueValidator() v.ValueValidator {
	return validator
}
