package yamlvalidator

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type highLevelZeroValue struct{ Name string }

func (v highLevelZeroValue) IsZero() bool { return v.Name == "omit" }

type highLevelCyclicZero struct {
	Next  *highLevelCyclicZero
	Calls *int
}

func (v *highLevelCyclicZero) IsZero() bool {
	*v.Calls++
	return true
}

func TestHighLevelMarshalHonorsCustomIsZero(t *testing.T) {
	type config struct {
		Value highLevelZeroValue `yaml:"value,omitempty"`
	}
	data, err := Marshal(config{Value: highLevelZeroValue{Name: "omit"}})
	if err != nil || strings.Contains(string(data), "value:") {
		t.Fatalf("custom IsZero omission: %q, %v", data, err)
	}
}

func TestHighLevelCyclePreflightDoesNotInvokeIsZero(t *testing.T) {
	type config struct {
		Value any `yaml:"value,omitempty"`
	}
	calls := 0
	cyclic := &highLevelCyclicZero{Calls: &calls}
	cyclic.Next = cyclic
	if _, err := Marshal(config{Value: cyclic}); err == nil || calls != 0 {
		t.Fatalf("preflight must reject cycle before user hook; calls=%d err=%v", calls, err)
	}
}

func TestHighLevelScalarSurface(t *testing.T) {
	type config struct {
		Name     string  `yaml:"name" yamlvalidate:"required,minLength=2,maxLength=8,pattern='^[a-z]+$',description='service name'"`
		Mode     string  `yaml:"mode" yamlvalidate:"enum=['',auto],deprecated='use profile'"`
		URL      string  `yaml:"url" yamlvalidate:"url={requireScheme=true,schemes=[https]}"`
		Host     *string `yaml:"host" yamlvalidate:"format=hostname"`
		Scale    float64 `yaml:"scale" yamlvalidate:"min=0.25,max=1.5"`
		Fallback *string `yaml:"fallback" yamlvalidate:"default=null"`
	}
	var got config
	var diagnostics []ValidationError
	opts := UnmarshalOptions{OnDiagnostic: func(d ValidationError) { diagnostics = append(diagnostics, d) }}
	if err := opts.Unmarshal([]byte("name: api\nmode: ''\nurl: https://example.com\nhost: null\nscale: 1.25\n"), &got); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 || diagnostics[0].Level != LevelWarning || diagnostics[1].Level != LevelWarning {
		t.Fatalf("expected deprecated and explicit-null default warnings: %#v", diagnostics)
	}
	for _, source := range []string{"name: A\nurl: https://example.com\n", "name: abcdefghi\nurl: https://example.com\n", "name: api\nurl: http://example.com\n", "name: api\nurl: https://example.com\nhost: invalid host\n", "name: api\nurl: https://example.com\nscale: 2\n"} {
		if err := Unmarshal([]byte(source), &got); err == nil {
			t.Errorf("invalid scalar accepted: %q", source)
		}
	}
}

func TestHighLevelNullTypeNarrowsNullablePointer(t *testing.T) {
	type config struct {
		Value *string `yaml:"value" yamlvalidate:"type=null"`
	}
	var got config
	if err := Unmarshal([]byte("value: null\n"), &got); err != nil {
		t.Fatalf("nullable pointer cannot be constrained to null: %v", err)
	}
	if err := Unmarshal([]byte("value: text\n"), &got); !diagnosticCode(err, "type_mismatch") {
		t.Fatalf("non-null value accepted: %v", err)
	}
}

func TestHighLevelObjectRuleSurface(t *testing.T) {
	type config struct {
		File   string            `yaml:"file" yamlvalidate:"anyOfRequired=[[file],[host,port]],oneOfRequired=[[file],[host,port]],forbiddenTogether=[[file,port]],dependentRequired={host=[port]},when={field=mode,eq=prod,forbid=[debug]}"`
		Host   string            `yaml:"host"`
		Port   int               `yaml:"port"`
		Mode   string            `yaml:"mode"`
		Debug  bool              `yaml:"debug"`
		Labels map[string]string `yaml:"labels" yamlvalidate:"minProperties=1,maxProperties=2,keys={minLength=2,maxLength=8,pattern='^[a-z]+$'},values={minLength=2},properties={good={pattern='^o'}},additional={pattern='^a'}"`
	}
	var got config
	if err := Unmarshal([]byte("file: x\nlabels: {good: ok}\n"), &got); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"labels: {good: ok}\n", "file: x\nport: 2\nlabels: {good: ok}\n", "host: x\nlabels: {good: ok}\n", "file: x\nmode: prod\ndebug: true\nlabels: {good: ok}\n", "file: x\nlabels: {X: aa}\n", "file: x\nlabels: {good: no}\n", "file: x\nlabels: {good: ok, other: bad}\n"} {
		if err := Unmarshal([]byte(source), &got); err == nil {
			t.Errorf("invalid object rule accepted: %q", source)
		}
	}
}

func TestHighLevelCompositionAndRegistryErrors(t *testing.T) {
	r, err := NewRegistry(RegistryConfig{Schemas: map[string]*FieldSchema{"text": {Type: TypeString}, "number": {Type: TypeInt}}})
	if err != nil {
		t.Fatal(err)
	}
	type config struct {
		Choice   any    `yaml:"choice" yamlvalidate:"oneOfSchemas=[text,number]"`
		Fallback any    `yaml:"fallback" yamlvalidate:"anyOfSchemas=[{type=string},{type=int}]"`
		Name     string `yaml:"name" yamlvalidate:"minLength=3"`
		Raw      string `yaml:"raw" yamlvalidate:"pattern='^x'"`
	}
	var got config
	opts := UnmarshalOptions{Registry: r}
	if err := opts.Unmarshal([]byte("choice: hello\nfallback: 2\nname: long\nraw: xyz\n"), &got); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"choice: []\nfallback: 2\nname: long\nraw: xyz\n", "choice: hello\nfallback: []\nname: long\nraw: xyz\n", "choice: hello\nfallback: 2\nname: x\nraw: xyz\n", "choice: hello\nfallback: 2\nname: long\nraw: bad\n"} {
		if err := opts.Unmarshal([]byte(source), &got); err == nil {
			t.Errorf("invalid composition accepted: %q", source)
		}
	}
	type badRef struct {
		Value string `yaml:"value" yamlvalidate:"ref=missing"`
	}
	var bad badRef
	var schemaErr *SchemaError
	if err := (UnmarshalOptions{Registry: r}).Unmarshal([]byte("value: x"), &bad); !errors.As(err, &schemaErr) {
		t.Errorf("unknown ref: %v", err)
	}
}

func TestHighLevelMalformedDeclarationTable(t *testing.T) {
	cases := []struct {
		kind reflect.Type
		tag  string
	}{
		{reflect.TypeFor[string](), "pattern='["}, {reflect.TypeFor[string](), "url={requireScheme=maybe}"}, {reflect.TypeFor[string](), "url={schemes=[1bad]}"},
		{reflect.TypeFor[string](), "enum=[]"}, {reflect.TypeFor[string](), "minLength=5,maxLength=1"}, {reflect.TypeFor[string](), "default={x=1,x=2}"},
		{reflect.TypeFor[map[string]string](), "keys={minLength=5,maxLength=1}"}, {reflect.TypeFor[[]int](), "minItems=5,maxItems=1"},
		{reflect.TypeFor[int](), "min=9,max=1"}, {reflect.TypeFor[int](), "nonempty"}, {reflect.TypeFor[string](), "check=missing"},
		{reflect.TypeFor[string](), "required=1"}, {reflect.TypeFor[string](), "unknown_rule"},
	}
	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: tc.kind, Tag: reflect.StructTag("yaml:\"value\" yamlvalidate:" + strconv.Quote(tc.tag))}})
			dst := reflect.New(typ).Interface()
			var schemaErr *SchemaError
			if err := Unmarshal([]byte("{}"), dst); !errors.As(err, &schemaErr) {
				t.Fatalf("accepted bad tag %q: %v", tc.tag, err)
			}
		})
	}
}

func TestHighLevelResourceAndOperationalEdges(t *testing.T) {
	type config struct {
		Value int `yaml:"value" yamlvalidate:"min=1"`
	}
	for _, opts := range []UnmarshalOptions{{Limits: Limits{MaxBytes: -1}}, {UnknownKeyPolicy: UnknownKeyPolicy(100)}} {
		var got config
		if err := opts.Unmarshal([]byte("value: 2"), &got); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	var got config
	if err := Unmarshal([]byte("value: 2"), got); err == nil {
		t.Fatal("nonpointer destination accepted")
	}
	if err := (UnmarshalOptions{Limits: Limits{MaxBytes: 3}}).Unmarshal([]byte("value: 2"), &got); err == nil {
		t.Fatal("byte limit ignored")
	}
	if err := (UnmarshalOptions{SkipValidation: true}).Unmarshal([]byte("value: 0"), &got); err != nil || got.Value != 0 {
		t.Fatalf("skip validation: %#v %v", got, err)
	}
	if _, err := (MarshalOptions{Indent: -1}).Marshal(config{Value: 1}); err == nil {
		t.Fatal("negative indent accepted")
	}
	if _, err := (MarshalOptions{Limits: Limits{MaxBytes: 2}}).Marshal(config{Value: 1}); err == nil {
		t.Fatal("marshal byte limit ignored")
	}
	if err := Unmarshal([]byte("value: 0\n"), &got); !diagnosticCode(err, "min") {
		t.Fatalf("native error missing: %v", err)
	}
	var aggregate *ValidationErrors
	err := Unmarshal([]byte("value: 0\n"), &got)
	if !errors.As(err, &aggregate) || strings.Contains(aggregate.Error(), "value: 0") {
		t.Fatalf("diagnostic source leaked: %v", err)
	}
	err = (UnmarshalOptions{Limits: Limits{MaxNodeVisits: 1}}).Unmarshal([]byte("value: 2\n"), &got)
	if !errors.As(err, &aggregate) || !aggregate.Truncated() || !strings.Contains(aggregate.Error(), "incomplete") {
		t.Fatalf("visit limit did not report incomplete validation: %v", err)
	}
}
