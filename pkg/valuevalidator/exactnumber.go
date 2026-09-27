package valuevalidator

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/Yakwilik/go-yamlvalidator/internal/checks"
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
	return checks.ParseFiniteNumber(raw)
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
