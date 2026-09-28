package yamlvalidator

import (
	genspec "github.com/Yakwilik/go-yamlvalidator/internal/genspec"
	"reflect"
	"sync/atomic"
	"testing"
)

type generatedPlanCacheProbe struct{}

var generatedPlanCacheCalls atomic.Int64

func (generatedPlanCacheProbe) YAMLValidatorGeneratedType() reflect.Type {
	return reflect.TypeFor[generatedPlanCacheProbe]()
}

func (generatedPlanCacheProbe) YAMLValidatorSchemaSpec() (genspec.Graph, []reflect.Type) {
	generatedPlanCacheCalls.Add(1)
	return genspec.Graph{Nodes: []genspec.Node{{Type: genspec.NodeType(TypeMap), HasAllowedKeys: true}}}, nil
}

func TestGeneratedPlanUsesRegistryCache(t *testing.T) {
	generatedPlanCacheCalls.Store(0)
	registry, err := NewRegistry(RegistryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	value := generatedPlanCacheProbe{}
	for range 3 {
		if _, generated, err := generatedPlan(value, registry, false); err != nil || !generated {
			t.Fatalf("decode plan: generated=%t err=%v", generated, err)
		}
	}
	if got := generatedPlanCacheCalls.Load(); got != 1 {
		t.Fatalf("decode schema built %d times, want 1", got)
	}
	for range 3 {
		if _, generated, err := generatedPlan(value, registry, true); err != nil || !generated {
			t.Fatalf("encode plan: generated=%t err=%v", generated, err)
		}
	}
	if got := generatedPlanCacheCalls.Load(); got != 2 {
		t.Fatalf("encode schema built %d total times, want 2", got)
	}
}
