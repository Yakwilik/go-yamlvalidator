package valuevalidator

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// RangeValidator validates that a numeric value is within a range.
type RangeValidator struct {
	Min *float64 // Minimum value (nil = no minimum)
	Max *float64 // Maximum value (nil = no maximum)
}

// Validate implements ValueValidator.
func (vld RangeValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	val, err := parseYAMLNumber(node)
	if err != nil {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "expected numeric value",
			Got:     node.Value,
		})
		return
	}

	if vld.Min != nil && val < *vld.Min {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     node.Line,
			Column:   node.Column,
			Message:  "value below minimum",
			Got:      fmt.Sprintf("%v", val),
			Expected: fmt.Sprintf(">= %v", *vld.Min),
		})
	}

	if vld.Max != nil && val > *vld.Max {
		ctx.AddError(v.ValidationError{
			Level:    v.LevelError,
			Path:     path,
			Line:     node.Line,
			Column:   node.Column,
			Message:  "value above maximum",
			Got:      fmt.Sprintf("%v", val),
			Expected: fmt.Sprintf("<= %v", *vld.Max),
		})
	}
}

func parseYAMLNumber(node *yaml.Node) (float64, error) {
	val := node.Value
	lower := strings.ToLower(val)

	// Handle YAML 1.2/1.1 special floats
	if lower == ".inf" || lower == "+.inf" || lower == "-.inf" {
		if strings.HasPrefix(lower, "-") {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	}
	if lower == ".nan" {
		return math.NaN(), nil
	}

	// Try standard float parsing
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f, nil
	}

	// Try int forms, including hex/bin/octal (0o) and +/-
	s := val
	sign := 1.0
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	}

	// Octal 0o / 0O
	if strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
		if i, err := strconv.ParseInt(s[2:], 8, 64); err == nil {
			return float64(sign) * float64(i), nil
		}
	}
	// Binary 0b / 0B
	if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
		if i, err := strconv.ParseInt(s[2:], 2, 64); err == nil {
			return float64(sign) * float64(i), nil
		}
	}
	// Hex (0x...) or plain decimal
	if i, err := strconv.ParseInt(s, 0, 64); err == nil {
		return float64(sign) * float64(i), nil
	}

	return 0, fmt.Errorf("not a numeric value")
}
