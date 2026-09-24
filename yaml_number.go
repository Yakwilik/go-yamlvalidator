package yamlvalidator

import (
	"fmt"
	"math/big"
	"strings"
)

func normalizeYAMLInteger(value string) (string, error) {
	s := strings.ReplaceAll(value, "_", "")
	if s == "" {
		return "", fmt.Errorf("invalid YAML integer")
	}

	sign := ""
	if s[0] == '+' || s[0] == '-' {
		sign = s[:1]
		s = s[1:]
	}
	if s == "" {
		return "", fmt.Errorf("invalid YAML integer")
	}

	base := 10
	digits := s
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		base, digits = 16, s[2:]
	case strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O"):
		base, digits = 8, s[2:]
	case strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B"):
		base, digits = 2, s[2:]
	case len(s) > 1 && s[0] == '0' && isOctalDigits(s[1:]):
		base, digits = 8, s[1:]
	}
	if digits == "" {
		return "", fmt.Errorf("invalid YAML integer")
	}

	integer, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return "", fmt.Errorf("invalid YAML integer %q", value)
	}
	if sign == "-" {
		integer.Neg(integer)
	}
	return integer.String(), nil
}

func isOctalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '7' {
			return false
		}
	}
	return true
}

func yamlScalarLooksInteger(value string) bool {
	_, err := normalizeYAMLInteger(value)
	return err == nil
}
