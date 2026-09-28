package yamlvalidator

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrJSONSchemaResourceNotFound allows resolver chains to distinguish a miss
// from an actual resolver failure.
var ErrJSONSchemaResourceNotFound = errors.New("JSON Schema resource not found")

// ErrJSONSchemaResourceOutsideRoot is returned by JSONSchemaFileResolver when
// a file URL escapes its configured root.
var ErrJSONSchemaResourceOutsideRoot = errors.New("JSON Schema resource is outside resolver root")

// JSONSchemaResolver resolves an absolute JSON Schema retrieval URL.
// Implementations must be safe for concurrent use if a compile configuration is
// reused concurrently.
type JSONSchemaResolver interface {
	Resolve(ctx context.Context, url string) ([]byte, error)
}

// JSONSchemaResolverFunc adapts a function to JSONSchemaResolver.
type JSONSchemaResolverFunc func(context.Context, string) ([]byte, error)

// Resolve implements JSONSchemaResolver.
func (f JSONSchemaResolverFunc) Resolve(ctx context.Context, url string) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("JSON Schema resolver function is nil")
	}
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f(ctx, url)
}

// JSONSchemaResourceMap is an in-memory resolver keyed by absolute retrieval URL.
// Treat the map as immutable while it is in use; resolved byte slices are copied.
type JSONSchemaResourceMap map[string][]byte

// Resolve implements JSONSchemaResolver.
func (m JSONSchemaResourceMap) Resolve(ctx context.Context, resourceURL string) ([]byte, error) {
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := m[resourceURL]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
	}
	return append([]byte(nil), data...), nil
}

// JSONSchemaResolverChain tries resolvers in order. Only
// ErrJSONSchemaResourceNotFound advances to the next resolver. Treat the slice
// as immutable while it is in use.
type JSONSchemaResolverChain []JSONSchemaResolver

// Resolve implements JSONSchemaResolver.
func (chain JSONSchemaResolverChain) Resolve(ctx context.Context, resourceURL string) ([]byte, error) {
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, resolver := range chain {
		if resolver == nil {
			continue
		}
		data, err := resolver.Resolve(ctx, resourceURL)
		switch {
		case err == nil:
			return data, nil
		case errors.Is(err, ErrJSONSchemaResourceNotFound):
			continue
		default:
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
}

// JSONSchemaCachingResolver caches successful resolutions.
// Returned byte slices are defensive copies.
type JSONSchemaCachingResolver struct {
	resolver JSONSchemaResolver

	mu    sync.RWMutex
	cache map[string][]byte
}

// NewJSONSchemaCachingResolver wraps resolver with a concurrency-safe cache.
func NewJSONSchemaCachingResolver(resolver JSONSchemaResolver) *JSONSchemaCachingResolver {
	return &JSONSchemaCachingResolver{
		resolver: resolver,
		cache:    make(map[string][]byte),
	}
}

// Resolve implements JSONSchemaResolver.
func (resolver *JSONSchemaCachingResolver) Resolve(
	ctx context.Context,
	resourceURL string,
) ([]byte, error) {
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if resolver == nil || resolver.resolver == nil {
		return nil, fmt.Errorf("JSON Schema caching resolver has no underlying resolver")
	}

	resolver.mu.RLock()
	cached, ok := resolver.cache[resourceURL]
	resolver.mu.RUnlock()
	if ok {
		return append([]byte(nil), cached...), nil
	}

	data, err := resolver.resolver.Resolve(ctx, resourceURL)
	if err != nil {
		return nil, err
	}
	snapshot := append([]byte(nil), data...)
	resolver.mu.Lock()
	if resolver.cache == nil {
		resolver.cache = make(map[string][]byte)
	}
	resolver.cache[resourceURL] = snapshot
	resolver.mu.Unlock()
	return append([]byte(nil), snapshot...), nil
}

// JSONSchemaFileResolver resolves file:// resources confined to one directory.
// The root must exist when the resolver is created.
type JSONSchemaFileResolver struct {
	root string
}

// NewJSONSchemaFileResolver creates a file resolver constrained to root.
func NewJSONSchemaFileResolver(root string) (*JSONSchemaFileResolver, error) {
	if root == "" {
		return nil, fmt.Errorf("JSON Schema file resolver root must not be empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve JSON Schema file resolver root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve JSON Schema file resolver root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat JSON Schema file resolver root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("JSON Schema file resolver root %q is not a directory", root)
	}
	return &JSONSchemaFileResolver{root: filepath.Clean(resolved)}, nil
}

// Resolve implements JSONSchemaResolver.
func (resolver *JSONSchemaFileResolver) Resolve(
	ctx context.Context,
	resourceURL string,
) ([]byte, error) {
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if resolver == nil || resolver.root == "" {
		return nil, fmt.Errorf("JSON Schema file resolver is not initialized")
	}

	parsed, err := url.Parse(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("parse JSON Schema resource URL %q: %w", resourceURL, err)
	}
	if parsed.Scheme != "file" {
		return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return nil, fmt.Errorf("unsupported file URL host %q", parsed.Host)
	}

	escapedPath := parsed.EscapedPath()
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return nil, fmt.Errorf("decode JSON Schema file URL %q: %w", resourceURL, err)
	}
	target := filepath.Clean(filepath.FromSlash(decodedPath))
	if !filepath.IsAbs(target) {
		return nil, fmt.Errorf("JSON Schema file URL %q is not absolute", resourceURL)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
		}
		return nil, fmt.Errorf("resolve JSON Schema resource symlinks %q: %w", resourceURL, err)
	}
	if !pathWithinRoot(resolver.root, resolvedTarget) {
		return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceOutsideRoot, resourceURL)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolvedTarget)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
		}
		return nil, fmt.Errorf("read JSON Schema resource %q: %w", resourceURL, err)
	}
	return data, nil
}

func pathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func jsonSchemaResolverContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

type jsonSchemaNoExternalResolver struct{}

func (jsonSchemaNoExternalResolver) Resolve(
	ctx context.Context,
	resourceURL string,
) ([]byte, error) {
	ctx = jsonSchemaResolverContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%w: %s", ErrJSONSchemaResourceNotFound, resourceURL)
}
