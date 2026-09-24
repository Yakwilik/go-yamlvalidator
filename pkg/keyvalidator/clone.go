package keyvalidator

import (
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func (validator RegexKeyValidator) CloneKeyValidator() v.KeyValidator {
	if validator.Pattern != nil {
		validator.Pattern = regexp.MustCompile(validator.Pattern.String())
	}
	return validator
}

func (validator ForbiddenKeyValidator) CloneKeyValidator() v.KeyValidator {
	validator.Forbidden = append([]string(nil), validator.Forbidden...)
	return validator
}

func (validator LengthKeyValidator) CloneKeyValidator() v.KeyValidator {
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
