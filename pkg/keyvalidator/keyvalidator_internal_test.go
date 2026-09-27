package keyvalidator

import (
	"regexp"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

func runKeyValidator(t *testing.T, validator v.KeyValidator, key string) []v.ValidationError {
	t.Helper()
	ctx := v.NewValidationContext()
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, Line: 1, Column: 1}
	validator.ValidateKey(key, node, key, ctx)
	return ctx.Collector().Errors()
}

func TestKeyValidators(t *testing.T) {
	regex := RegexKeyValidator{Pattern: regexp.MustCompile("^[a-z]+$")}
	if err := regex.ValidateDefinition(); err != nil {
		t.Fatal(err)
	}
	if errs := runKeyValidator(t, regex, "Name"); len(errs) == 0 || errs[0].Code != "key_pattern" {
		t.Fatalf("regex=%v", errs)
	}
	if errs := runKeyValidator(t, regex, "name"); len(errs) != 0 {
		t.Fatalf("valid regex=%v", errs)
	}
}

func TestForbiddenAndLengthKeyValidators(t *testing.T) {
	forbidden := ForbiddenKeyValidator{Forbidden: []string{"secret"}}
	if err := forbidden.ValidateDefinition(); err != nil {
		t.Fatal(err)
	}
	if errs := runKeyValidator(t, forbidden, "secret"); len(errs) == 0 || errs[0].Code != "forbidden_key" {
		t.Fatalf("forbidden=%v", errs)
	}
	if errs := runKeyValidator(t, forbidden, "name"); len(errs) != 0 {
		t.Fatalf("allowed=%v", errs)
	}

	length := LengthKeyValidator{Min: v.Ptr(2), Max: v.Ptr(4)}
	if err := length.ValidateDefinition(); err != nil {
		t.Fatal(err)
	}
	if errs := runKeyValidator(t, length, "x"); len(errs) == 0 || errs[0].Code != "key_min_length" {
		t.Fatalf("min=%v", errs)
	}
	if errs := runKeyValidator(t, length, "abcde"); len(errs) == 0 || errs[0].Code != "key_max_length" {
		t.Fatalf("max=%v", errs)
	}
	if errs := runKeyValidator(t, length, "name"); len(errs) != 0 {
		t.Fatalf("valid=%v", errs)
	}
}

func TestKeyValidatorDefinitionsAndClones(t *testing.T) {
	if err := (RegexKeyValidator{}).ValidateDefinition(); err == nil {
		t.Fatal("nil regex accepted")
	}
	if err := (ForbiddenKeyValidator{}).ValidateDefinition(); err == nil {
		t.Fatal("empty forbidden list accepted")
	}
	if err := (ForbiddenKeyValidator{Forbidden: []string{"x", "x"}}).ValidateDefinition(); err == nil {
		t.Fatal("duplicate forbidden key accepted")
	}
	if err := (LengthKeyValidator{Min: v.Ptr(-1)}).ValidateDefinition(); err == nil {
		t.Fatal("negative minimum accepted")
	}
	if err := (LengthKeyValidator{Min: v.Ptr(3), Max: v.Ptr(2)}).ValidateDefinition(); err == nil {
		t.Fatal("inverted length accepted")
	}

	regex := RegexKeyValidator{Pattern: regexp.MustCompile("^x$")}
	regexClone := regex.CloneKeyValidator().(RegexKeyValidator)
	if regexClone.Pattern == regex.Pattern || regexClone.Pattern.String() != "^x$" {
		t.Fatal("regex clone failed")
	}

	forbidden := ForbiddenKeyValidator{Forbidden: []string{"x"}}
	forbiddenClone := forbidden.CloneKeyValidator().(ForbiddenKeyValidator)
	forbidden.Forbidden[0] = "y"
	if forbiddenClone.Forbidden[0] != "x" {
		t.Fatal("forbidden clone shared slice")
	}

	length := LengthKeyValidator{Min: v.Ptr(1), Max: v.Ptr(2)}
	lengthClone := length.CloneKeyValidator().(LengthKeyValidator)
	*length.Min = 9
	if *lengthClone.Min != 1 {
		t.Fatal("length clone shared bound")
	}
}
