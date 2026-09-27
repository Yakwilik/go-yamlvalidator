// Package checks contains calculation primitives shared by the native
// validators and the high-level tag adapters. It has no dependency on the
// public yamlvalidator packages.
package checks

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// ParseFiniteNumber parses decimal/scientific and YAML-style integer literals
// without converting through float64. The second result is normalized display
// text with underscores removed.
func ParseFiniteNumber(raw string) (*big.Rat, string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "_", "")
	text := normalized
	if text == "" {
		return nil, "", fmt.Errorf("empty numeric literal")
	}
	sign := 1
	if text[0] == '+' || text[0] == '-' {
		if text[0] == '-' {
			sign = -1
		}
		text = text[1:]
	}
	if text == "" {
		return nil, "", fmt.Errorf("invalid numeric literal %q", raw)
	}
	if integer, ok, err := parseBasedInteger(text); ok {
		if err != nil {
			return nil, "", err
		}
		if sign < 0 {
			integer.Neg(integer)
		}
		return new(big.Rat).SetInt(integer), normalized, nil
	}
	mantissa, exponent, err := splitDecimalExponent(text)
	if err != nil {
		return nil, "", fmt.Errorf("invalid numeric literal %q: %w", raw, err)
	}
	whole, fraction, err := splitDecimalMantissa(mantissa)
	if err != nil {
		return nil, "", fmt.Errorf("invalid numeric literal %q: %w", raw, err)
	}
	digits := whole + fraction
	if digits == "" || !allDecimalDigits(digits) {
		return nil, "", fmt.Errorf("invalid numeric literal %q", raw)
	}
	numerator, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, "", fmt.Errorf("invalid numeric literal %q", raw)
	}
	if sign < 0 {
		numerator.Neg(numerator)
	}
	scale := len(fraction) - exponent
	rat := new(big.Rat)
	if scale > 0 {
		rat.SetFrac(numerator, pow10(scale))
	} else if scale < 0 {
		numerator.Mul(numerator, pow10(-scale))
		rat.SetInt(numerator)
	} else {
		rat.SetInt(numerator)
	}
	return rat, normalized, nil
}

func parseBasedInteger(text string) (*big.Int, bool, error) {
	base, digits, ok := 0, "", false
	switch {
	case len(text) > 2 && (strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X")):
		base, digits, ok = 16, text[2:], true
	case len(text) > 2 && (strings.HasPrefix(text, "0o") || strings.HasPrefix(text, "0O")):
		base, digits, ok = 8, text[2:], true
	case len(text) > 2 && (strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B")):
		base, digits, ok = 2, text[2:], true
	case len(text) > 1 && text[0] == '0' && allOctalDigits(text[1:]):
		base, digits, ok = 8, text[1:], true
	}
	if !ok {
		return nil, false, nil
	}
	integer, parsed := new(big.Int).SetString(digits, base)
	if !parsed {
		return nil, true, fmt.Errorf("invalid base-%d integer %q", base, text)
	}
	return integer, true, nil
}
func splitDecimalExponent(text string) (string, int, error) {
	index := strings.IndexAny(text, "eE")
	if index < 0 {
		return text, 0, nil
	}
	if strings.ContainsAny(text[index+1:], "eE") {
		return "", 0, fmt.Errorf("multiple exponents")
	}
	mantissa, expText := text[:index], text[index+1:]
	if mantissa == "" || expText == "" {
		return "", 0, fmt.Errorf("missing mantissa or exponent")
	}
	exponent64, err := strconv.ParseInt(expText, 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid exponent")
	}
	if exponent64 > 100000 || exponent64 < -100000 {
		return "", 0, fmt.Errorf("exponent magnitude exceeds 100000")
	}
	return mantissa, int(exponent64), nil
}
func splitDecimalMantissa(text string) (string, string, error) {
	if strings.Count(text, ".") > 1 {
		return "", "", fmt.Errorf("multiple decimal points")
	}
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		whole, fraction := text[:dot], text[dot+1:]
		if whole == "" && fraction == "" {
			return "", "", fmt.Errorf("missing digits")
		}
		if whole != "" && !allDecimalDigits(whole) {
			return "", "", fmt.Errorf("invalid integer part")
		}
		if fraction != "" && !allDecimalDigits(fraction) {
			return "", "", fmt.Errorf("invalid fractional part")
		}
		return whole, fraction, nil
	}
	if !allDecimalDigits(text) {
		return "", "", fmt.Errorf("invalid digits")
	}
	return text, "", nil
}
func allDecimalDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
func allOctalDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '7' {
			return false
		}
	}
	return true
}
func pow10(power int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil)
}
