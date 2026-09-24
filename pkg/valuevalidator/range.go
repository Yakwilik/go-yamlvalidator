package valuevalidator

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// RangeValidator validates that a numeric value is within a range.
//
// Min/Max are kept for backwards compatibility and use their shortest decimal
// float64 representation as the bound. MinExact/MaxExact are available when a
// bound must be represented without float64 rounding. A side may configure
// either the legacy or exact form, but not both.
type RangeValidator struct {
	Min      *float64
	Max      *float64
	MinExact *ExactNumber
	MaxExact *ExactNumber
}

func (vld RangeValidator) ValidateDefinition() error {
	minimum, hasMinimum, maximum, hasMaximum, err := vld.bounds()
	if err != nil {
		return err
	}
	if hasMinimum && hasMaximum {
		cmp, ok := compareNumeric(minimum, maximum)
		if !ok {
			return fmt.Errorf("numeric bounds must be comparable")
		}
		if cmp > 0 {
			return fmt.Errorf("minimum must not exceed maximum")
		}
	}
	return nil
}

func (vld RangeValidator) bounds() (
	minimum exactNumericValue,
	hasMinimum bool,
	maximum exactNumericValue,
	hasMaximum bool,
	err error,
) {
	if vld.Min != nil && vld.MinExact != nil {
		err = fmt.Errorf("minimum cannot set both Min and MinExact")
		return
	}
	if vld.Max != nil && vld.MaxExact != nil {
		err = fmt.Errorf("maximum cannot set both Max and MaxExact")
		return
	}

	if vld.MinExact != nil {
		minimum, err = exactNumberValue(vld.MinExact)
		hasMinimum = true
		if err != nil {
			err = fmt.Errorf("minimum: %w", err)
			return
		}
	} else if vld.Min != nil {
		minimum, err = floatBound(*vld.Min)
		hasMinimum = true
		if err != nil {
			err = fmt.Errorf("minimum: %w", err)
			return
		}
	}

	if vld.MaxExact != nil {
		maximum, err = exactNumberValue(vld.MaxExact)
		hasMaximum = true
		if err != nil {
			err = fmt.Errorf("maximum: %w", err)
			return
		}
	} else if vld.Max != nil {
		maximum, err = floatBound(*vld.Max)
		hasMaximum = true
		if err != nil {
			err = fmt.Errorf("maximum: %w", err)
			return
		}
	}
	return
}

// Validate implements ValueValidator.
func (vld RangeValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	value, err := parseYAMLExactNumber(node)
	if err != nil {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Code:    "number",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "expected numeric value",
			Got:     node.Value,
		})
		return
	}
	minimum, hasMinimum, maximum, hasMaximum, err := vld.bounds()
	if err != nil {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Code:    "range_definition",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "invalid range validator definition",
			Got:     err.Error(),
		})
		return
	}

	if value.kind == numericNaN && (hasMinimum || hasMaximum) {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Code:    "number",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "NaN cannot be checked against numeric bounds",
			Got:     node.Value,
		})
		return
	}

	if hasMinimum {
		cmp, comparable := compareNumeric(value, minimum)
		if !comparable {
			return
		}
		if cmp < 0 {
			ctx.AddError(v.ValidationError{
				Level:    v.LevelError,
				Code:     "minimum",
				Path:     path,
				Line:     node.Line,
				Column:   node.Column,
				Message:  "value below minimum",
				Got:      value.display,
				Expected: ">= " + minimum.display,
				Details: v.NumericRangeDetails{
					Value: value.display,
					Bound: minimum.display,
				},
			})
		}
	}
	if hasMaximum {
		cmp, comparable := compareNumeric(value, maximum)
		if !comparable {
			return
		}
		if cmp > 0 {
			ctx.AddError(v.ValidationError{
				Level:    v.LevelError,
				Code:     "maximum",
				Path:     path,
				Line:     node.Line,
				Column:   node.Column,
				Message:  "value above maximum",
				Got:      value.display,
				Expected: "<= " + maximum.display,
				Details: v.NumericRangeDetails{
					Value: value.display,
					Bound: maximum.display,
				},
			})
		}
	}
}
