package valuevalidator

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExactNumber is an immutable finite number used by RangeValidator when a
// bound cannot be represented exactly as float64.
type ExactNumber struct {
	value *big.Rat
	text  string
}

// ParseExactNumber parses a decimal/scientific or YAML-style integer literal
// without float64 rounding.
func ParseExactNumber(text string) (ExactNumber, error) {
	rat, _, err := parseFiniteExactNumber(text)
	if err != nil {
		return ExactNumber{}, err
	}
	display := strings.ReplaceAll(strings.TrimSpace(text), "_", "")
	return ExactNumber{value: rat, text: display}, nil
}

// MustExactNumber is ParseExactNumber for static schema definitions.
func MustExactNumber(text string) *ExactNumber {
	number, err := ParseExactNumber(text)
	if err != nil {
		panic(err)
	}
	return &number
}

// String returns a stable representation of the configured number.
func (n ExactNumber) String() string {
	return n.text
}

type numericKind uint8

const (
	numericFinite numericKind = iota
	numericNegativeInfinity
	numericPositiveInfinity
	numericNaN
)

type exactNumericValue struct {
	kind    numericKind
	value   *big.Rat
	display string
}

func finiteNumeric(value *big.Rat, display string) exactNumericValue {
	return exactNumericValue{
		kind:    numericFinite,
		value:   new(big.Rat).Set(value),
		display: display,
	}
}

func exactNumberValue(number *ExactNumber) (exactNumericValue, error) {
	if number == nil || number.value == nil {
		return exactNumericValue{}, fmt.Errorf("exact number is not initialized")
	}
	return finiteNumeric(number.value, number.text), nil
}

func floatBound(value float64) (exactNumericValue, error) {
	switch {
	case math.IsNaN(value):
		return exactNumericValue{}, fmt.Errorf("bound must not be NaN")
	case math.IsInf(value, -1):
		return exactNumericValue{kind: numericNegativeInfinity, display: "-Inf"}, nil
	case math.IsInf(value, 1):
		return exactNumericValue{kind: numericPositiveInfinity, display: "+Inf"}, nil
	default:
		text := strconv.FormatFloat(value, 'g', -1, 64)
		rat, _, err := parseFiniteExactNumber(text)
		if err != nil {
			return exactNumericValue{}, fmt.Errorf("invalid float bound: %w", err)
		}
		return finiteNumeric(rat, text), nil
	}
}

func parseYAMLExactNumber(node *yaml.Node) (exactNumericValue, error) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return exactNumericValue{}, fmt.Errorf("not a numeric scalar")
	}

	raw := strings.TrimSpace(node.Value)
	lower := strings.ToLower(strings.ReplaceAll(raw, "_", ""))
	switch lower {
	case ".nan", "+.nan", "-.nan":
		return exactNumericValue{kind: numericNaN, display: raw}, nil
	case ".inf", "+.inf":
		return exactNumericValue{kind: numericPositiveInfinity, display: raw}, nil
	case "-.inf":
		return exactNumericValue{kind: numericNegativeInfinity, display: raw}, nil
	}

	rat, _, err := parseFiniteExactNumber(raw)
	if err != nil {
		return exactNumericValue{}, err
	}
	return finiteNumeric(rat, raw), nil
}

func parseFiniteExactNumber(raw string) (*big.Rat, string, error) {
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
		denominator := pow10(scale)
		rat.SetFrac(numerator, denominator)
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
	mantissa := text[:index]
	expText := text[index+1:]
	if mantissa == "" || expText == "" {
		return "", 0, fmt.Errorf("missing mantissa or exponent")
	}
	exponent64, err := strconv.ParseInt(expText, 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid exponent")
	}
	// Prevent absurd schema literals from causing giant allocations.
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

func compareNumeric(left, right exactNumericValue) (int, bool) {
	if left.kind == numericNaN || right.kind == numericNaN {
		return 0, false
	}
	if left.kind == right.kind {
		if left.kind == numericFinite {
			return left.value.Cmp(right.value), true
		}
		return 0, true
	}
	if left.kind == numericNegativeInfinity || right.kind == numericPositiveInfinity {
		return -1, true
	}
	if left.kind == numericPositiveInfinity || right.kind == numericNegativeInfinity {
		return 1, true
	}
	// Remaining cases are finite vs infinity.
	if right.kind == numericNegativeInfinity {
		return 1, true
	}
	return -1, true
}

func cloneExactNumber(number *ExactNumber) *ExactNumber {
	if number == nil {
		return nil
	}
	cloned := *number
	if number.value != nil {
		cloned.value = new(big.Rat).Set(number.value)
	}
	return &cloned
}
