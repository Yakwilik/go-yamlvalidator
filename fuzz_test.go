package yamlvalidator_test

import (
	"reflect"
	"testing"
	"unicode/utf8"

	. "github.com/Yakwilik/go-yamlvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func FuzzNativeValidationNoPanic(f *testing.F) {
	schema, err := CompileFieldSchema(&FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name":  {Type: TypeString, Required: true},
			"items": {Type: TypeSequence, ItemSchema: &FieldSchema{Type: TypeInt}},
		},
		UnknownKeyPolicy: UnknownKeyError,
	})
	if err != nil {
		f.Fatalf("compile seed schema: %v", err)
	}
	for _, seed := range [][]byte{
		[]byte(`name: app
items: [1, 2]
`),
		[]byte("name: ["),
		[]byte(`a: &a
  self: *a
`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		result := schema.ValidateWithOptions(data, ValidationContext{
			MaxBytes:       1 << 20,
			MaxDepth:       64,
			MaxDocuments:   32,
			MaxDiagnostics: 64,
		})
		if result == nil || result.Collector == nil {
			t.Fatal("validation returned nil result/collector")
		}
	})
}

func FuzzJSONSchemaValidationNoPanic(f *testing.F) {
	schema, err := CompileJSONSchema([]byte(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"additionalProperties":{"type":["string","number","boolean","null"]}
	}`))
	if err != nil {
		f.Fatalf("compile seed schema: %v", err)
	}
	f.Add([]byte(`x: value`))
	f.Add([]byte(`"0": 184467440737095516160000`))
	f.Add([]byte(`x: &x [*x]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		result := NewValidator(schema).ValidateWithOptions(data, ValidationContext{
			MaxBytes:       1 << 20,
			MaxDepth:       64,
			MaxDiagnostics: 64,
		})
		if result == nil || result.Collector == nil {
			t.Fatal("validation returned nil result/collector")
		}
	})
}

func FuzzCompileJSONSchemaNoPanic(f *testing.F) {
	f.Add([]byte(`{"type":"string"}`))
	f.Add([]byte(`true`))
	f.Add([]byte(`{"oneOf":[{"type":"integer"},{"type":"string"}]}`))

	f.Fuzz(func(t *testing.T, schemaData []byte) {
		schema, err := CompileJSONSchema(schemaData)
		if err != nil {
			return
		}
		result := NewValidator(schema).ValidateWithOptions(
			[]byte("value"),
			ValidationContext{MaxDepth: 32, MaxDiagnostics: 32},
		)
		if result == nil {
			t.Fatal("compiled schema returned nil validation result")
		}
	})
}

func FuzzPathTokensRoundTrip(f *testing.F) {
	f.Add("name", uint8(0))
	f.Add("a.b", uint8(1))
	f.Add("0", uint8(255))
	f.Add(`quote"slash\`, uint8(7))

	f.Fuzz(func(t *testing.T, property string, index uint8) {
		if !utf8.ValidString(property) {
			t.Skip()
		}
		tokens := []PathToken{
			{Kind: PathTokenProperty, Name: property},
			{Kind: PathTokenIndex, Index: int(index)},
		}
		path := FormatPathTokens(tokens)
		parsed, err := ParsePathTokens(path)
		if err != nil {
			t.Fatalf("parse formatted path %q: %v", path, err)
		}
		if !reflect.DeepEqual(parsed, tokens) {
			t.Fatalf(
				"round trip mismatch: got %#v want %#v (path %q)",
				parsed, tokens, path,
			)
		}
	})
}

func FuzzExactNumberRangeNoPanic(f *testing.F) {
	f.Add("9007199254740993")
	f.Add("1.0000000000000000001e-20")
	f.Add("0xFFFFFFFFFFFFFFFFFFFFFFFF")
	f.Add("-0.0000000000000000000001")

	f.Fuzz(func(t *testing.T, literal string) {
		number, err := valv.ParseExactNumber(literal)
		if err != nil {
			return
		}
		schema := &FieldSchema{
			Type: TypeFloat,
			Validators: []ValueValidator{
				valv.RangeValidator{MinExact: &number, MaxExact: &number},
			},
		}
		_ = NewValidator(schema).ValidateBytes([]byte(number.String()))
	})
}
