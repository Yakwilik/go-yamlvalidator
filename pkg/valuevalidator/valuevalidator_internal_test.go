package valuevalidator

import (
	"math"
	"math/big"
	"regexp"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

func runValueValidator(t *testing.T, validator v.ValueValidator, node *yaml.Node) []v.ValidationError {
	t.Helper()
	ctx := v.NewValidationContext()
	validator.Validate(node, "field", ctx)
	return ctx.Collector().Errors()
}

func scalar(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value, Line: 1, Column: 1}
}

func TestBuiltinValidatorsDirectly(t *testing.T) {
	tests := []struct {
		name      string
		validator v.ValueValidator
		node      *yaml.Node
		wantCode  string
	}{
		{"enum ok", EnumValidator{Allowed: []string{"dev", "prod"}}, scalar("!!str", "dev"), ""},
		{"enum bad", EnumValidator{Allowed: []string{"dev", "prod"}}, scalar("!!str", "test"), "enum"},
		{"regex", RegexValidator{Pattern: regexp.MustCompile("^a+$")}, scalar("!!str", "b"), "pattern"},
		{"nonempty", NonEmptyValidator{}, scalar("!!str", ""), "non_empty"},
		{"length", LengthValidator{Min: v.Ptr(2), Max: v.Ptr(3)}, scalar("!!str", "x"), "min_length"},
		{"url", URLValidator{RequireScheme: true, AllowedSchemes: []string{"https"}}, scalar("!!str", "http://example.com"), "url_scheme"},
		{"one type", OneOfTypeValidator{Types: []v.NodeType{v.TypeString, v.TypeInt}}, scalar("!!bool", "true"), "type_mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := runValueValidator(t, tc.validator, tc.node)
			if tc.wantCode == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) == 0 || errs[0].Code != tc.wantCode {
				t.Fatalf("errors=%v want code %q", errs, tc.wantCode)
			}
		})
	}
}

func TestExactNumberParsingAndComparison(t *testing.T) {
	for _, input := range []string{"1", "-1.25", "1e3", "0xff", "0o17", "0b101", "077"} {
		number, err := ParseExactNumber(input)
		if err != nil || number.String() == "" {
			t.Errorf("ParseExactNumber(%q)=%v,%v", input, number, err)
		}
	}
	for _, input := range []string{"", ".", "1e", "0x", "1e100001"} {
		if _, err := ParseExactNumber(input); err == nil {
			t.Errorf("invalid exact number %q accepted", input)
		}
	}

	one := finiteNumeric(newRat(t, "1"), "1")
	two := finiteNumeric(newRat(t, "2"), "2")
	if cmp, ok := compareNumeric(one, two); !ok || cmp >= 0 {
		t.Fatalf("1 vs 2 = %d,%v", cmp, ok)
	}
	if cmp, ok := compareNumeric(exactNumericValue{kind: numericNegativeInfinity}, one); !ok || cmp >= 0 {
		t.Fatalf("-inf compare = %d,%v", cmp, ok)
	}
	if _, ok := compareNumeric(exactNumericValue{kind: numericNaN}, one); ok {
		t.Fatal("NaN reported comparable")
	}
}

func newRat(t *testing.T, text string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("bad test rational %q", text)
	}
	return r
}

func TestExactNumberSpecialValues(t *testing.T) {
	if _, err := floatBound(math.NaN()); err == nil {
		t.Fatal("NaN bound accepted")
	}
	if got, err := floatBound(math.Inf(-1)); err != nil || got.kind != numericNegativeInfinity {
		t.Fatalf("-inf=%v,%v", got, err)
	}
	if got, err := floatBound(math.Inf(1)); err != nil || got.kind != numericPositiveInfinity {
		t.Fatalf("+inf=%v,%v", got, err)
	}
	if got, err := floatBound(1.25); err != nil || got.kind != numericFinite {
		t.Fatalf("finite=%v,%v", got, err)
	}
	if _, err := exactNumberValue(nil); err == nil {
		t.Fatal("nil exact number accepted")
	}
}

func TestRangeAndCloneBranches(t *testing.T) {
	rangeValidator := RangeValidator{Min: v.Ptr(1.0), Max: v.Ptr(2.0)}
	if err := rangeValidator.ValidateDefinition(); err != nil {
		t.Fatal(err)
	}
	if errs := runValueValidator(t, rangeValidator, scalar("!!int", "0")); len(errs) == 0 || errs[0].Code != "minimum" {
		t.Fatalf("minimum errors=%v", errs)
	}
	if errs := runValueValidator(t, rangeValidator, scalar("!!int", "3")); len(errs) == 0 || errs[0].Code != "maximum" {
		t.Fatalf("maximum errors=%v", errs)
	}

	clone := rangeValidator.CloneValueValidator().(RangeValidator)
	*rangeValidator.Min = 99
	if *clone.Min != 1 {
		t.Fatalf("range clone shared Min: %v", *clone.Min)
	}

	enum := EnumValidator{Allowed: []string{"a"}}
	enumClone := enum.CloneValueValidator().(EnumValidator)
	enum.Allowed[0] = "b"
	if enumClone.Allowed[0] != "a" {
		t.Fatal("enum clone shared slice")
	}

	urlv := URLValidator{AllowedSchemes: []string{"https"}}
	urlClone := urlv.CloneValueValidator().(URLValidator)
	urlv.AllowedSchemes[0] = "http"
	if urlClone.AllowedSchemes[0] != "https" {
		t.Fatal("url clone shared slice")
	}
}

func TestValidatorDefinitions(t *testing.T) {
	definitions := []struct {
		name      string
		validator interface{ ValidateDefinition() error }
		wantErr   bool
	}{
		{"enum valid", EnumValidator{Allowed: []string{"a"}}, false},
		{"enum empty", EnumValidator{}, true},
		{"enum duplicate", EnumValidator{Allowed: []string{"a", "a"}}, true},
		{"regex valid", RegexValidator{Pattern: regexp.MustCompile("a")}, false},
		{"regex nil", RegexValidator{}, true},
		{"length valid", LengthValidator{Min: v.Ptr(1), Max: v.Ptr(2)}, false},
		{"length negative", LengthValidator{Min: v.Ptr(-1)}, true},
		{"length inverted", LengthValidator{Min: v.Ptr(2), Max: v.Ptr(1)}, true},
		{"types valid", OneOfTypeValidator{Types: []v.NodeType{v.TypeString}}, false},
		{"types empty", OneOfTypeValidator{}, true},
		{"types duplicate", OneOfTypeValidator{Types: []v.NodeType{v.TypeString, v.TypeString}}, true},
		{"types invalid", OneOfTypeValidator{Types: []v.NodeType{v.NodeType(99)}}, true},
	}
	for _, tc := range definitions {
		err := tc.validator.ValidateDefinition()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestURLValidatorBranches(t *testing.T) {
	for _, tc := range []struct {
		v       URLValidator
		wantErr bool
	}{
		{URLValidator{}, false},
		{URLValidator{AllowedSchemes: []string{"https", "git+ssh"}}, false},
		{URLValidator{AllowedSchemes: []string{"1http"}}, true},
		{URLValidator{AllowedSchemes: []string{"HTTP", "http"}}, true},
	} {
		if err := tc.v.ValidateDefinition(); (err != nil) != tc.wantErr {
			t.Errorf("definition %+v err=%v wantErr=%v", tc.v, err, tc.wantErr)
		}
	}
	if !validURLScheme("git+ssh") || validURLScheme("1http") || isASCIIAlpha('1') {
		t.Fatal("URL scheme helpers mismatch")
	}

	required := URLValidator{RequireScheme: true}
	if errs := runValueValidator(t, required, scalar("!!str", "example.com/path")); len(errs) == 0 || errs[0].Code != "url_scheme" {
		t.Fatalf("missing scheme errors=%v", errs)
	}
	if errs := runValueValidator(t, required, scalar("!!str", "http://[::1")); len(errs) == 0 || errs[0].Code != "url" {
		t.Fatalf("bad URL errors=%v", errs)
	}
	if errs := runValueValidator(t, required, scalar("!!str", "https://example.com")); len(errs) != 0 {
		t.Fatalf("valid URL errors=%v", errs)
	}
}

func TestRemainingCloneBranches(t *testing.T) {
	regex := RegexValidator{Pattern: regexp.MustCompile("^a+$")}
	regexClone := regex.CloneValueValidator().(RegexValidator)
	if regexClone.Pattern == regex.Pattern || regexClone.Pattern.String() != regex.Pattern.String() {
		t.Fatal("regex clone did not copy pattern")
	}

	length := LengthValidator{Min: v.Ptr(1), Max: v.Ptr(3)}
	lengthClone := length.CloneValueValidator().(LengthValidator)
	*length.Min = 9
	if *lengthClone.Min != 1 {
		t.Fatal("length clone shared Min")
	}

	types := OneOfTypeValidator{Types: []v.NodeType{v.TypeString, v.TypeInt}}
	typesClone := types.CloneValueValidator().(OneOfTypeValidator)
	types.Types[0] = v.TypeBool
	if typesClone.Types[0] != v.TypeString {
		t.Fatal("type clone shared slice")
	}

	if _, ok := (NonEmptyValidator{}).CloneValueValidator().(NonEmptyValidator); !ok {
		t.Fatal("nonempty clone changed type")
	}
	exact := MustExactNumber("1.25")
	cloned := cloneExactNumber(exact)
	if cloned == exact || cloned.String() != exact.String() {
		t.Fatal("exact number clone failed")
	}
	if cloneExactNumber(nil) != nil {
		t.Fatal("nil exact clone not nil")
	}
}

func TestNumericComparisonAndRangeErrors(t *testing.T) {
	finite := finiteNumeric(newRat(t, "1"), "1")
	cases := []struct {
		left, right exactNumericValue
		want        int
		ok          bool
	}{
		{exactNumericValue{kind: numericNegativeInfinity}, exactNumericValue{kind: numericNegativeInfinity}, 0, true},
		{exactNumericValue{kind: numericPositiveInfinity}, exactNumericValue{kind: numericPositiveInfinity}, 0, true},
		{exactNumericValue{kind: numericPositiveInfinity}, finite, 1, true},
		{finite, exactNumericValue{kind: numericPositiveInfinity}, -1, true},
		{finite, exactNumericValue{kind: numericNegativeInfinity}, 1, true},
		{exactNumericValue{kind: numericNaN}, finite, 0, false},
	}
	for _, tc := range cases {
		got, ok := compareNumeric(tc.left, tc.right)
		if got != tc.want || ok != tc.ok {
			t.Errorf("compare=%d,%v want %d,%v", got, ok, tc.want, tc.ok)
		}
	}

	if err := (RangeValidator{Min: v.Ptr(1.0), MinExact: MustExactNumber("1")}).ValidateDefinition(); err == nil {
		t.Fatal("duplicate minimum representations accepted")
	}
	if err := (RangeValidator{Max: v.Ptr(1.0), MaxExact: MustExactNumber("1")}).ValidateDefinition(); err == nil {
		t.Fatal("duplicate maximum representations accepted")
	}
	if err := (RangeValidator{Min: v.Ptr(3.0), Max: v.Ptr(2.0)}).ValidateDefinition(); err == nil {
		t.Fatal("inverted range accepted")
	}
}
