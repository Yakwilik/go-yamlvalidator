package yamlvalidator

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestHighLevelUnicodeAtomAndGoTagEscape(t *testing.T) {
	type config struct {
		Name string `yaml:"name" custom:"\x41" yamlvalidate:"enum=[Россия,мир]"`
	}
	var got config
	if err := Unmarshal([]byte("name: Россия\n"), &got); err != nil {
		t.Fatal(err)
	}
}

func TestHighLevelDefinitionBoundsAndShape(t *testing.T) {
	type badBounds struct {
		Value int `yaml:"value" yamlvalidate:"min=9,max=1"`
	}
	var bad badBounds
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("{}\n"), &bad); !errors.As(err, &schemaErr) {
		t.Errorf("contradictory bounds: %v", err)
	}
	type badNonempty struct {
		Value int `yaml:"value" yamlvalidate:"nonempty"`
	}
	var nonempty badNonempty
	if err := Unmarshal([]byte("{}\n"), &nonempty); !errors.As(err, &schemaErr) {
		t.Errorf("nonempty integer: %v", err)
	}
	type boundedArray struct {
		Values [2]int `yaml:"values" yamlvalidate:"minItems=0,maxItems=100"`
	}
	var array boundedArray
	err := Unmarshal([]byte("values: [1]\n"), &array)
	var validation *ValidationErrors
	if !errors.As(err, &schemaErr) && !errors.As(err, &validation) {
		t.Errorf("array shape reached codec: %v", err)
	}
}

func TestHighLevelInlineMapRulesSeeCapturedKeysOnly(t *testing.T) {
	type config struct {
		Fixed  string         `yaml:"fixed"`
		Extras map[string]int `yaml:",inline" yamlvalidate:"minProperties=1,maxProperties=1,keys={pattern='^x_'},values={min=1,max=9}"`
	}
	var got config
	if err := Unmarshal([]byte("fixed: ok\nx_one: 4\n"), &got); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"fixed: ok\n", "fixed: ok\nx_one: 1\nx_two: 2\n", "fixed: ok\nbad: 1\n", "fixed: ok\nx_one: 10\n"} {
		if err := Unmarshal([]byte(source), &got); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}

func TestHighLevelInlineStructGroupsApplyAtParent(t *testing.T) {
	type child struct {
		Left  string `yaml:"left" yamlvalidate:"exactlyOneOf=[left,right]"`
		Right string `yaml:"right"`
	}
	type config struct {
		Extra child  `yaml:",inline"`
		Name  string `yaml:"name"`
	}
	var got config
	if err := Unmarshal([]byte("name: n\nleft: yes\n"), &got); err != nil {
		t.Fatal(err)
	}
	if err := Unmarshal([]byte("name: n\n"), &got); !diagnosticCode(err, "exactly_one_of") {
		t.Errorf("inline group lost: %v", err)
	}
}

func TestHighLevelInlineStructReferencesParentKeys(t *testing.T) {
	type child struct {
		TLS bool `yaml:"tls" yamlvalidate:"require=[mode],when={field=mode,eq=prod,require=[tls]}"`
	}
	type config struct {
		Child child  `yaml:",inline"`
		Mode  string `yaml:"mode"`
	}
	var got config
	if err := Unmarshal([]byte("mode: prod\ntls: true\n"), &got); err != nil {
		t.Fatal(err)
	}
	if err := Unmarshal([]byte("tls: true\n"), &got); !diagnosticCode(err, "required") {
		t.Fatalf("inline required sibling not enforced: %v", err)
	}
	if err := Unmarshal([]byte("mode: prod\n"), &got); !diagnosticCode(err, "condition_required") {
		t.Fatalf("inline condition sibling not enforced: %v", err)
	}
}

func TestHighLevelInlineStructBadReferenceReportsChildTag(t *testing.T) {
	type child struct {
		TLS bool `yaml:"tls" yamlvalidate:"description='child',when={field=missing,eq=yes,require=[mode]}"`
	}
	type config struct {
		Child child  `yaml:",inline"`
		Mode  string `yaml:"mode"`
	}
	var got config
	var schemaErr *SchemaError
	err := Unmarshal([]byte("mode: yes\n"), &got)
	if !errors.As(err, &schemaErr) || schemaErr.Field != "TLS" || schemaErr.Offset != len("description='child',") {
		t.Fatalf("inline child rule lost source position: %v", err)
	}
}

func TestHighLevelSchemaErrorCarriesRuleOffset(t *testing.T) {
	type config struct {
		Value string `yaml:"value" yamlvalidate:"required,does_not_exist"`
	}
	var got config
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("value: x"), &got); !errors.As(err, &schemaErr) {
		t.Fatalf("expected SchemaError: %v", err)
	}
	if schemaErr.Offset != 9 || schemaErr.Field != "Value" || schemaErr.Tag != "required,does_not_exist" {
		t.Fatalf("schema context: %+v", schemaErr)
	}
}

func TestHighLevelSemanticErrorsCarryOffendingOffsets(t *testing.T) {
	cases := []struct {
		fieldType reflect.Type
		tag       string
		offset    int
	}{
		{reflect.TypeFor[string](), "required,nullable", 9},
		{reflect.TypeFor[int](), "min=9,max=1", 6},
		{reflect.TypeFor[uint8](), "min=300", 0},
		{reflect.TypeFor[map[string]string](), "keys={minLength=5,maxLength=1}", 18},
		{reflect.TypeFor[[]string](), "minItems=5,maxItems=1", 11},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: tc.fieldType, Tag: reflect.StructTag("yaml:\"value\" yamlvalidate:" + strconv.Quote(tc.tag))}})
			err := Unmarshal([]byte("{}\n"), reflect.New(typ).Interface())
			var schemaErr *SchemaError
			if !errors.As(err, &schemaErr) || schemaErr.Offset != tc.offset {
				t.Fatalf("wanted offset %d, got %v", tc.offset, err)
			}
		})
	}
}

func TestHighLevelOuterTagInvalidEscapeOffset(t *testing.T) {
	outer := `yaml:"value" custom:"bad\q"`
	_, err := parseOuterStructTag(outer)
	var offset *tagOffsetError
	if !errors.As(err, &offset) || offset.Offset != strings.Index(outer, `\q`) {
		t.Fatalf("wanted bad escape offset %d, got %v", strings.Index(outer, `\q`), err)
	}
}

func TestHighLevelOuterTagUnterminatedOpeningQuote(t *testing.T) {
	for _, input := range []string{`0:"`, `0:"\`, `yaml:"name`} {
		if _, err := parseOuterStructTag(input); err == nil {
			t.Fatalf("accepted unterminated tag %q", input)
		}
	}
}

func TestHighLevelObjectReferenceErrorKeepsTagLocation(t *testing.T) {
	declaration := "description='scope',when={field=missing,eq=yes,require=[name]}"
	typ := reflect.StructOf([]reflect.StructField{
		{Name: "Name", Type: reflect.TypeFor[string](), Tag: reflect.StructTag("yaml:\"name\" yamlvalidate:" + strconv.Quote(declaration))},
	})
	var schemaErr *SchemaError
	err := Unmarshal([]byte("name: ok\n"), reflect.New(typ).Interface())
	if !errors.As(err, &schemaErr) || schemaErr.Field != "Name" || schemaErr.Offset != len("description='scope',") || schemaErr.Tag != declaration {
		t.Fatalf("object reference lost tag location: %v", err)
	}
}
