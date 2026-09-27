package yamlvalidator

import (
	"errors"
	"reflect"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPhase1GroupScopes(t *testing.T) {
	type endpoint struct {
		File   string `yaml:"file" yamlvalidate:"exactlyOneOf=[file,url,inline],nonempty"`
		URL    string `yaml:"url"`
		Inline string `yaml:"inline"`
	}
	for _, input := range []string{"{}\n", "file: a\nurl: b\n"} {
		var got endpoint
		requireDiagnosticCode(t, Unmarshal([]byte(input), &got), "exactly_one_of")
	}
	var got endpoint
	if err := Unmarshal([]byte("url: x\n"), &got); err != nil {
		t.Fatal(err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("file: ''\n"), &got), "nonempty")
	type wrapper struct {
		Choice map[string]any `yaml:"choice" yamlvalidate:"exactlyOneOfKeys=[file,url]"`
	}
	var nested wrapper
	requireDiagnosticCode(t, Unmarshal([]byte("choice: {}\n"), &nested), "exactly_one_of")
	if err := Unmarshal([]byte("{}\n"), &nested); err != nil {
		t.Fatal(err)
	}
	if err := Unmarshal([]byte("choice: null\n"), &nested); err != nil {
		t.Fatal(err)
	}
	type bad struct {
		Value string `yaml:"value" yamlvalidate:"exactlyOneOfKeys=[a,b]"`
	}
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("value: x\n"), new(bad)); !errors.As(err, &schemaErr) {
		t.Fatalf("want SchemaError: %v", err)
	}
}

func TestPhase1RegularYAMLIgnoresTags(t *testing.T) {
	type config struct {
		Name string `yaml:"name" yamlvalidate:"required,minLength=3"`
	}
	var plain config
	if err := yaml.Unmarshal([]byte("name: x\n"), &plain); err != nil {
		t.Fatal(err)
	}
	if plain.Name != "x" {
		t.Fatalf("plain decode: %#v", plain)
	}
	var checked config
	requireDiagnosticCode(t, Unmarshal([]byte("name: x\n"), &checked), "min_length")
}

func TestPhase1RejectedTags(t *testing.T) {
	types := []any{
		new(struct {
			_ struct{} `yamlvalidate:"exactlyOneOf=[a,b]"`
			A string   `yaml:"a"`
			B string   `yaml:"b"`
		}),
		new(struct {
			A string `yaml:"a" yamljsonschema:"{}"`
		}),
		new(struct {
			A string `yaml:"a" yamlvalidate:"jsonschema=name"`
		}),
		new(struct {
			A string `yaml:"a" yamlvalidate:"format=not-a-format"`
		}),
		new(struct {
			A any `yaml:"a" yamlvalidate:"exactlyOneOfKeys=[x,y],type=string"`
		}),
	}
	for _, out := range types {
		var schemaErr *SchemaError
		if err := Unmarshal([]byte("a: x\n"), out); !errors.As(err, &schemaErr) {
			t.Fatalf("want SchemaError for %T: %v", out, err)
		}
	}
}

func TestPhase1NativeFormatAndUniqueItems(t *testing.T) {
	type config struct {
		Host  string `yaml:"host" yamlvalidate:"format=hostname"`
		Items []any  `yaml:"items" yamlvalidate:"uniqueItems"`
	}
	var got config
	requireDiagnosticCode(t, Unmarshal([]byte("host: bad..host\nitems: []\n"), &got), "format")
	for _, items := range []string{"[1, 1.0]", "[a, a]", "[.nan, .nan]", "[&x {a: 1}, *x]"} {
		err := Unmarshal([]byte("host: example.com\nitems: "+items+"\n"), &got)
		requireDiagnosticCode(t, err, "unique_items")
	}
	if err := Unmarshal([]byte("host: example.com\nitems: [1, '1']\n"), &got); err != nil {
		t.Fatal(err)
	}
}

func TestPhase1IndependentAlternativesAndConditionDedup(t *testing.T) {
	type config struct {
		A    string `yaml:"a" yamlvalidate:"anyOfRequired=[[a],[b]],when={field=mode,eq=prod,require=[c]}"`
		B    string `yaml:"b" yamlvalidate:"anyOfRequired=[[c],[d]],when={field=mode,eq=prod,require=[c]}"`
		C    string `yaml:"c"`
		D    string `yaml:"d"`
		Mode string `yaml:"mode"`
	}
	var got config
	requireDiagnosticCode(t, Unmarshal([]byte("a: x\n"), &got), "any_of_required")
	if err := Unmarshal([]byte("a: x\nd: y\n"), &got); err != nil {
		t.Fatal(err)
	}
	var all *ValidationErrors
	err := Unmarshal([]byte("a: x\nd: y\nmode: prod\n"), &got)
	if !errors.As(err, &all) {
		t.Fatal(err)
	}
	count := 0
	for _, d := range all.Diagnostics() {
		if d.Code == "condition_required" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate condition diagnostics: %#v", all.Diagnostics())
	}
}

func TestPhase1NativeKeyFormatsAndDynamicMapping(t *testing.T) {
	type config struct {
		Hosts  map[string]string `yaml:"hosts" yamlvalidate:"keys={format=hostname}"`
		Choice any               `yaml:"choice" yamlvalidate:"exactlyOneOfKeys=[a,b]"`
	}
	var got config
	if err := Unmarshal([]byte("hosts: {api.example.com: ok}\nchoice: {a: null}\n"), &got); err != nil {
		t.Fatal(err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("hosts: {'bad host': x}\nchoice: {a: x}\n"), &got), "key_format")
	requireDiagnosticCode(t, Unmarshal([]byte("choice: text\n"), &got), "type_mismatch")
}

func TestPhase1UniqueItemsCustomTagsAndMerges(t *testing.T) {
	type config struct {
		Items []any `yaml:"items" yamlvalidate:"uniqueItems"`
	}
	var got config
	if err := Unmarshal([]byte("items: [!one x, !two x]\n"), &got); err != nil {
		t.Fatal(err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("items: [!one x, !one x]\n"), &got), "unique_items")
	requireDiagnosticCode(t, Unmarshal([]byte("items: [&base {a: 1}, {<<: *base}]\n"), &got), "unique_items")
}

func TestPhase1AllGroupNamesHaveExplicitScopes(t *testing.T) {
	cases := []struct {
		parent, value, badParent, badValue, goodParent, goodValue, code string
	}{
		{"exactlyOneOf=[a,b]", "exactlyOneOfKeys=[a,b]", "{}", "value: {}", "a: x", "value: {a: x}", "exactly_one_of"},
		{"mutuallyExclusive=[a,b]", "mutuallyExclusiveKeys=[a,b]", "a: x\nb: y", "value: {a: x, b: y}", "a: x", "value: {a: x}", "mutually_exclusive"},
		{"anyOfRequired=[[a],[b]]", "anyOfRequiredKeys=[[a],[b]]", "{}", "value: {}", "a: x", "value: {a: x}", "any_of_required"},
		{"oneOfRequired=[[a],[b]]", "oneOfRequiredKeys=[[a],[b]]", "a: x\nb: y", "value: {a: x, b: y}", "a: x", "value: {a: x}", "one_of_required"},
		{"forbiddenTogether=[[a,b]]", "forbiddenTogetherKeys=[[a,b]]", "a: x\nb: y", "value: {a: x, b: y}", "a: x", "value: {a: x}", "forbidden_together"},
		{"dependentRequired={a=[b]}", "dependentRequiredKeys={a=[b]}", "a: x", "value: {a: x}", "a: x\nb: y", "value: {a: x, b: y}", "dependent_required"},
		{"when={field=mode,eq=prod,require=[b]}", "whenKeys={field=mode,eq=prod,require=[b]}", "mode: prod", "value: {mode: prod}", "mode: dev", "value: {mode: dev}", "condition_required"},
		{"require=[a]", "requireKeys=[a]", "{}", "value: {}", "a: x", "value: {a: x}", "required"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			parent := reflect.StructOf([]reflect.StructField{
				{Name: "Carrier", Type: reflect.TypeFor[string](), Tag: reflect.StructTag("yaml:\"carrier\" yamlvalidate:" + strconv.Quote(tc.parent))},
				{Name: "A", Type: reflect.TypeFor[string](), Tag: `yaml:"a"`},
				{Name: "B", Type: reflect.TypeFor[string](), Tag: `yaml:"b"`},
				{Name: "Mode", Type: reflect.TypeFor[string](), Tag: `yaml:"mode"`},
			})
			value := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[map[string]any](), Tag: reflect.StructTag("yaml:\"value\" yamlvalidate:" + strconv.Quote(tc.value))}})
			requireDiagnosticCode(t, Unmarshal([]byte(tc.badParent), reflect.New(parent).Interface()), tc.code)
			requireDiagnosticCode(t, Unmarshal([]byte(tc.badValue), reflect.New(value).Interface()), tc.code)
			if err := Unmarshal([]byte(tc.goodParent), reflect.New(parent).Interface()); err != nil {
				t.Fatalf("parent valid: %v", err)
			}
			if err := Unmarshal([]byte(tc.goodValue), reflect.New(value).Interface()); err != nil {
				t.Fatalf("value valid: %v", err)
			}
		})
	}
}
