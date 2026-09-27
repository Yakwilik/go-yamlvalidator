package yamlvalidator

import (
	"errors"
	"strings"
	"testing"
)

func diagnosticCode(err error, code string) bool {
	var aggregate *ValidationErrors
	if !errors.As(err, &aggregate) {
		return false
	}
	for _, item := range aggregate.Diagnostics() {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestHighLevelPresenceAndNull(t *testing.T) {
	type nullable struct {
		Value *int `yaml:"value" yamlvalidate:"required"`
	}
	for _, source := range []string{"value: null\n", "value: 0\n"} {
		var got nullable
		if err := Unmarshal([]byte(source), &got); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
	var got nullable
	if err := Unmarshal([]byte("{}\n"), &got); !diagnosticCode(err, "required") {
		t.Errorf("missing field: %v", err)
	}
	type notnull struct {
		Value *int `yaml:"value" yamlvalidate:"required,notnull"`
	}
	var strict notnull
	if err := Unmarshal([]byte("value: null\n"), &strict); !diagnosticCode(err, "type_mismatch") {
		t.Errorf("null accepted: %v", err)
	}
	type nonempty struct {
		Values []string `yaml:"values" yamlvalidate:"nonempty"`
	}
	var list nonempty
	if err := Unmarshal([]byte("values: []\n"), &list); !diagnosticCode(err, "nonempty") {
		t.Errorf("empty list accepted: %v", err)
	}
}

func TestHighLevelMapScopeAndUnknownPolicy(t *testing.T) {
	type config struct {
		Data map[string]int `yaml:"data" yamlvalidate:"properties={known={min=1}},values={max=10},additional=warn"`
	}
	var got config
	err := Unmarshal([]byte("data:\n  known: 12\n  extra: 13\n"), &got)
	if !diagnosticCode(err, "max") {
		t.Fatalf("values did not apply to declared and extra keys: %v", err)
	}
	if !diagnosticCode(err, "unknown_key") {
		t.Fatalf("additional=warn did not report unknown key: %v", err)
	}
	if err := Unmarshal([]byte("data:\n  extra: 4\n"), &got); err != nil {
		t.Fatalf("warning should not fail: %v", err)
	}
	var warning []ValidationError
	o := UnmarshalOptions{UnknownKeyPolicy: UnknownKeyIgnore, OnDiagnostic: func(d ValidationError) { warning = append(warning, d) }}
	if err := o.Unmarshal([]byte("data:\n  extra: 4\n"), &got); err != nil {
		t.Fatal(err)
	}
	if len(warning) == 0 {
		t.Fatal("explicit additional=warn must override option ignore")
	}
}

func TestHighLevelObjectGroupsAndAliases(t *testing.T) {
	type config struct {
		File  string `yaml:"file" yamlvalidate:"anyOfRequired=[[file],[host,port]],exactlyOneOf=[file,host],exactlyOneOf=[debug,quiet],mutuallyExclusive=[debug,quiet],forbiddenTogether=[[file,port]],dependentRequired={host=[port]},when={field=mode,eq=prod,require=[tls]}"`
		Host  string `yaml:"host"`
		Port  int    `yaml:"port"`
		Debug bool   `yaml:"debug"`
		Quiet bool   `yaml:"quiet"`
		Mode  string `yaml:"mode"`
		TLS   bool   `yaml:"tls"`
	}
	var got config
	err := Unmarshal([]byte("mode: prod\nhost: x\nport: 1\ndebug: true\ntls: true\n"), &got)
	if err != nil {
		t.Fatalf("valid groups: %v", err)
	}
	err = Unmarshal([]byte("file: &prod prod\nmode: *prod\nport: 2\nquiet: true\n"), &got)
	if !diagnosticCode(err, "forbidden_together") {
		t.Errorf("forbidden group: %v", err)
	}
}

func TestHighLevelStrictTypeAndSchemaErrors(t *testing.T) {
	type strict struct {
		Name   string `yaml:"name"`
		Number uint8  `yaml:"number"`
	}
	var got strict
	if err := Unmarshal([]byte("name: 123\nnumber: 1\n"), &got); !diagnosticCode(err, "type_mismatch") {
		t.Errorf("numeric string coercion: %v", err)
	}
	if err := Unmarshal([]byte("name: ok\nnumber: 256\n"), &got); !diagnosticCode(err, "representability") {
		t.Errorf("unsigned overflow: %v", err)
	}
	type malformed struct {
		Name string `yaml:"name" yamlvalidate:"min=oops"`
	}
	var bad malformed
	err := Unmarshal([]byte("name: ok\n"), &bad)
	var schemaErr *SchemaError
	if !errors.As(err, &schemaErr) {
		t.Errorf("malformed rule: %v", err)
	}
	type unknown struct {
		Name string `yaml:"name" yamlvalidate:"mystery"`
	}
	var u unknown
	if err := Unmarshal([]byte("name: ok\n"), &u); !errors.As(err, &schemaErr) {
		t.Errorf("unknown directive: %v", err)
	}
}

func TestHighLevelRejectsIncompatibleNullabilityAndUnion(t *testing.T) {
	type impossible struct {
		Count int `yaml:"count" yamlvalidate:"nullable"`
	}
	var a impossible
	var schemaErr *SchemaError
	if err := Unmarshal([]byte("count: 1\n"), &a); !errors.As(err, &schemaErr) {
		t.Errorf("nullable nonnil field accepted: %v", err)
	}
	type badUnion struct {
		Count int `yaml:"count" yamlvalidate:"types=[string,map]"`
	}
	var b badUnion
	if err := Unmarshal([]byte("count: 1\n"), &b); !errors.As(err, &schemaErr) {
		t.Errorf("incompatible union accepted: %v", err)
	}
}

func TestHighLevelMarshalRequiredOmitEmptyAndExactBytes(t *testing.T) {
	type item struct {
		Name string `yaml:"name,omitempty" yamlvalidate:"required"`
	}
	if data, err := Marshal(item{}); data != nil || !diagnosticCode(err, "required") {
		t.Fatalf("required omitted: %q %v", data, err)
	}
	data, err := Marshal(item{Name: "present"})
	if err != nil || !strings.Contains(string(data), "name: present") {
		t.Fatalf("marshal: %q %v", data, err)
	}
}

func TestHighLevelTagCollectionRules(t *testing.T) {
	type item struct {
		Labels map[string]string `yaml:"labels" yamlvalidate:"keys={pattern='^[a-z]+$',minLength=2},minProperties=1"`
		Names  []string          `yaml:"names" yamlvalidate:"minItems=1,maxItems=2,items={minLength=2,pattern='^[a-z]+$'},uniqueItems"`
	}
	var got item
	err := Unmarshal([]byte("labels:\n  A: x\nnames: [a, a]\n"), &got)
	for _, code := range []string{"key_pattern", "key_min_length", "min_length", "unique_items"} {
		if !diagnosticCode(err, code) {
			t.Errorf("missing %s in %v", code, err)
		}
	}
}
