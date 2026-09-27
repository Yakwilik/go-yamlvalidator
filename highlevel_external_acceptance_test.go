// Reviewer-authored phase-one contract tests. Fix implementation, do not weaken assertions.
package yamlvalidator_test

import (
	"errors"
	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
	"reflect"
	"testing"
)

type PhaseOneQAIntrinsic struct {
	File   *string "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url,inline]\""
	URL    *string "yaml:\"url,omitempty\""
	Inline *string "yaml:\"inline,omitempty\""
}
type PhaseOneQAPlain struct {
	File   *string "yaml:\"file,omitempty\""
	URL    *string "yaml:\"url,omitempty\""
	Inline *string "yaml:\"inline,omitempty\""
}

func PhaseOneQAAssertResult(t *testing.T, input string, out any, valid bool) {
	t.Helper()
	err := v.Unmarshal([]byte(input), out)
	if (err == nil) != valid {
		t.Fatalf("input=%q valid=%v err=%v", input, valid, err)
	}
	if !valid {
		var e *v.ValidationErrors
		if !errors.As(err, &e) {
			t.Fatalf("expected data validation error, got %T: %v", err, err)
		}
	}
}
func PhaseOneQAAssertDefinitionError(t *testing.T, out any) {
	t.Helper()
	err := v.Unmarshal([]byte("{}"), out)
	var e *v.SchemaError
	if !errors.As(err, &e) {
		t.Fatalf("expected SchemaError, got %T: %v", err, err)
	}
}
func TestHighLevelPhaseOneQA_IntrinsicAndMissingCarrier(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{"{}", false}, {"url: x", true}, {"file: null", true}, {"file: ''", true},
		{"file: x\nurl: y", false}, {"inline: z", true},
	} {
		t.Run(tc.input, func(t *testing.T) { PhaseOneQAAssertResult(t, tc.input, &PhaseOneQAIntrinsic{}, tc.valid) })
	}
}
func TestHighLevelPhaseOneQA_UseSiteAndCacheIsolation(t *testing.T) {
	type Config struct {
		A PhaseOneQAPlain "yaml:\"a\" yamlvalidate:\"required,exactlyOneOfKeys=[file,url]\""
		B PhaseOneQAPlain "yaml:\"b\" yamlvalidate:\"required,exactlyOneOfKeys=[url,inline]\""
		C PhaseOneQAPlain "yaml:\"c\""
	}
	for i := 0; i < 3; i++ {
		PhaseOneQAAssertResult(t, "a: {file: x}\nb: {inline: y}\nc: {}", &Config{}, true)
		PhaseOneQAAssertResult(t, "a: {inline: x}\nb: {file: y}\nc: {}", &Config{}, false)
		PhaseOneQAAssertResult(t, "{}", &PhaseOneQAPlain{}, true)
	}
}
func TestHighLevelPhaseOneQA_CarrierTypeDoesNotChangeParentScope(t *testing.T) {
	type Payload struct {
		Value string "yaml:\"value,omitempty\""
	}
	type Config struct {
		File Payload "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url]\""
		URL  string  "yaml:\"url,omitempty\""
	}
	PhaseOneQAAssertResult(t, "url: x", &Config{}, true)
	PhaseOneQAAssertResult(t, "file: {}", &Config{}, true)
	PhaseOneQAAssertResult(t, "file: {}\nurl: x", &Config{}, false)
}
func TestHighLevelPhaseOneQA_GroupsDeduplicateButStayIndependent(t *testing.T) {
	type Config struct {
		A *string "yaml:\"a,omitempty\" yamlvalidate:\"exactlyOneOf=[a,b]\""
		B *string "yaml:\"b,omitempty\" yamlvalidate:\"exactlyOneOf=[b,a]\""
		C *string "yaml:\"c,omitempty\" yamlvalidate:\"exactlyOneOf=[c,d]\""
		D *string "yaml:\"d,omitempty\""
	}
	PhaseOneQAAssertResult(t, "a: x\nd: y", &Config{}, true)
	err := v.Unmarshal([]byte("{}"), &Config{})
	var e *v.ValidationErrors
	if !errors.As(err, &e) {
		t.Fatal(err)
	}
	if len(e.Diagnostics()) != 2 {
		t.Fatalf("want two independent group errors (not 3 and not 1), got %#v", e.Diagnostics())
	}
}
func TestHighLevelPhaseOneQA_CollectionsAndOptionalContainer(t *testing.T) {
	type Config struct {
		Optional *PhaseOneQAPlain           "yaml:\"optional,omitempty\" yamlvalidate:\"exactlyOneOfKeys=[file,url]\""
		List     []PhaseOneQAPlain          "yaml:\"list\" yamlvalidate:\"items={exactlyOneOfKeys=[file,url]}\""
		Map      map[string]PhaseOneQAPlain "yaml:\"map\" yamlvalidate:\"values={exactlyOneOfKeys=[url,inline]}\""
	}
	PhaseOneQAAssertResult(t, "list: [{url: x}]\nmap: {one: {inline: y}}", &Config{}, true)
	PhaseOneQAAssertResult(t, "optional: null\nlist: [{file: x}]", &Config{}, true)
	PhaseOneQAAssertResult(t, "optional: {}", &Config{}, false)
	PhaseOneQAAssertResult(t, "list: [{}]", &Config{}, false)
	PhaseOneQAAssertResult(t, "map: {one: {}}", &Config{}, false)
}
func TestHighLevelPhaseOneQA_DefinitionFailures(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		type C struct {
			_ struct{} "yamlvalidate:\"exactlyOneOf=[a,b]\""
			A string
			B string
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("rawJSON", func(t *testing.T) {
		type C struct {
			A string "yamljsonschema:\"{}\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("namedJSON", func(t *testing.T) {
		type C struct {
			A string "yamlvalidate:\"jsonschema=x\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("scalarKeys", func(t *testing.T) {
		type C struct {
			A string "yamlvalidate:\"exactlyOneOfKeys=[a,b]\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("missingName", func(t *testing.T) {
		type C struct {
			A string "yaml:\"a\" yamlvalidate:\"exactlyOneOf=[a,missing]\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("implicitItemScope", func(t *testing.T) {
		type C struct {
			A []PhaseOneQAPlain "yamlvalidate:\"items={exactlyOneOf=[file,url]}\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("duplicateMember", func(t *testing.T) {
		type C struct {
			A string "yaml:\"a\" yamlvalidate:\"exactlyOneOf=[a,a]\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
	t.Run("unknownFormat", func(t *testing.T) {
		type C struct {
			A string "yamlvalidate:\"format=not-a-real-format\""
		}
		PhaseOneQAAssertDefinitionError(t, &C{})
	})
}
func TestHighLevelPhaseOneQA_InlineAndLiteralNames(t *testing.T) {
	type Config struct {
		PhaseOneQAIntrinsic "yaml:\",inline\""
		Other               string "yaml:\"other,omitempty\""
	}
	PhaseOneQAAssertResult(t, "url: x\nother: y", &Config{}, true)
	PhaseOneQAAssertResult(t, "other: y", &Config{}, false)
	type Dotted struct {
		A string "yaml:\"a.b,omitempty\" yamlvalidate:\"exactlyOneOf=['a.b','c[d]']\""
		B string "yaml:\"c[d],omitempty\""
	}
	PhaseOneQAAssertResult(t, "'c[d]': y", &Dotted{}, true)
}
func TestHighLevelPhaseOneQA_StandardAPIWithoutGenerationStaysStandard(t *testing.T) {
	type Config struct {
		Port int "yaml:\"port\" yamlvalidate:\"min=100\""
	}
	var c Config
	if err := yaml.Unmarshal([]byte("port: 1\nunknown: value"), &c); err != nil || c.Port != 1 {
		t.Fatalf("yaml backend changed: %#v %v", c, err)
	}
	PhaseOneQAAssertResult(t, "port: 1", &Config{}, false)
	data, err := yaml.Marshal(Config{1})
	if err != nil || len(data) == 0 {
		t.Fatal(err)
	}
	data, err = v.Marshal(Config{1})
	if err == nil || data != nil {
		t.Fatalf("high-level did not validate output: %q %v", data, err)
	}
}

var PhaseOneQAHookCalls int

type PhaseOneQAManual string

func (m *PhaseOneQAManual) UnmarshalYAML(n *yaml.Node) error {
	PhaseOneQAHookCalls++
	*m = PhaseOneQAManual(n.Value)
	return nil
}
func TestHighLevelPhaseOneQA_ManualHookBinding(t *testing.T) {
	registry, err := v.NewRegistry(v.RegistryConfig{TypeBindings: map[reflect.Type]v.TypeBinding{reflect.TypeOf(PhaseOneQAManual("")): {Decode: &v.FieldSchema{Type: v.TypeString}}}})
	if err != nil {
		t.Fatal(err)
	}
	type C struct {
		A PhaseOneQAManual "yaml:\"a\""
	}
	var c C
	PhaseOneQAHookCalls = 0
	err = (v.UnmarshalOptions{Registry: registry}).Unmarshal([]byte("a: hi"), &c)
	if err != nil || PhaseOneQAHookCalls != 1 || c.A != "hi" {
		t.Fatalf("calls=%d c=%#v err=%v", PhaseOneQAHookCalls, c, err)
	}
}
func TestHighLevelPhaseOneQA_NativeFormatsAndUniqueItems(t *testing.T) {
	type H struct {
		Host string "yaml:\"host\" yamlvalidate:\"format=hostname\""
	}
	PhaseOneQAAssertResult(t, "host: api.example.org", &H{}, true)
	PhaseOneQAAssertResult(t, "host: 'bad host'", &H{}, false)
	type U struct {
		Items []any "yaml:\"items\" yamlvalidate:\"uniqueItems\""
	}
	PhaseOneQAAssertResult(t, "items: [1, '1']", &U{}, true)
	PhaseOneQAAssertResult(t, "items: [1, 0x1]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [1, 1.0]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [.inf, -.inf]", &U{}, true)
	PhaseOneQAAssertResult(t, "items: [.nan, .nan]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [&x {a: 1}, *x]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [{a: 1, b: 2}, {b: 2, a: 1}]", &U{}, false)
}
func TestHighLevelPhaseOneQA_ValidationFailureDoesNotMutateAndMarshalUsesPresence(t *testing.T) {
	initial := "old"
	c := PhaseOneQAIntrinsic{File: &initial}
	err := v.Unmarshal([]byte("file: new\nurl: other"), &c)
	if err == nil || c.File != &initial || *c.File != "old" || c.URL != nil {
		t.Fatalf("mutation %#v %v", c, err)
	}
	if data, err := v.Marshal(PhaseOneQAIntrinsic{}); err == nil || data != nil {
		t.Fatalf("invalid output: %q %v", data, err)
	}
	if _, err := v.Marshal(c); err != nil {
		t.Fatal(err)
	}
}
func TestHighLevelPhaseOneQA_LowLevelJSONStillWorksAndRegistryIsNative(t *testing.T) {
	schema, err := v.CompileJSONSchema([]byte("{\"type\":\"integer\",\"minimum\":5}"))
	if err != nil {
		t.Fatal(err)
	}
	if !v.NewValidator(schema).ValidateBytes([]byte("3")).HasErrors() {
		t.Fatal("lost low-level JSON Schema")
	}
	typ := reflect.TypeOf(v.RegistryConfig{})
	for _, name := range []string{"JSONSchemas", "JSONOptions"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("legacy high-level registry field %s", name)
		}
	}
}

func TestHighLevelPhaseOneQA_ParentConditionsAndDependenciesOnAbsentCarrier(t *testing.T) {
	type C struct {
		Mode string "yaml:\"mode\" yamlvalidate:\"enum=[dev,prod]\""
		Cert string "yaml:\"cert,omitempty\" yamlvalidate:\"when={field=mode,eq=prod,require=[cert,key]},dependentRequired={cert=[key]}\""
		Key  string "yaml:\"key,omitempty\""
	}
	PhaseOneQAAssertResult(t, "mode: dev", &C{}, true)
	PhaseOneQAAssertResult(t, "mode: prod", &C{}, false)
	PhaseOneQAAssertResult(t, "mode: prod\ncert: c\nkey: k", &C{}, true)
	PhaseOneQAAssertResult(t, "mode: dev\ncert: c", &C{}, false)
}
func TestHighLevelPhaseOneQA_IntrinsicAndUseSiteGroupsAreConjoined(t *testing.T) {
	type C struct {
		Source PhaseOneQAIntrinsic "yaml:\"source\" yamlvalidate:\"required,exactlyOneOfKeys=[file,url]\""
	}
	PhaseOneQAAssertResult(t, "source: {inline: z}", &C{}, false)
	PhaseOneQAAssertResult(t, "source: {file: z}", &C{}, true)
	PhaseOneQAAssertResult(t, "source: {file: z, inline: z}", &C{}, false)
}
func TestHighLevelPhaseOneQA_InlineUseSiteKeysAndCarrierMoved(t *testing.T) {
	type C struct {
		PhaseOneQAPlain "yaml:\",inline\" yamlvalidate:\"exactlyOneOfKeys=[file,url]\""
		X               int "yaml:\"x,omitempty\""
	}
	PhaseOneQAAssertResult(t, "file: z\nx: 2", &C{}, true)
	PhaseOneQAAssertResult(t, "x: 2", &C{}, false)
	type Moved struct {
		A string "yaml:\"a,omitempty\""
		B string "yaml:\"b,omitempty\" yamlvalidate:\"exactlyOneOf=[a,b]\""
	}
	PhaseOneQAAssertResult(t, "a: x", &Moved{}, true)
	PhaseOneQAAssertResult(t, "{}", &Moved{}, false)
}
func TestHighLevelPhaseOneQA_WarningsStayOptionsAndSingleError(t *testing.T) {
	type C struct {
		A int "yaml:\"a\""
	}
	seen := 0
	opts := v.UnmarshalOptions{UnknownKeyPolicy: v.UnknownKeyWarn, OnDiagnostic: func(d v.ValidationError) { seen++ }}
	var out C
	err := opts.Unmarshal([]byte("a: 1\nextra: x"), &out)
	if err != nil || seen != 1 || out.A != 1 {
		t.Fatalf("warning result %#v count=%d err=%v", out, seen, err)
	}
	seen = 0
	out.A = 9
	opts.WarningsAsErrors = true
	err = opts.Unmarshal([]byte("a: 1\nextra: x"), &out)
	if err == nil || seen != 1 || out.A != 9 {
		t.Fatalf("warning promoted: %#v count=%d err=%v", out, seen, err)
	}
}
func TestHighLevelPhaseOneQA_NativeUniqueMergesAndTags(t *testing.T) {
	type U struct {
		Items []any "yaml:\"items\" yamlvalidate:\"uniqueItems\""
	}
	PhaseOneQAAssertResult(t, "items: [&base {a: 1}, {<<: *base}]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [!one x, !two x]", &U{}, true)
	PhaseOneQAAssertResult(t, "items: [!one x, !one x]", &U{}, false)
	PhaseOneQAAssertResult(t, "items: [1, true, null, '1']", &U{}, true)
}

func TestHighLevelPhaseOneQA_MappingGroupNeverSilentlyAcceptsScalar(t *testing.T) {
	type C struct {
		Value any "yaml:\"value\" yamlvalidate:\"exactlyOneOfKeys=[a,b]\""
	}
	err := v.Unmarshal([]byte("value: scalar"), &C{})
	if err == nil {
		t.Fatal("mapping group silently skipped on scalar dynamic value")
	}
}
func TestHighLevelPhaseOneQA_DefinitionsCheckedInsideCollections(t *testing.T) {
	type C struct {
		Items []PhaseOneQAPlain "yaml:\"items\" yamlvalidate:\"items={exactlyOneOfKeys=[missing,file]}\""
	}
	PhaseOneQAAssertDefinitionError(t, &C{})
	type D struct {
		Items map[string]PhaseOneQAPlain "yaml:\"items\" yamlvalidate:\"values={exactlyOneOfKeys=[missing,file]}\""
	}
	PhaseOneQAAssertDefinitionError(t, &D{})
}
func TestHighLevelPhaseOneQA_MarshalInputWithSharedAcyclicSlices(t *testing.T) {
	data := make([]any, 2)
	data[0] = "x"
	data[1] = data[:1]
	out, err := v.Marshal(data)
	if err != nil || len(out) == 0 {
		t.Fatalf("acyclic shared slice rejected: %v", err)
	}
}

func TestHighLevelPhaseOneQA_NativeNumericEdgeCases(t *testing.T) {
	type U struct {
		Items []any "yaml:\"items\" yamlvalidate:\"uniqueItems\""
	}
	t.Run("positiveInfinity", func(t *testing.T) { PhaseOneQAAssertResult(t, "items: [.inf, +.inf]", &U{}, false) })
	t.Run("underscoredFloat", func(t *testing.T) { PhaseOneQAAssertResult(t, "items: [1_0.0, 10]", &U{}, false) })
	type Bounded struct {
		Value any "yaml:\"value\" yamlvalidate:\"type=float,max=100\""
	}
	t.Run("boundedInfinity", func(t *testing.T) { PhaseOneQAAssertResult(t, "value: .inf", &Bounded{}, false) })
	t.Run("boundedNaN", func(t *testing.T) { PhaseOneQAAssertResult(t, "value: .nan", &Bounded{}, false) })
	type Floats struct {
		Value float64 "yaml:\"value\""
	}
	t.Run("hexIntegerAsFloat", func(t *testing.T) { PhaseOneQAAssertResult(t, "value: 0x10", &Floats{}, true) })
}

type phaseOneCountingCheck struct{ calls *int }

func (c phaseOneCountingCheck) Validate(n *yaml.Node, path string, ctx *v.ValidationContext) {
	*c.calls++
}
func TestHighLevelPhaseOneQA_InlineCheckRunsOnce(t *testing.T) {
	calls := 0
	reg, err := v.NewRegistry(v.RegistryConfig{ValueValidators: map[string]v.ValueValidator{"count": phaseOneCountingCheck{&calls}}})
	if err != nil {
		t.Fatal(err)
	}
	type Inner struct {
		A int "yaml:\"a\""
	}
	type Outer struct {
		Inner "yaml:\",inline\" yamlvalidate:\"check=count\""
	}
	var value Outer
	err = (v.UnmarshalOptions{Registry: reg}).Unmarshal([]byte("a: 1"), &value)
	if err != nil || calls != 1 {
		t.Fatalf("inline check called %d times: %v", calls, err)
	}
}
func TestHighLevelPhaseOneQA_UseSiteDuplicateDedup(t *testing.T) {
	type C struct {
		Source PhaseOneQAIntrinsic "yaml:\"source\" yamlvalidate:\"exactlyOneOfKeys=[inline,url,file]\""
	}
	err := v.Unmarshal([]byte("source: {}"), &C{})
	var e *v.ValidationErrors
	if !errors.As(err, &e) || len(e.Diagnostics()) != 1 {
		t.Fatalf("duplicate use-site/type invariant: %v", err)
	}
}
