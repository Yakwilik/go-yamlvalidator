package recipes_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestJSONSchemaMemoryResources(t *testing.T) {
	options := v.JSONSchemaCompileOptions{
		SchemaURL: "https://schemas.example.test/root.json",
		Resources: map[string][]byte{"https://schemas.example.test/common.json": []byte(`{"$defs":{"name":{"type":"string","minLength":2}}}`)},
	}
	validator := compileJSON(t, `{"$ref":"common.json#/$defs/name"}`, options)
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "api", valid: true}, {name: "invalid", input: "a", code: "minLength"}})
	if _, err := v.CompileJSONSchema([]byte(`{"$ref":"https://schemas.example.test/missing"}`)); err == nil {
		t.Fatal("unexpected implicit resource access")
	}
}

func TestJSONSchemaResolverChainAndCache(t *testing.T) {
	calls := 0
	fallback := v.JSONSchemaResolverFunc(func(ctx context.Context, uri string) ([]byte, error) {
		calls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if uri != "https://schemas.example.test/remote" {
			return nil, fmt.Errorf("%w: %s", v.ErrJSONSchemaResourceNotFound, uri)
		}
		return []byte(`{"type":"integer","minimum":1}`), nil
	})
	cache := v.NewJSONSchemaCachingResolver(v.JSONSchemaResolverChain{
		v.JSONSchemaResourceMap{"https://schemas.example.test/local": []byte(`{"type":"string"}`)}, fallback,
	})
	for range 2 {
		validator := compileJSON(t, `{"$ref":"https://schemas.example.test/remote"}`, v.JSONSchemaCompileOptions{Resolver: cache})
		checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "1", valid: true}, {name: "invalid", input: "0", code: "minimum"}})
	}
	if calls != 1 {
		t.Fatalf("sequential cache calls=%d, want 1", calls)
	}
	payload, err := cache.Resolve(context.Background(), "https://schemas.example.test/local")
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	again, err := cache.Resolve(context.Background(), "https://schemas.example.test/local")
	if err != nil {
		t.Fatal(err)
	}
	if again[0] != '{' {
		t.Fatal("cache leaked mutable slice")
	}
}

func TestJSONSchemaFileResolver(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "schemas")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(root, "common.json")
	if err := os.WriteFile(common, []byte(`{"type":"string"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := v.NewJSONSchemaFileResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	rootURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(root, "root.json"))}).String()
	validator := compileJSON(t, `{"$ref":"common.json"}`, v.JSONSchemaCompileOptions{SchemaURL: rootURI, Resolver: resolver})
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{{name: "valid", input: "api", valid: true}, {name: "invalid", input: "1", code: "type"}})
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte(`true`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Resolve(context.Background(), (&url.URL{Scheme: "file", Path: filepath.ToSlash(outside)}).String())
	if !errors.Is(err, v.ErrJSONSchemaResourceOutsideRoot) {
		t.Fatalf("expected root boundary, got %v", err)
	}
}

func TestJSONSchemaContextCompilation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.CompileJSONSchemaContext(ctx, []byte(`true`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	schema, err := v.CompileJSONSchemaContextWithOptions(context.Background(), []byte(`{"$ref":"urn:example:name"}`), v.JSONSchemaCompileOptions{
		Resolver: v.JSONSchemaResourceMap{"urn:example:name": []byte(`{"type":"string"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkInputs(t, v.NewValidator(schema), v.ValidationContext{}, []inputCase{{name: "context compilation", input: "api", valid: true}})
}
