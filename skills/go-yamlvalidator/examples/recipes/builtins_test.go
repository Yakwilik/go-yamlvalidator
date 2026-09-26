package recipes_test

import (
	"regexp"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func TestBuiltinValueValidators(t *testing.T) {
	tests := []struct {
		name                 string
		typ                  v.NodeType
		validator            v.ValueValidator
		valid, invalid, code string
	}{
		{"enum", v.TypeString, valv.EnumValidator{Allowed: []string{"dev", "prod"}}, "prod", "unknown", "enum"},
		{"regex", v.TypeString, valv.RegexValidator{Pattern: regexp.MustCompile("^[a-z]+$")}, "service", "Service", "pattern"},
		{"range", v.TypeInt, valv.RangeValidator{Min: v.Ptr(1.0), Max: v.Ptr(10.0)}, "2", "11", "maximum"},
		{"exact range", v.TypeInt, valv.RangeValidator{MinExact: valv.MustExactNumber("9007199254740993")}, "9007199254740993", "9007199254740992", "minimum"},
		{"nonempty", v.TypeString, valv.NonEmptyValidator{}, "value", "\"\"", "non_empty"},
		{"length", v.TypeString, valv.LengthValidator{Min: v.Ptr(2), Max: v.Ptr(3)}, "ёж", "x", "min_length"},
		{"url", v.TypeString, valv.URLValidator{RequireScheme: true, AllowedSchemes: []string{"https"}}, "https://example.test/path", "http://example.test", "url_scheme"},
		{"one of type", v.TypeAny, valv.OneOfTypeValidator{Types: []v.NodeType{v.TypeString, v.TypeInt}}, "2", "true", "type_mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := &v.FieldSchema{Type: tc.typ, Validators: []v.ValueValidator{tc.validator}}
			checked, err := v.CompileFieldSchema(schema)
			if err != nil {
				t.Fatal(err)
			}
			checkInputs(t, checked, v.ValidationContext{}, []inputCase{
				{name: "accepted", input: tc.valid, valid: true}, {name: "rejected", input: tc.invalid, code: tc.code},
			})
		})
	}
	bound, err := valv.ParseExactNumber("1.25")
	if err != nil {
		t.Fatal(err)
	}
	if bound.String() == "" {
		t.Fatal("empty exact representation")
	}
}

func TestBuiltinKeyValidators(t *testing.T) {
	tests := []struct {
		name            string
		validator       v.KeyValidator
		good, bad, code string
	}{
		{"regex", keyv.RegexKeyValidator{Pattern: regexp.MustCompile("^[a-z]+$")}, "name", "Name", "key_pattern"},
		{"forbidden", keyv.ForbiddenKeyValidator{Forbidden: []string{"secret"}}, "name", "secret", "forbidden_key"},
		{"length", keyv.LengthKeyValidator{Min: v.Ptr(2), Max: v.Ptr(5)}, "name", "n", "key_min_length"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := &v.FieldSchema{Type: v.TypeMap, AdditionalProperties: &v.FieldSchema{Type: v.TypeString}, KeyValidators: []v.KeyValidator{tc.validator}}
			checked, err := v.CompileFieldSchema(schema)
			if err != nil {
				t.Fatal(err)
			}
			checkInputs(t, checked, v.ValidationContext{}, []inputCase{
				{name: "accepted", input: tc.good + ": value", valid: true}, {name: "rejected", input: tc.bad + ": value", code: tc.code},
			})
		})
	}
}

func TestMalformedBuiltinDefinitions(t *testing.T) {
	tests := []struct {
		name      string
		validator v.ValueValidator
	}{
		{"nil regex", valv.RegexValidator{}},
		{"empty enum", valv.EnumValidator{}},
		{"duplicate enum", valv.EnumValidator{Allowed: []string{"x", "x"}}},
		{"range sides", valv.RangeValidator{Min: v.Ptr(1.0), MinExact: valv.MustExactNumber("1")}},
		{"range inverted", valv.RangeValidator{Min: v.Ptr(3.0), Max: v.Ptr(2.0)}},
		{"negative length", valv.LengthValidator{Min: v.Ptr(-1)}},
		{"bad url scheme", valv.URLValidator{AllowedSchemes: []string{"1https"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := v.CompileFieldSchema(&v.FieldSchema{Type: v.TypeAny, Validators: []v.ValueValidator{tc.validator}}); err == nil {
				t.Fatal("expected definition error")
			}
		})
	}
}
