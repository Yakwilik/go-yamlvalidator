package yamlvalidator

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

type benchmarkConfig struct {
	Name string `yaml:"name" yamlvalidate:"required,minLength=2"`
	Port int    `yaml:"port" yamlvalidate:"min=1,max=65535"`
}

var benchmarkYAML = []byte("name: service\nport: 8080\n")

func BenchmarkHighLevelCompileCold(b *testing.B) {
	typ := reflect.TypeFor[benchmarkConfig]()
	for i := 0; i < b.N; i++ {
		c := &highLevelCompiler{visiting: make(map[reflect.Type]bool)}
		if _, err := c.infer(typ); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkHighLevelCompileCacheHit(b *testing.B) {
	typ := reflect.TypeFor[benchmarkConfig]()
	if _, err := compileHighLevel(typ, false, nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := compileHighLevel(typ, false, nil); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkHighLevelUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var out benchmarkConfig
		if err := Unmarshal(benchmarkYAML, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkYAMLUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var out benchmarkConfig
		if err := yaml.Unmarshal(benchmarkYAML, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkValidateThenDecode(b *testing.B) {
	schema := &FieldSchema{Type: TypeMap, AllowedKeys: map[string]*FieldSchema{"name": {Type: TypeString, Required: true}, "port": {Type: TypeInt}}}
	v := NewValidator(schema)
	for i := 0; i < b.N; i++ {
		result := v.ValidateBytes(benchmarkYAML)
		if result.HasErrors() {
			b.Fatal(result.Collector.Errors())
		}
		var out benchmarkConfig
		if err := yaml.Unmarshal(benchmarkYAML, &out); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkHighLevelMarshal(b *testing.B) {
	in := benchmarkConfig{Name: "service", Port: 8080}
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(in); err != nil {
			b.Fatal(err)
		}
	}
}
