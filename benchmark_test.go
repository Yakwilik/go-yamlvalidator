package yamlvalidator_test

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func BenchmarkNativeCompile(b *testing.B) {
	schema := benchmarkNativeSchema()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := CompileFieldSchema(schema); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNativeValidationSmall(b *testing.B) {
	validator, err := CompileFieldSchema(benchmarkNativeSchema())
	if err != nil {
		b.Fatal(err)
	}
	input := []byte(`name: example
count: 42
tags: [one, two, three]
`)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		result := validator.ValidateBytes(input)
		if result.HasErrors() {
			b.Fatal(result.Collector.Errors())
		}
	}
}

func BenchmarkNativeValidationLarge(b *testing.B) {
	validator, err := CompileFieldSchema(&FieldSchema{
		Type:                 TypeMap,
		AdditionalProperties: &FieldSchema{Type: TypeInt},
	})
	if err != nil {
		b.Fatal(err)
	}
	input := benchmarkLargeMapping(500)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		result := validator.ValidateBytes(input)
		if result.HasErrors() {
			b.Fatal(result.Collector.Errors())
		}
	}
}

func BenchmarkJSONSchemaCompile(b *testing.B) {
	schema := benchmarkJSONSchema()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := CompileJSONSchema(schema); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONSchemaValidationSmall(b *testing.B) {
	schema, err := CompileJSONSchema(benchmarkJSONSchema())
	if err != nil {
		b.Fatal(err)
	}
	validator := NewValidator(schema)
	input := []byte(`name: example
count: 42
tags: [one, two, three]
`)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		result := validator.ValidateBytes(input)
		if result.HasErrors() {
			b.Fatal(result.Collector.Errors())
		}
	}
}

func BenchmarkJSONSchemaValidationLarge(b *testing.B) {
	schema, err := CompileJSONSchema([]byte(`{
		"type":"object",
		"additionalProperties":{"type":"integer"}
	}`))
	if err != nil {
		b.Fatal(err)
	}
	validator := NewValidator(schema)
	input := benchmarkLargeMapping(500)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		result := validator.ValidateBytes(input)
		if result.HasErrors() {
			b.Fatal(result.Collector.Errors())
		}
	}
}

func benchmarkNativeSchema() *FieldSchema {
	return &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name":  {Type: TypeString, Required: true},
			"count": {Type: TypeInt, Required: true},
			"tags": {
				Type:       TypeSequence,
				ItemSchema: &FieldSchema{Type: TypeString},
			},
		},
		UnknownKeyPolicy: UnknownKeyError,
	}
}

func benchmarkJSONSchema() []byte {
	return []byte(`{
		"type":"object",
		"properties":{
			"name":{"type":"string"},
			"count":{"type":"integer"},
			"tags":{"type":"array","items":{"type":"string"}}
		},
		"required":["name","count"],
		"additionalProperties":false
	}`)
}

func benchmarkLargeMapping(entries int) []byte {
	var builder strings.Builder
	builder.Grow(entries * 16)
	for i := range entries {
		fmt.Fprintf(&builder, `field_%d: %d
`, i, i)
	}
	return []byte(builder.String())
}
