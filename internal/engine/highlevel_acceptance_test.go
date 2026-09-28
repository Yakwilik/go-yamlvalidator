package yamlvalidator

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func requireDiagnosticCode(t *testing.T, err error, code string) {
	t.Helper()
	var all *ValidationErrors
	if !errors.As(err, &all) {
		t.Fatalf("expected ValidationErrors for %q, got %v", code, err)
	}
	for _, diagnostic := range all.Diagnostics() {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %q in %#v", code, all.Diagnostics())
}

type highLevelCustomText struct {
	Value string
	Calls *int
}

func (c *highLevelCustomText) UnmarshalYAML(node *yaml.Node) error {
	if c.Calls != nil {
		*c.Calls++
	}
	c.Value = node.Value
	return nil
}

type highLevelCustomOutput struct{ Calls *int }

func (c highLevelCustomOutput) MarshalYAML() (any, error) {
	if c.Calls != nil {
		*c.Calls++
	}
	return "encoded", nil
}

func TestHighLevelPresenceNullAndOmitEmpty(t *testing.T) {
	type config struct {
		Value *string `yaml:"value,omitempty" yamlvalidate:"required"`
	}
	var result config
	if err := Unmarshal([]byte("value: null\n"), &result); err != nil {
		t.Fatalf("required pointer accepts present null: %v", err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("{}\n"), &result), "required")
	data, err := Marshal(config{})
	if data != nil {
		t.Fatalf("invalid Marshal returned bytes: %q", data)
	}
	requireDiagnosticCode(t, err, "required")
	var nonnull struct {
		Value *string `yaml:"value" yamlvalidate:"required,notnull"`
	}
	requireDiagnosticCode(t, Unmarshal([]byte("value: null\n"), &nonnull), "type_mismatch")
}

func TestHighLevelDocumentAndSchemaFailurePreserveDestination(t *testing.T) {
	type config struct {
		Name string `yaml:"name" yamlvalidate:"required"`
	}
	initial := config{Name: "kept"}
	for _, input := range []string{"", "name: first\n---\nname: second\n", "other: x\n"} {
		got := initial
		if err := Unmarshal([]byte(input), &got); err == nil {
			t.Fatalf("accepted %q", input)
		}
		if got != initial {
			t.Fatalf("destination changed for %q: %#v", input, got)
		}
	}
	type malformed struct {
		Name string `yaml:"name" yamlvalidate:"min=oops"`
	}
	var got malformed
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("name: x\n"), &got); !errors.As(err, &schemaErr) {
		t.Fatalf("expected SchemaError: %v", err)
	}
}

func TestHighLevelPolicyWarningsAndCallback(t *testing.T) {
	type config struct {
		Name string `yaml:"name" yamlvalidate:"deprecated"`
	}
	var called []ValidationError
	opts := UnmarshalOptions{UnknownKeyPolicy: UnknownKeyWarn, OnDiagnostic: func(d ValidationError) { called = append(called, d) }}
	var got config
	if err := opts.Unmarshal([]byte("name: old\nextra: value\n"), &got); err != nil {
		t.Fatal(err)
	}
	if len(called) != 2 || called[0].Level != LevelWarning || called[1].Level != LevelWarning {
		t.Fatalf("callback got %#v", called)
	}
	opts.WarningsAsErrors = true
	requireDiagnosticCode(t, opts.Unmarshal([]byte("name: old\n"), &got), "deprecated")
	if err := Unmarshal([]byte("extra: value\n"), &got); err == nil {
		t.Fatal("default high-level policy accepted unknown key")
	}
	if err := (UnmarshalOptions{UnknownKeyPolicy: UnknownKeyIgnore}).Unmarshal([]byte("extra: value\n"), &got); err != nil {
		t.Fatalf("explicit ignore policy rejected unknown key: %v", err)
	}
}

func TestHighLevelNestedScopeAndNumericExactness(t *testing.T) {
	type config struct {
		Values map[string]uint64 `yaml:"values" yamlvalidate:"values={min=9007199254740993},keys={pattern='^[a-z]+$'}"`
		Items  []string          `yaml:"items" yamlvalidate:"items={minLength=2},minItems=1"`
	}
	var got config
	if err := Unmarshal([]byte("values:\n  alpha: 9007199254740993\nitems: [ab]\n"), &got); err != nil {
		t.Fatal(err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("values:\n  alpha: 9007199254740992\nitems: [ab]\n"), &got), "min")
	requireDiagnosticCode(t, Unmarshal([]byte("values:\n  Bad: 9007199254740993\nitems: [ab]\n"), &got), "key_pattern")
	requireDiagnosticCode(t, Unmarshal([]byte("values:\n  alpha: 9007199254740993\nitems: [a]\n"), &got), "min_length")
	requireDiagnosticCode(t, Unmarshal([]byte("values:\n  alpha: 18446744073709551616\nitems: [ab]\n"), &got), "representability")
}

func TestHighLevelMetadataGroupsAndAliasCondition(t *testing.T) {
	type config struct {
		Base string `yaml:"base"`
		Mode string `yaml:"mode"`
		TLS  bool   `yaml:"tls"`
		File string `yaml:"file" yamlvalidate:"exactlyOneOf=[file,url],when={field=mode,eq=prod,require=[tls]}"`
		URL  string `yaml:"url"`
	}
	var got config
	requireDiagnosticCode(t, Unmarshal([]byte("file: f\nurl: u\n"), &got), "exactly_one_of")
	requireDiagnosticCode(t, Unmarshal([]byte("base: &prod prod\nmode: *prod\nfile: f\n"), &got), "condition_required")
	if err := Unmarshal([]byte("mode: &prod prod\nfile: f\ntls: true\n"), &got); err != nil {
		t.Fatal(err)
	}
}

func TestHighLevelJSONSchemaRawAndSkipValidation(t *testing.T) {
	type config struct {
		Host string `yaml:"host" yamlvalidate:"minLength=3"`
	}
	var got config
	if err := Unmarshal([]byte("host: x\n"), &got); err == nil {
		t.Fatal("native length rule was ignored")
	}
	if err := (UnmarshalOptions{SkipValidation: true}).Unmarshal([]byte("host: x\n"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Host != "x" {
		t.Fatalf("decoded %#v", got)
	}
}

func TestHighLevelDiagnosticSourceIsExplicit(t *testing.T) {
	type config struct {
		Value int `yaml:"value" yamlvalidate:"min=2"`
	}
	data, err := Marshal(config{Value: 1})
	if data != nil {
		t.Fatalf("Marshal returned invalid bytes: %q", data)
	}
	var validation *ValidationErrors
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationErrors: %v", err)
	}
	if !validation.GeneratedSource() {
		t.Fatal("Marshal source flag missing")
	}
	if strings.Contains(validation.Error(), "value: 1") {
		t.Fatal("Error unexpectedly includes source snippet")
	}
	if !strings.Contains(validation.FormatWithSource(), "value: 1") {
		t.Fatal("explicit source formatter omitted YAML")
	}
}

func TestHighLevelMapScopeConjunction(t *testing.T) {
	type config struct {
		Entries map[string]string `yaml:"entries" yamlvalidate:"properties={known={pattern='^k'}},values={minLength=2},additional={pattern='^a'}"`
	}
	var got config
	if err := Unmarshal([]byte("entries:\n  known: keep\n  other: apple\n"), &got); err != nil {
		t.Fatalf("declared property should not be checked by additional: %v", err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("entries:\n  known: k\n"), &got), "min_length")
	requireDiagnosticCode(t, Unmarshal([]byte("entries:\n  known: keep\n  other: bad\n"), &got), "pattern")
	type closed struct {
		Entries map[string]string `yaml:"entries" yamlvalidate:"properties={known={type=string}},additional=forbid,values={minLength=2}"`
	}
	var c closed
	if err := Unmarshal([]byte("entries:\n  known: ab\n"), &c); err != nil {
		t.Fatalf("closed known property: %v", err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("entries:\n  other: ab\n"), &c), "unknown_key")
	requireDiagnosticCode(t, Unmarshal([]byte("entries:\n  known: a\n"), &c), "min_length")
}

func TestHighLevelVisitBudgetFailsClosedAfterWarning(t *testing.T) {
	type config struct {
		Name  string `yaml:"name" yamlvalidate:"deprecated"`
		Count int    `yaml:"count"`
	}
	got := config{Name: "unchanged"}
	err := (UnmarshalOptions{Limits: Limits{MaxNodeVisits: 10}}).Unmarshal([]byte("name: old\ncount: 1\n"), &got)
	var all *ValidationErrors
	if !errors.As(err, &all) || !all.Truncated() {
		t.Fatalf("expected truncated validation failure, got %v", err)
	}
	if len(all.Diagnostics()) != 1 || all.Diagnostics()[0].Level != LevelWarning {
		t.Fatalf("expected warning-only incomplete result, got %#v", all.Diagnostics())
	}
	if !strings.Contains(all.Error(), "incomplete") {
		t.Fatalf("truncated error does not indicate incomplete validation: %v", all)
	}
	if got.Name != "unchanged" {
		t.Fatalf("destination changed after incomplete validation: %#v", got)
	}
}

func TestBoundedMergeExpansionStopsBeforeBlowup(t *testing.T) {
	var source strings.Builder
	source.WriteString("a: &a {x: 1}\n")
	for i := 0; i < 12; i++ {
		previous := string(rune('a' + i))
		current := string(rune('b' + i))
		source.WriteString(current + ": &" + current + " {<<: [*" + previous + ", *" + previous + "]}\n")
	}
	source.WriteString("root: {<<: [*m, *m]}\n")
	root, err := parseSingleDocument([]byte(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := NewValidationContext()
	ctx.MaxNodeVisits = 30
	var target *yaml.Node
	for i := 0; i+1 < len(root.Content[0].Content); i += 2 {
		if root.Content[0].Content[i].Value == "root" {
			target = root.Content[0].Content[i+1]
		}
	}
	if target == nil {
		t.Fatal("missing root")
	}
	_ = expandMappingWithMergesBounded(target, ctx)
	if !ctx.limitReached {
		t.Fatal("merge expansion did not consume visit budget")
	}
}

func TestHighLevelPinnedCodecSpecialTypes(t *testing.T) {
	type config struct {
		Duration time.Duration `yaml:"duration"`
		When     time.Time     `yaml:"when"`
		Bytes    []byte        `yaml:"bytes"`
		Number   **int         `yaml:"number"`
	}
	value := 42
	inner := &value
	input := config{Duration: time.Second + 2*time.Millisecond, When: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), Bytes: []byte{1, 2, 3}, Number: &inner}
	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal special types: %v", err)
	}
	var got config
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal special types: %v; YAML %q", err, data)
	}
	if got.Duration != input.Duration || !got.When.Equal(input.When) || string(got.Bytes) != string(input.Bytes) || got.Number == nil || *got.Number == nil || **got.Number != 42 {
		t.Fatalf("roundtrip mismatch: %#v vs %#v", got, input)
	}
}

func TestHighLevelCustomCodecRequiresOpaqueDeclarationAndRunsOnce(t *testing.T) {
	type unknown struct {
		Value highLevelCustomText `yaml:"value"`
	}
	var invalid unknown
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("value: x\n"), &invalid); !errors.As(err, &schemaErr) {
		t.Fatalf("custom codec without binding accepted: %v", err)
	}
	type decoded struct {
		Value highLevelCustomText `yaml:"value" yamlvalidate:"type=any"`
	}
	count := 0
	got := decoded{Value: highLevelCustomText{Calls: &count}}
	if err := Unmarshal([]byte("value: x\n"), &got); err != nil {
		t.Fatal(err)
	}
	if count != 1 || got.Value.Value != "x" {
		t.Fatalf("custom Unmarshal count/value = %d/%q", count, got.Value.Value)
	}
	type encoded struct {
		Value highLevelCustomOutput `yaml:"value" yamlvalidate:"type=any"`
	}
	count = 0
	data, err := Marshal(encoded{Value: highLevelCustomOutput{Calls: &count}})
	if err != nil || count != 1 || !strings.Contains(string(data), "encoded") {
		t.Fatalf("custom Marshal = %q, %v, calls %d", data, err, count)
	}
}

func TestHighLevelNonemptyAcceptsNilCapableDeclarationAndRejectsNull(t *testing.T) {
	type config struct {
		Items  []string          `yaml:"items" yamlvalidate:"nonempty"`
		Labels map[string]string `yaml:"labels" yamlvalidate:"nonempty"`
	}
	var got config
	if err := Unmarshal([]byte("items: [x]\nlabels: {a: b}\n"), &got); err != nil {
		t.Fatalf("nonempty slice/map declaration rejected: %v", err)
	}
	requireDiagnosticCode(t, Unmarshal([]byte("items: null\nlabels: {a: b}\n"), &got), "nonempty")
	requireDiagnosticCode(t, Unmarshal([]byte("items: []\nlabels: {}\n"), &got), "nonempty")
}
