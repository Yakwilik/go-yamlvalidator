package yamlvalidator_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func TestAdversarialWideMapHonorsMaxDiagnostics(t *testing.T) {
	var input strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&input, "unknown_%d: %d\n", i, i)
	}
	schema := &FieldSchema{
		Type:             TypeMap,
		AllowedKeys:      map[string]*FieldSchema{},
		UnknownKeyPolicy: UnknownKeyError,
	}

	result := NewValidator(schema).ValidateWithOptions(
		[]byte(input.String()),
		ValidationContext{MaxDiagnostics: 25},
	)
	if got := len(result.Collector.All()); got != 25 {
		t.Fatalf("diagnostic limit ignored: got %d, want 25", got)
	}
	if !result.Truncated {
		t.Fatal("result must report truncation after MaxDiagnostics")
	}
}

func TestAdversarialDeepNestingHonorsMaxDepth(t *testing.T) {
	var input strings.Builder
	for i := 0; i < 300; i++ {
		input.WriteString(strings.Repeat("  ", i))
		fmt.Fprintf(&input, "k%d:\n", i)
	}
	input.WriteString(strings.Repeat("  ", 300))
	input.WriteString("value\n")

	result := NewValidator(&FieldSchema{
		Type:             TypeAny,
		UnknownKeyPolicy: UnknownKeyIgnore,
	}).ValidateWithOptions(
		[]byte(input.String()),
		ValidationContext{MaxDepth: 32},
	)
	if !containsDiagnosticCode(result, "max_depth") {
		t.Fatalf("expected max_depth diagnostic, got %v", result.Collector.Errors())
	}
}

func TestAdversarialAliasFanoutRemainsBounded(t *testing.T) {
	var input strings.Builder
	input.WriteString("base: &base\n  key: value\nitems:\n")
	for i := 0; i < 2000; i++ {
		input.WriteString("  - *base\n")
	}

	result := NewValidator(&FieldSchema{
		Type:             TypeAny,
		UnknownKeyPolicy: UnknownKeyIgnore,
	}).ValidateWithOptions(
		[]byte(input.String()),
		ValidationContext{
			MaxDepth:       64,
			MaxDiagnostics: 32,
		},
	)
	if result.Truncated {
		t.Fatalf("alias fanout unexpectedly exhausted diagnostic budget: %v", result.Collector.All())
	}
	if result.HasErrors() {
		t.Fatalf("valid alias fanout rejected: %v", result.Collector.Errors())
	}
}

func TestAdversarialHugeIntegerExactRange(t *testing.T) {
	maximum := "99999999999999999999999999999999999999999999999999"
	schema := &FieldSchema{
		Type: TypeInt,
		Validators: []ValueValidator{
			valv.RangeValidator{MaxExact: valv.MustExactNumber(maximum)},
		},
	}
	validator, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result := validator.ValidateBytes([]byte(maximum)); result.HasErrors() {
		t.Fatalf("huge exact boundary rejected: %v", result.Collector.Errors())
	}
	result := validator.ValidateBytes([]byte(maximum + "0"))
	if !containsDiagnosticCode(result, "maximum") {
		t.Fatalf("huge integer above maximum accepted: %v", result.Collector.Errors())
	}
}
