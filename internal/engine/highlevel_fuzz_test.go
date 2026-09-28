package yamlvalidator

import (
	"reflect"
	"testing"
)

func FuzzHighLevelTagGrammar(f *testing.F) {
	for _, seed := range []string{"required,min=1", "when={field=mode,eq='a,b',require=[tls]}", "x=[a,{b=2}]", "x='bad\\q'"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 20<<10 {
			return
		}
		_, _ = parseTagRules(input)
	})
}

func FuzzHighLevelOuterTag(f *testing.F) {
	for _, seed := range []string{"yaml:\"name\" yamlvalidate:\"required\"", "yamlvalidate:\"min=1", "bad tag"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 20<<10 {
			return
		}
		_, _ = parseOuterStructTag(input)
	})
}

func FuzzHighLevelYAML(f *testing.F) {
	for _, seed := range []string{"name: ok\n", "name: &x ok\ncopy: *x\n", "---\n\n---\n", "'<<': literal\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 64<<10 {
			return
		}
		var out map[string]any
		_ = (UnmarshalOptions{Limits: Limits{MaxBytes: 64 << 10, MaxNodeVisits: 10000}}).Unmarshal([]byte(input), &out)
	})
}

func FuzzHighLevelReflectShape(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(1))
	f.Add(uint8(2))
	f.Add(uint8(3))
	f.Fuzz(func(t *testing.T, selector uint8) {
		types := []reflect.Type{reflect.TypeFor[int](), reflect.TypeFor[map[string]int](), reflect.TypeFor[[]string](), reflect.TypeFor[struct {
			A string `yaml:"a"`
		}]()}
		_, _ = compileHighLevel(types[int(selector)%len(types)], false, nil)
	})
}
