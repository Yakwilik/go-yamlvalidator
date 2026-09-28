package benchmarks

import (
	"fmt"
	"testing"

	yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/benchmarks/model"
	"gopkg.in/yaml.v3"
)

type runtimeConfig struct {
	Name        string            `yaml:"name" yamlvalidate:"required,nonempty,minLength=3"`
	Environment string            `yaml:"environment" yamlvalidate:"required,enum=[dev,stage,prod]"`
	Server      runtimeServer     `yaml:"server" yamlvalidate:"required"`
	Services    []runtimeService  `yaml:"services" yamlvalidate:"required,minItems=1"`
	Labels      map[string]string `yaml:"labels,omitempty" yamlvalidate:"keys={pattern='^[a-z][a-z0-9_-]*$'},values={nonempty}"`
	Features    map[string]bool   `yaml:"features,omitempty"`
}

type runtimeServer struct {
	Host string      `yaml:"host" yamlvalidate:"required,nonempty,format=hostname"`
	Port int         `yaml:"port" yamlvalidate:"required,min=1,max=65535"`
	TLS  *runtimeTLS `yaml:"tls,omitempty"`
}

type runtimeTLS struct {
	Cert string `yaml:"cert" yamlvalidate:"required,nonempty"`
	Key  string `yaml:"key" yamlvalidate:"required,nonempty"`
}

type runtimeService struct {
	Name      string            `yaml:"name" yamlvalidate:"required,nonempty,pattern='^[a-z][a-z0-9-]*$'"`
	Replicas  int               `yaml:"replicas" yamlvalidate:"min=1,max=1000"`
	Endpoints []runtimeEndpoint `yaml:"endpoints" yamlvalidate:"required,minItems=1"`
}

type runtimeEndpoint struct {
	Path      string `yaml:"path" yamlvalidate:"required,nonempty,pattern='^/'"`
	TimeoutMS int    `yaml:"timeout_ms" yamlvalidate:"min=1,max=60000"`
}

type fixture struct {
	data      []byte
	runtime   runtimeConfig
	generated model.Config
	node      *yaml.Node
}

type benchmarkSize struct {
	name      string
	services  int
	endpoints int
}

var benchmarkSizes = []benchmarkSize{
	{name: "small", services: 1, endpoints: 2},
	{name: "medium", services: 20, endpoints: 4},
	{name: "large", services: 100, endpoints: 5},
}

var sinkBytes []byte
var sinkNode *yaml.Node
var sinkRuntime runtimeConfig
var sinkGenerated model.Config

func makeFixture(tb testing.TB, services, endpoints int) fixture {
	tb.Helper()
	runtimeValue := runtimeConfig{
		Name:        "benchmark",
		Environment: "prod",
		Server: runtimeServer{
			Host: "api.example.com",
			Port: 8443,
			TLS:  &runtimeTLS{Cert: "certificate-data", Key: "private-key-data"},
		},
		Labels: map[string]string{
			"app": "benchmark", "team": "platform", "region": "eu-central",
			"owner": "backend", "tier": "api",
		},
		Features: map[string]bool{"cache": true, "metrics": true, "tracing": true},
	}
	for i := 0; i < services; i++ {
		service := runtimeService{Name: fmt.Sprintf("service-%d", i), Replicas: i%8 + 1}
		for j := 0; j < endpoints; j++ {
			service.Endpoints = append(service.Endpoints, runtimeEndpoint{
				Path: fmt.Sprintf("/api/%d/%d", i, j), TimeoutMS: 1000 + j*250,
			})
		}
		runtimeValue.Services = append(runtimeValue.Services, service)
	}
	data, err := yaml.Marshal(runtimeValue)
	if err != nil {
		tb.Fatal(err)
	}
	var generatedValue model.Config
	if err := yamlvalidator.Unmarshal(data, &generatedValue); err != nil {
		tb.Fatal(err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		tb.Fatal(err)
	}
	return fixture{data: data, runtime: runtimeValue, generated: generatedValue, node: root.Content[0]}
}

func benchmarkDecode(b *testing.B, f fixture, name string, fn func([]byte) error) {
	b.Helper()
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(f.data)))
		b.ResetTimer()
		for range b.N {
			if err := fn(f.data); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkUnmarshal(b *testing.B) {
	for _, size := range benchmarkSizes {
		f := makeFixture(b, size.services, size.endpoints)
		b.Run(size.name, func(b *testing.B) {
			benchmarkDecode(b, f, "yaml_v3_runtime", func(data []byte) error {
				var value runtimeConfig
				err := yaml.Unmarshal(data, &value)
				sinkRuntime = value
				return err
			})
			benchmarkDecode(b, f, "validator_runtime", func(data []byte) error {
				var value runtimeConfig
				err := yamlvalidator.Unmarshal(data, &value)
				sinkRuntime = value
				return err
			})
			benchmarkDecode(b, f, "validator_generated", func(data []byte) error {
				var value model.Config
				err := yamlvalidator.Unmarshal(data, &value)
				sinkGenerated = value
				return err
			})
			benchmarkDecode(b, f, "yaml_v3_generated_hook", func(data []byte) error {
				var value model.Config
				err := yaml.Unmarshal(data, &value)
				sinkGenerated = value
				return err
			})
		})
	}
}

func BenchmarkMarshal(b *testing.B) {
	for _, size := range benchmarkSizes {
		f := makeFixture(b, size.services, size.endpoints)
		b.Run(size.name, func(b *testing.B) {
			b.Run("yaml_v3_runtime", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					data, err := yaml.Marshal(f.runtime)
					if err != nil {
						b.Fatal(err)
					}
					sinkBytes = data
				}
			})
			b.Run("validator_runtime", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					data, err := yamlvalidator.Marshal(f.runtime)
					if err != nil {
						b.Fatal(err)
					}
					sinkBytes = data
				}
			})
			b.Run("validator_generated", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					data, err := yamlvalidator.Marshal(f.generated)
					if err != nil {
						b.Fatal(err)
					}
					sinkBytes = data
				}
			})
			b.Run("yaml_v3_generated_hook", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					data, err := yaml.Marshal(f.generated)
					if err != nil {
						b.Fatal(err)
					}
					sinkBytes = data
				}
			})
		})
	}
}

func BenchmarkNodeCodec(b *testing.B) {
	for _, size := range benchmarkSizes {
		f := makeFixture(b, size.services, size.endpoints)
		b.Run(size.name, func(b *testing.B) {
			b.Run("decode_yaml_node_runtime", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var value runtimeConfig
					if err := f.node.Decode(&value); err != nil {
						b.Fatal(err)
					}
					sinkRuntime = value
				}
			})
			b.Run("decode_generated", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var value model.Config
					if err := value.YAMLValidatorDecode(f.node); err != nil {
						b.Fatal(err)
					}
					sinkGenerated = value
				}
			})
			b.Run("encode_yaml_node_runtime", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var node yaml.Node
					if err := node.Encode(f.runtime); err != nil {
						b.Fatal(err)
					}
					sinkNode = &node
				}
			})
			b.Run("encode_generated", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					node, err := f.generated.YAMLValidatorEncode()
					if err != nil {
						b.Fatal(err)
					}
					sinkNode = node
				}
			})
		})
	}
}
