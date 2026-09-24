package yamlvalidator_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestJSONSchemaResourceMap(t *testing.T) {
	resolver := JSONSchemaResourceMap{
		"https://example.test/schema.json": []byte(`{"type":"string"}`),
	}

	first, err := resolver.Resolve(context.Background(), "https://example.test/schema.json")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	first[0] = 'X'
	second, err := resolver.Resolve(context.Background(), "https://example.test/schema.json")
	if err != nil {
		t.Fatalf("resolve again: %v", err)
	}
	if string(second) != `{"type":"string"}` {
		t.Fatalf("resolver returned mutable shared bytes: %q", second)
	}

	_, err = resolver.Resolve(context.Background(), "https://example.test/missing.json")
	if !errors.Is(err, ErrJSONSchemaResourceNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestJSONSchemaResolverChainAndCache(t *testing.T) {
	var calls atomic.Int32
	base := JSONSchemaResolverFunc(func(_ context.Context, resourceURL string) ([]byte, error) {
		calls.Add(1)
		if resourceURL != "https://example.test/hit.json" {
			return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
		}
		return []byte(`{"type":"integer"}`), nil
	})
	cache := NewJSONSchemaCachingResolver(base)
	chain := JSONSchemaResolverChain{
		JSONSchemaResourceMap{},
		cache,
	}

	for range 2 {
		data, err := chain.Resolve(context.Background(), "https://example.test/hit.json")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if string(data) != `{"type":"integer"}` {
			t.Fatalf("unexpected data: %s", data)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying resolver called %d times, want 1", got)
	}
}
func TestJSONSchemaFileResolver(t *testing.T) {
	root := t.TempDir()
	childPath := filepath.Join(root, "child.json")
	if err := os.WriteFile(childPath, []byte(`{"type":"integer","minimum":2}`), 0o600); err != nil {
		t.Fatal(err)
	}

	resolver, err := NewJSONSchemaFileResolver(root)
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}
	rootURL := (&url.URL{Scheme: "file", Path: filepath.Join(root, "root.json")}).String()
	schema, err := CompileJSONSchemaContextWithOptions(
		context.Background(),
		[]byte(`{"$ref":"child.json"}`),
		JSONSchemaCompileOptions{
			SchemaURL: rootURL,
			Resolver:  resolver,
		},
	)
	if err != nil {
		t.Fatalf("compile with file resolver: %v", err)
	}
	if result := NewValidator(schema).ValidateBytes([]byte("2")); result.HasErrors() {
		t.Fatalf("valid value rejected: %v", result.Collector.Errors())
	}
	if result := NewValidator(schema).ValidateBytes([]byte("1")); !result.HasErrors() {
		t.Fatal("invalid value accepted")
	}
}

func TestJSONSchemaFileResolverRejectsEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.json")
	if err := os.WriteFile(outside, []byte(`true`), 0o600); err != nil {
		t.Fatal(err)
	}

	resolver, err := NewJSONSchemaFileResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	outsideURL := (&url.URL{Scheme: "file", Path: outside}).String()
	_, err = resolver.Resolve(context.Background(), outsideURL)
	if !errors.Is(err, ErrJSONSchemaResourceOutsideRoot) {
		t.Fatalf("expected root escape error, got %v", err)
	}
}

func TestCompileJSONSchemaContextCancelsResolver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	resolver := JSONSchemaResolverFunc(func(ctx context.Context, _ string) ([]byte, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})

	_, err := CompileJSONSchemaContextWithOptions(
		ctx,
		[]byte(`{"$ref":"https://example.test/external.json"}`),
		JSONSchemaCompileOptions{Resolver: resolver},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
func TestJSONSchemaResolverObservesCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	resolver := JSONSchemaResolverFunc(func(context.Context, string) ([]byte, error) {
		called = true
		return nil, nil
	})
	_, err := resolver.Resolve(ctx, "https://example.test/schema.json")
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("resolver func should reject before callback: err=%v called=%v", err, called)
	}

	_, err = (JSONSchemaResourceMap{}).Resolve(
		ctx,
		"https://example.test/schema.json",
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled resource-map lookup, got %v", err)
	}
}

func TestJSONSchemaCachingResolverDoesNotCacheErrors(t *testing.T) {
	var calls atomic.Int32
	resolver := NewJSONSchemaCachingResolver(JSONSchemaResolverFunc(
		func(context.Context, string) ([]byte, error) {
			n := calls.Add(1)
			if n == 1 {
				return nil, ErrJSONSchemaResourceNotFound
			}
			return []byte(`true`), nil
		},
	))

	if _, err := resolver.Resolve(context.Background(), "https://example.test/x"); err == nil {
		t.Fatal("expected first miss")
	}
	if _, err := resolver.Resolve(context.Background(), "https://example.test/x"); err != nil {
		t.Fatalf("second resolution should succeed: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying calls=%d, want 2", got)
	}
}

func TestCompileJSONSchemaContextAlreadyExpired(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := CompileJSONSchemaContext(ctx, []byte(`true`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestCompileJSONSchemaDoesNotReadFilesWithoutResolver(t *testing.T) {
	root := t.TempDir()
	childPath := filepath.Join(root, "child.json")
	if err := os.WriteFile(childPath, []byte(`{"type":"integer"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rootURL := (&url.URL{Scheme: "file", Path: filepath.Join(root, "root.json")}).String()

	_, err := CompileJSONSchemaWithOptions(
		[]byte(`{"$ref":"child.json"}`),
		JSONSchemaCompileOptions{SchemaURL: rootURL},
	)
	if err == nil {
		t.Fatal("external file reference must require an explicit resolver")
	}
	if !strings.Contains(err.Error(), ErrJSONSchemaResourceNotFound.Error()) {
		t.Fatalf("unexpected closed-world resolution error: %v", err)
	}
}
