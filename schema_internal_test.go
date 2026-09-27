package yamlvalidator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFieldSchemaInvalidDefinitions(t *testing.T) {
	negative := -1
	one, zero := 1, 0
	invalid := []struct {
		name   string
		schema *FieldSchema
	}{
		{"nil", nil},
		{"invalid type", &FieldSchema{Type: NodeType(99)}},
		{"duplicate allowed type", &FieldSchema{AllowedTypes: []NodeType{TypeString, TypeString}}},
		{"invalid unknown policy", &FieldSchema{Type: TypeMap, UnknownKeyPolicy: UnknownKeyPolicy(99)}},
		{"negative min items", &FieldSchema{Type: TypeSequence, MinItems: &negative}},
		{"negative max items", &FieldSchema{Type: TypeSequence, MaxItems: &negative}},
		{"inverted items", &FieldSchema{Type: TypeSequence, MinItems: &one, MaxItems: &zero}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFieldSchema(tc.schema); err == nil {
				t.Fatal("expected schema definition error")
			}
		})
	}
}

func TestValidateFieldSchemaStructuralErrors(t *testing.T) {
	recursive := &FieldSchema{Type: TypeMap}
	recursive.AllowedKeys = map[string]*FieldSchema{"self": recursive}

	invalid := []struct {
		name   string
		schema *FieldSchema
	}{
		{"mapping constraint on string", &FieldSchema{
			Type:        TypeString,
			AllowedKeys: map[string]*FieldSchema{},
		}},
		{"sequence constraint on string", &FieldSchema{
			Type:       TypeString,
			ItemSchema: &FieldSchema{Type: TypeString},
		}},
		{"nil allowed child", &FieldSchema{
			Type:        TypeMap,
			AllowedKeys: map[string]*FieldSchema{"child": nil},
		}},
		{"nil one-of schema", &FieldSchema{Type: TypeAny, OneOfSchemas: []*FieldSchema{nil}}},
		{"nil any-of schema", &FieldSchema{Type: TypeAny, AnyOfSchemas: []*FieldSchema{nil}}},
		{"empty any-of group", &FieldSchema{Type: TypeMap, AnyOf: [][]string{{}}}},
		{"recursive graph", recursive},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFieldSchema(tc.schema); err == nil {
				t.Fatal("expected schema definition error")
			}
		})
	}
}

func TestValidateFieldSchemaReferenceErrors(t *testing.T) {
	allowed := map[string]*FieldSchema{
		"a": {Type: TypeString},
		"b": {Type: TypeString},
	}
	cases := []struct {
		name   string
		schema *FieldSchema
	}{
		{"undeclared exactly-one field", &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, ExactlyOneOf: []string{"missing"},
		}},
		{"duplicate mutually-exclusive field", &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, MutuallyExclusive: []string{"a", "a"},
		}},
		{"empty dependent trigger", &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, DependentRequired: map[string][]string{"": {"b"}},
		}},
		{"undeclared dependent trigger", &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, DependentRequired: map[string][]string{"missing": {"b"}},
		}},
		{"undeclared dependent field", &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, DependentRequired: map[string][]string{"a": {"missing"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFieldSchema(tc.schema); err == nil {
				t.Fatal("expected schema reference error")
			}
		})
	}
}

func TestValidateFieldSchemaConditionErrors(t *testing.T) {
	allowed := map[string]*FieldSchema{"enabled": {Type: TypeBool}, "name": {Type: TypeString}}
	cases := []ConditionalRule{
		{},
		{ConditionField: "missing", ConditionValue: "true"},
		{ConditionField: "enabled", ConditionValue: "true", ThenRequired: []string{"missing"}},
		{ConditionField: "enabled", ConditionValue: "true", ThenForbidden: []string{"missing"}},
	}
	for i, condition := range cases {
		schema := &FieldSchema{
			Type: TypeMap, AllowedKeys: allowed, Conditions: []ConditionalRule{condition},
		}
		if err := ValidateFieldSchema(schema); err == nil {
			t.Fatalf("condition %d unexpectedly accepted", i)
		}
	}
}

func TestSchemaTypeHelpers(t *testing.T) {
	if !schemaCanHaveType(&FieldSchema{Type: TypeAny}, TypeMap) {
		t.Fatal("TypeAny should allow map")
	}
	if !schemaCanHaveType(&FieldSchema{AllowedTypes: []NodeType{TypeString, TypeMap}}, TypeMap) {
		t.Fatal("allowed map type not recognized")
	}
	if schemaCanHaveType(&FieldSchema{AllowedTypes: []NodeType{TypeString}}, TypeMap) {
		t.Fatal("disallowed map type recognized")
	}
	if nilInterface(42) || !nilInterface((*FieldSchema)(nil)) {
		t.Fatal("nil interface helper mismatch")
	}
}
func TestSchemaCloneHelpers(t *testing.T) {
	groups := [][]string{{"a", "b"}}
	clonedGroups := cloneStringGroups(groups)
	groups[0][0] = "changed"
	if clonedGroups[0][0] != "a" {
		t.Fatal("cloneStringGroups shared storage")
	}
	if cloneStringGroups(nil) != nil {
		t.Fatal("nil string groups should remain nil")
	}

	sourceSlice := []any{"x", map[string]any{"n": 1}}
	clonedSlice := cloneSchemaDefault(sourceSlice).([]any)
	sourceSlice[0] = "changed"
	if clonedSlice[0] != "x" {
		t.Fatal("default slice clone shared storage")
	}

	sourceMap := map[string]any{"nested": map[string]any{"x": "y"}}
	clonedMap := cloneSchemaDefault(sourceMap).(map[string]any)
	sourceMap["nested"].(map[string]any)["x"] = "changed"
	if clonedMap["nested"].(map[string]any)["x"] != "y" {
		t.Fatal("default map clone shared storage")
	}

	mixed := map[any]any{"k": []any{"v"}}
	clonedMixed := cloneSchemaDefault(mixed).(map[any]any)
	mixed["k"].([]any)[0] = "changed"
	if clonedMixed["k"].([]any)[0] != "v" {
		t.Fatal("mixed default map clone shared storage")
	}
	if cloneSchemaDefault("scalar") != "scalar" {
		t.Fatal("scalar default changed")
	}
}
func TestJSONSchemaResourceResolvers(t *testing.T) {
	const resourceURL = "https://schemas.example.test/common.json"
	resource := JSONSchemaResourceMap{resourceURL: []byte("true")}

	data, err := resource.Resolve(nil, resourceURL)
	if err != nil || string(data) != "true" {
		t.Fatalf("resource resolve=%q,%v", data, err)
	}
	data[0] = 'X'
	again, err := resource.Resolve(context.Background(), resourceURL)
	if err != nil || string(again) != "true" {
		t.Fatal("resource map returned shared bytes")
	}
	if _, err := resource.Resolve(context.Background(), "https://schemas.example.test/missing.json"); !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("missing resource error=%v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resource.Resolve(canceled, resourceURL); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resource error=%v", err)
	}
}

func TestJSONSchemaResolverFuncAndChain(t *testing.T) {
	var nilResolver JSONSchemaResolverFunc
	if _, err := nilResolver.Resolve(nil, "x"); err == nil {
		t.Fatal("nil resolver function accepted")
	}

	hit := JSONSchemaResolverFunc(func(_ context.Context, resourceURL string) ([]byte, error) {
		if resourceURL != "hit" {
			return nil, ErrJSONSchemaResourceNotFound
		}
		return []byte("ok"), nil
	})
	chain := JSONSchemaResolverChain{nil, JSONSchemaResourceMap{}, hit}
	data, err := chain.Resolve(nil, "hit")
	if err != nil || string(data) != "ok" {
		t.Fatalf("chain resolve=%q,%v", data, err)
	}
	hard := errors.New("hard failure")
	chain = JSONSchemaResolverChain{
		JSONSchemaResolverFunc(func(context.Context, string) ([]byte, error) {
			return nil, hard
		}),
		hit,
	}
	if _, err := chain.Resolve(context.Background(), "hit"); !errors.Is(err, hard) {
		t.Fatalf("hard resolver error=%v", err)
	}
	if _, err := (JSONSchemaResolverChain{}).Resolve(nil, "missing"); !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("empty chain error=%v", err)
	}
}

func TestJSONSchemaCachingResolverBranches(t *testing.T) {
	if _, err := (*JSONSchemaCachingResolver)(nil).Resolve(nil, "x"); err == nil {
		t.Fatal("nil cache resolver accepted")
	}
	if _, err := NewJSONSchemaCachingResolver(nil).Resolve(nil, "x"); err == nil {
		t.Fatal("cache without resolver accepted")
	}

	calls := 0
	cache := NewJSONSchemaCachingResolver(JSONSchemaResolverFunc(
		func(context.Context, string) ([]byte, error) {
			calls++
			return []byte("cached"), nil
		},
	))
	first, err := cache.Resolve(nil, "x")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	second, err := cache.Resolve(nil, "x")
	if err != nil || string(second) != "cached" || calls != 1 {
		t.Fatalf("cache second=%q calls=%d err=%v", second, calls, err)
	}

	cache.cache = nil
	if _, err := cache.Resolve(nil, "other"); err != nil {
		t.Fatal(err)
	}
	if cache.cache == nil {
		t.Fatal("nil cache was not reinitialized")
	}
}
func TestJSONSchemaFileResolverBranches(t *testing.T) {
	if _, err := NewJSONSchemaFileResolver(""); err == nil {
		t.Fatal("empty root accepted")
	}
	if _, err := NewJSONSchemaFileResolver(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
	root := t.TempDir()
	plainFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(plainFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewJSONSchemaFileResolver(plainFile); err == nil {
		t.Fatal("file root accepted")
	}

	resolver, err := NewJSONSchemaFileResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(nil, "https://example.test/schema.json"); !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("non-file scheme error=%v", err)
	}
	if _, err := resolver.Resolve(nil, "file://remotehost/tmp/schema.json"); err == nil {
		t.Fatal("remote file host accepted")
	}
	if _, err := resolver.Resolve(nil, "file:///definitely/not/present/schema.json"); !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("missing file error=%v", err)
	}
}
func TestJSONSchemaFileResolverReadsWithinRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "schemas")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "common.json")
	if err := os.WriteFile(inside, []byte(`{"type":"string"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte("true"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewJSONSchemaFileResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := resolver.Resolve(nil, "file://"+filepath.ToSlash(inside))
	if err != nil || string(data) != `{"type":"string"}` {
		t.Fatalf("inside read=%q,%v", data, err)
	}
	if _, err := resolver.Resolve(nil, "file://"+filepath.ToSlash(outside)); !errors.Is(err, ErrJSONSchemaResourceOutsideRoot) {
		t.Fatalf("outside root error=%v", err)
	}
	if !pathWithinRoot(root, inside) || pathWithinRoot(root, outside) {
		t.Fatal("root containment mismatch")
	}
	if jsonSchemaResolverContext(nil) == nil {
		t.Fatal("nil resolver context not normalized")
	}
	if _, err := (jsonSchemaNoExternalResolver{}).Resolve(nil, "https://example.test/x"); !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("no-external error=%v", err)
	}
}
