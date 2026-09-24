package yamlvalidator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"gopkg.in/yaml.v3"
)

// JSONSchemaDraft selects the default JSON Schema dialect used when a schema
// does not declare $schema. An explicit $schema in the document takes precedence.
type JSONSchemaDraft string

const (
	JSONSchemaDraft4    JSONSchemaDraft = "draft-04"
	JSONSchemaDraft6    JSONSchemaDraft = "draft-06"
	JSONSchemaDraft7    JSONSchemaDraft = "draft-07"
	JSONSchemaDraft2019 JSONSchemaDraft = "2019-09"
	JSONSchemaDraft2020 JSONSchemaDraft = "2020-12"
)

// JSONSchemaCompileOptions controls JSON Schema compilation and validation.
type JSONSchemaCompileOptions struct {
	// SchemaURL is the retrieval URL of the root schema. It is used as the base
	// URI for relative references when the schema does not establish another base
	// with $id. The default is an in-memory HTTPS URL.
	SchemaURL string

	// DefaultDraft is used only when the schema does not contain $schema.
	// The default is draft 2020-12.
	DefaultDraft JSONSchemaDraft

	// AssertFormat forces format to be an assertion even in dialects where it is
	// annotation-only by default.
	AssertFormat bool

	// AssertContent enables contentEncoding/contentMediaType/contentSchema assertions.
	AssertContent bool

	// AssertVocabs requires vocabularies declared by a metaschema to be known.
	AssertVocabs bool

	// RegexpTimeout bounds one ECMAScript regexp match. Zero preserves the
	// regexp engine's unlimited default. Use a positive duration for untrusted schemas.
	RegexpTimeout time.Duration

	// Formats registers first-class functional custom formats.
	Formats []JSONSchemaFormat

	// ContentEncodings registers custom contentEncoding decoders.
	ContentEncodings []JSONSchemaContentEncoding

	// ContentMediaTypes registers custom contentMediaType validators.
	ContentMediaTypes []JSONSchemaContentMediaType

	// Keywords registers functional custom keywords in an internal vocabulary.
	// They are activated for this compilation automatically.
	Keywords []JSONSchemaKeyword

	// Vocabularies registers named functional custom vocabularies. They are
	// activated for this compilation automatically.
	Vocabularies []JSONSchemaVocabulary

	// Resources preloads external resources addressed by their retrieval URLs.
	// This is the preferred way to compile a closed schema graph without I/O.
	Resources map[string][]byte

	// Resolver resolves unresolved external references. If nil, compilation is
	// closed-world: only the root schema, Resources, and built-in metaschemas are
	// available. No filesystem or network I/O is performed implicitly.
	Resolver JSONSchemaResolver

	// AdditionalPropertiesFalsePolicy optionally downgrades direct
	// additionalProperties:false diagnostics. This is a presentation policy and
	// intentionally does not rewrite JSON Schema combinator semantics.
	AdditionalPropertiesFalsePolicy *UnknownKeyPolicy
}

// CompileJSONSchema compiles a JSON Schema using draft 2020-12 as the default
// dialect. Schemas declaring $schema are compiled using their declared dialect.
func CompileJSONSchema(data []byte) (*FieldSchema, error) {
	return CompileJSONSchemaContextWithOptions(
		context.Background(),
		data,
		JSONSchemaCompileOptions{},
	)
}

// CompileJSONSchemaWithOptions compiles a standards-compliant JSON Schema and
// returns a FieldSchema adapter that validates YAML through the JSON Schema engine
// while preserving YAML source positions in diagnostics.
func CompileJSONSchemaWithOptions(data []byte, opts JSONSchemaCompileOptions) (*FieldSchema, error) {
	return CompileJSONSchemaContextWithOptions(context.Background(), data, opts)
}

// CompileJSONSchemaContext is the context-aware form of CompileJSONSchema.
// Context cancellation is observed by library-controlled compilation work and
// by JSONSchemaResolver implementations.
func CompileJSONSchemaContext(ctx context.Context, data []byte) (*FieldSchema, error) {
	return CompileJSONSchemaContextWithOptions(ctx, data, JSONSchemaCompileOptions{})
}

// CompileJSONSchemaContextWithOptions is the context-aware form of
// CompileJSONSchemaWithOptions.
func CompileJSONSchemaContextWithOptions(
	ctx context.Context,
	data []byte,
	opts JSONSchemaCompileOptions,
) (*FieldSchema, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JSON Schema: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(func(pattern string) (jsonschema.Regexp, error) {
		return compileECMAScriptRegexpWithTimeout(pattern, opts.RegexpTimeout)
	})
	draft, err := resolveJSONSchemaDraft(opts.DefaultDraft)
	if err != nil {
		return nil, err
	}
	compiler.DefaultDraft(draft)
	if opts.AssertFormat {
		compiler.AssertFormat()
	}
	if opts.AssertContent {
		compiler.AssertContent()
	}
	if opts.AssertVocabs {
		compiler.AssertVocabs()
	}

	resolver := opts.Resolver
	if resolver == nil {
		resolver = jsonSchemaNoExternalResolver{}
	}
	compiler.UseLoader(jsonSchemaResolverLoader{
		ctx:      ctx,
		resolver: resolver,
	})

	registerExtendedJSONSchemaFormats(compiler)
	if err := registerFunctionalFormats(compiler, opts.Formats); err != nil {
		return nil, err
	}
	if err := registerJSONSchemaContentExtensions(
		compiler,
		opts.ContentEncodings,
		opts.ContentMediaTypes,
	); err != nil {
		return nil, err
	}

	resourceURLs := make([]string, 0, len(opts.Resources))
	for resourceURL := range opts.Resources {
		resourceURLs = append(resourceURLs, resourceURL)
	}
	sort.Strings(resourceURLs)
	for _, resourceURL := range resourceURLs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(opts.Resources[resourceURL]))
		if err != nil {
			return nil, fmt.Errorf("decode JSON Schema resource %q: %w", resourceURL, err)
		}
		if err := compiler.AddResource(resourceURL, resource); err != nil {
			return nil, fmt.Errorf("add JSON Schema resource %q: %w", resourceURL, err)
		}
	}

	if err := registerFunctionalJSONSchemaExtensions(
		compiler,
		opts.Keywords,
		opts.Vocabularies,
	); err != nil {
		return nil, err
	}

	rootURL := opts.SchemaURL
	if rootURL == "" {
		rootURL = "https://yamlvalidator.local/schema.json"
	}
	if err := compiler.AddResource(rootURL, doc); err != nil {
		return nil, fmt.Errorf("add root JSON Schema: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile(rootURL)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("compile JSON Schema: %w", err)
	}

	var additionalPropertiesPolicy *UnknownKeyPolicy
	if opts.AdditionalPropertiesFalsePolicy != nil {
		policy := *opts.AdditionalPropertiesFalsePolicy
		additionalPropertiesPolicy = &policy
	}

	return &FieldSchema{
		Type: TypeAny,
		Validators: []ValueValidator{
			&compiledJSONSchemaValidator{
				schema:                          compiled,
				additionalPropertiesFalsePolicy: additionalPropertiesPolicy,
			},
		},
	}, nil
}

type ecmaRegexp struct {
	source string
	re     *regexp2.Regexp
}

func compileECMAScriptRegexpWithTimeout(pattern string, timeout time.Duration) (jsonschema.Regexp, error) {
	if timeout < 0 {
		return nil, fmt.Errorf("regexp timeout must be non-negative")
	}
	if err := rejectNonECMAScriptRegexpExtensions(pattern); err != nil {
		return nil, err
	}
	translated := normalizeECMAScriptUnicodeProperties(pattern)
	re, err := regexp2.Compile(translated, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		re.MatchTimeout = timeout
	}
	return &ecmaRegexp{source: pattern, re: re}, nil
}

func rejectNonECMAScriptRegexpExtensions(pattern string) error {
	if strings.Contains(pattern, "(?#") {
		return fmt.Errorf("inline comment groups are not valid ECMAScript regular expressions")
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			run := 1
			for i+run < len(pattern) && pattern[i+run] == '\\' {
				run++
			}
			if run%2 == 1 && i+run < len(pattern) && pattern[i+run] == 'a' {
				return fmt.Errorf(`\\a is not a valid ECMAScript control escape`)
			}
			i += run - 1
			continue
		}
		if i+3 < len(pattern) && pattern[i] == '(' && pattern[i+1] == '?' {
			j := i + 2
			seenFlag := false
			for j < len(pattern) && (pattern[j] == 'i' || pattern[j] == 'm' || pattern[j] == 's' || pattern[j] == '-') {
				if pattern[j] != '-' {
					seenFlag = true
				}
				j++
			}
			if seenFlag && j < len(pattern) && pattern[j] == ')' {
				return fmt.Errorf("global inline regexp flags are not valid ECMAScript syntax")
			}
		}
	}
	return nil
}

func normalizeECMAScriptUnicodeProperties(pattern string) string {
	// regexp2 uses Unicode category names while ECMAScript accepts the long
	// General_Category aliases exercised by the JSON Schema test suite.
	pattern = strings.ReplaceAll(pattern, `\p{Letter}`, `\p{L}`)
	pattern = strings.ReplaceAll(pattern, `\p{digit}`, `\p{Nd}`)
	return pattern
}

func (r *ecmaRegexp) String() string {
	return r.source
}

func (r *ecmaRegexp) MatchString(value string) bool {
	matched, err := r.re.MatchString(value)
	return err == nil && matched
}

func resolveJSONSchemaDraft(draft JSONSchemaDraft) (*jsonschema.Draft, error) {
	switch draft {
	case "", JSONSchemaDraft2020:
		return jsonschema.Draft2020, nil
	case JSONSchemaDraft2019:
		return jsonschema.Draft2019, nil
	case JSONSchemaDraft7:
		return jsonschema.Draft7, nil
	case JSONSchemaDraft6:
		return jsonschema.Draft6, nil
	case JSONSchemaDraft4:
		return jsonschema.Draft4, nil
	default:
		return nil, fmt.Errorf("unsupported default JSON Schema draft %q", draft)
	}
}

type jsonSchemaResolverLoader struct {
	ctx      context.Context
	resolver JSONSchemaResolver
}

func (loader jsonSchemaResolverLoader) Load(resourceURL string) (any, error) {
	if err := loader.ctx.Err(); err != nil {
		return nil, err
	}
	data, err := loader.resolver.Resolve(loader.ctx, resourceURL)
	if err != nil {
		return nil, err
	}
	if err := loader.ctx.Err(); err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}

type compiledJSONSchemaValidator struct {
	schema                          *jsonschema.Schema
	additionalPropertiesFalsePolicy *UnknownKeyPolicy
}

func (v *compiledJSONSchemaValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if ctx.IsStopped() {
		return
	}
	instance, index, conversionErr := yamlNodeToJSONSchemaInstance(node, path, ctx)
	if ctx.IsStopped() {
		return
	}
	if conversionErr != nil {
		ctx.AddError(*conversionErr)
		return
	}

	err := v.schema.Validate(instance)
	if ctx.IsStopped() {
		return
	}
	if err == nil {
		return
	}
	validationErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "json_schema",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: err.Error(),
		})
		return
	}

	v.addValidationError(validationErr, path, index, ctx)
}

func (v *compiledJSONSchemaValidator) addValidationError(err *jsonschema.ValidationError, basePath string,
	index *yamlJSONInstanceIndex, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}

	keyword := jsonSchemaErrorKeyword(err)
	if keyword != "" {
		if v.addSpecializedError(err, keyword, basePath, index, ctx) {
			return
		}
		message := jsonSchemaErrorMessage(err, keyword)
		location := jsonSchemaErrorInstanceLocation(err)
		node := index.closestValueNode(location)
		ctx.AddError(ValidationError{
			Level:      v.levelForKeyword(keyword),
			Code:       keyword,
			SchemaPath: jsonSchemaErrorSchemaPath(err),
			Path:       index.displayPath(basePath, location),
			Line:       nodeLine(node),
			Column:     nodeColumn(node),
			Message:    message,
			Details:    jsonSchemaErrorDetails(err),
		})
	}

	for _, cause := range err.Causes {
		v.addValidationError(cause, basePath, index, ctx)
	}
}

func (v *compiledJSONSchemaValidator) addSpecializedError(err *jsonschema.ValidationError, keyword, basePath string,
	index *yamlJSONInstanceIndex, ctx *ValidationContext) bool {

	switch k := err.ErrorKind.(type) {
	case *kind.Required:
		for _, missing := range k.Missing {
			parent := index.valueNode(err.InstanceLocation)
			ctx.AddError(ValidationError{
				Level:      LevelError,
				Code:       keyword,
				SchemaPath: jsonSchemaErrorSchemaPath(err),
				Path:       index.displayPath(basePath, appendPath(err.InstanceLocation, missing)),
				Line:       nodeLine(parent),
				Column:     nodeColumn(parent),
				Message:    fmt.Sprintf("required property %q is missing", missing),
				Details:    RequiredDetails{Field: missing},
			})
		}
		return true
	case *kind.DependentRequired:
		for _, missing := range k.Missing {
			parent := index.valueNode(err.InstanceLocation)
			ctx.AddError(ValidationError{
				Level:      LevelError,
				Code:       keyword,
				SchemaPath: jsonSchemaErrorSchemaPath(err),
				Path:       index.displayPath(basePath, appendPath(err.InstanceLocation, missing)),
				Line:       nodeLine(parent),
				Column:     nodeColumn(parent),
				Message:    fmt.Sprintf("property %q is required when %q is present", missing, k.Prop),
				Details: DependencyDetails{
					Field:   missing,
					Trigger: k.Prop,
				},
			})
		}
		return true
	case *kind.Dependency:
		for _, missing := range k.Missing {
			parent := index.valueNode(err.InstanceLocation)
			ctx.AddError(ValidationError{
				Level:      LevelError,
				Code:       keyword,
				SchemaPath: jsonSchemaErrorSchemaPath(err),
				Path:       index.displayPath(basePath, appendPath(err.InstanceLocation, missing)),
				Line:       nodeLine(parent),
				Column:     nodeColumn(parent),
				Message:    fmt.Sprintf("property %q is required when %q is present", missing, k.Prop),
				Details: DependencyDetails{
					Field:   missing,
					Trigger: k.Prop,
				},
			})
		}
		return true
	case *kind.AdditionalProperties:
		if v.additionalPropertiesFalsePolicy != nil && *v.additionalPropertiesFalsePolicy == UnknownKeyIgnore {
			return true
		}
		for _, property := range k.Properties {
			location := appendPath(err.InstanceLocation, property)
			node := index.keyNode(location)
			if node == nil {
				node = index.valueNode(location)
			}
			ctx.AddError(ValidationError{
				Level:      v.levelForKeyword(keyword),
				Code:       keyword,
				SchemaPath: jsonSchemaErrorSchemaPath(err),
				Path:       index.displayPath(basePath, location),
				Line:       nodeLine(node),
				Column:     nodeColumn(node),
				Message:    fmt.Sprintf("additional property %q is not allowed", property),
				Details:    UnknownKeyDetails{Key: property},
			})
		}
		return true
	}
	return false
}

func (v *compiledJSONSchemaValidator) levelForKeyword(keyword string) ErrorLevel {
	if keyword == "additionalProperties" && v.additionalPropertiesFalsePolicy != nil {
		switch *v.additionalPropertiesFalsePolicy {
		case UnknownKeyWarn:
			return LevelWarning
		}
	}
	return LevelError
}

func jsonSchemaErrorKeyword(err *jsonschema.ValidationError) string {
	path := err.ErrorKind.KeywordPath()
	if len(path) > 0 {
		return path[0]
	}
	switch err.ErrorKind.(type) {
	case *kind.Not:
		return "not"
	case *kind.FalseSchema:
		return "falseSchema"
	case *kind.InvalidJsonValue:
		return "invalidJsonValue"
	case *kind.RefCycle:
		return "$ref"
	case *kind.Group:
		return "group"
	case *kind.Schema:
		return ""
	default:
		return "jsonSchema"
	}
}

func jsonSchemaErrorMessage(err *jsonschema.ValidationError, keyword string) string {
	output := err.BasicOutput()
	if output.Error != nil {
		return output.Error.String()
	}
	return fmt.Sprintf("JSON Schema %s validation failed", keyword)
}

func jsonSchemaErrorDetails(err *jsonschema.ValidationError) any {
	switch k := err.ErrorKind.(type) {
	case *kind.Type:
		return TypeMismatchDetails{
			Expected: append([]string(nil), k.Want...),
			Actual:   k.Got,
		}
	case *kind.MinItems:
		return ItemCountDetails{Actual: k.Got, Bound: k.Want}
	case *kind.MaxItems:
		return ItemCountDetails{Actual: k.Got, Bound: k.Want}
	case *kind.Minimum:
		return NumericRangeDetails{Value: k.Got.RatString(), Bound: k.Want.RatString()}
	case *kind.Maximum:
		return NumericRangeDetails{Value: k.Got.RatString(), Bound: k.Want.RatString()}
	case *kind.ExclusiveMinimum:
		return NumericRangeDetails{Value: k.Got.RatString(), Bound: k.Want.RatString()}
	case *kind.ExclusiveMaximum:
		return NumericRangeDetails{Value: k.Got.RatString(), Bound: k.Want.RatString()}
	default:
		return nil
	}
}

func jsonSchemaErrorInstanceLocation(err *jsonschema.ValidationError) []string {
	location := append([]string(nil), err.InstanceLocation...)
	if functional, ok := err.ErrorKind.(*functionalKeywordError); ok {
		location = append(location, functional.relativePath...)
	}
	return location
}

func jsonSchemaErrorSchemaPath(err *jsonschema.ValidationError) string {
	if err.SchemaURL == "" {
		return ""
	}
	parts := err.ErrorKind.KeywordPath()
	if len(parts) == 0 {
		return err.SchemaURL
	}
	base := strings.TrimSuffix(err.SchemaURL, "/")
	for _, part := range parts {
		base += "/" + escapeJSONPointerToken(part)
	}
	return base
}

func appendPath(path []string, segment string) []string {
	result := make([]string, 0, len(path)+1)
	result = append(result, path...)
	result = append(result, segment)
	return result
}

type yamlJSONInstanceIndex struct {
	values map[string]*yaml.Node
	keys   map[string]*yaml.Node
}

func newYAMLJSONInstanceIndex() *yamlJSONInstanceIndex {
	return &yamlJSONInstanceIndex{
		values: make(map[string]*yaml.Node),
		keys:   make(map[string]*yaml.Node),
	}
}

func (i *yamlJSONInstanceIndex) valueNode(path []string) *yaml.Node {
	return i.values[jsonPointer(path)]
}

func (i *yamlJSONInstanceIndex) closestValueNode(path []string) *yaml.Node {
	for end := len(path); end >= 0; end-- {
		if node := i.valueNode(path[:end]); node != nil {
			return node
		}
	}
	return nil
}

func (i *yamlJSONInstanceIndex) keyNode(path []string) *yaml.Node {
	return i.keys[jsonPointer(path)]
}

func (i *yamlJSONInstanceIndex) displayPath(base string, segments []string) string {
	path := base
	parentPath := make([]string, 0, len(segments))
	for _, segment := range segments {
		parent := i.valueNode(parentPath)
		for parent != nil && parent.Kind == yaml.AliasNode && parent.Alias != nil {
			parent = parent.Alias
		}
		if parent != nil && parent.Kind == yaml.SequenceNode {
			path += "[" + segment + "]"
		} else if isSimplePathSegment(segment) && !(path == "" && segment == "doc") {
			if path != "" {
				path += "."
			}
			path += segment
		} else {
			encoded, _ := json.Marshal(segment)
			path += "[" + string(encoded) + "]"
		}
		parentPath = append(parentPath, segment)
	}
	return cleanPath(path)
}

func yamlNodeToJSONSchemaInstance(node *yaml.Node, basePath string, ctx *ValidationContext) (any, *yamlJSONInstanceIndex, *ValidationError) {
	index := newYAMLJSONInstanceIndex()
	value, err := convertYAMLNodeToJSON(node, nil, basePath, ctx, index, make(map[*yaml.Node]bool), 1)
	return value, index, err
}

func convertYAMLNodeToJSON(node *yaml.Node, location []string, basePath string, ctx *ValidationContext,
	index *yamlJSONInstanceIndex, visiting map[*yaml.Node]bool, depth int) (any, *ValidationError) {

	if ctx != nil && ctx.IsStopped() {
		return nil, nil
	}
	if node == nil {
		return nil, nil
	}
	if ctx != nil && ctx.MaxDepth > 0 && depth > ctx.MaxDepth {
		return nil, &ValidationError{
			Level:    LevelError,
			Code:     "max_depth",
			Path:     joinJSONSchemaInstancePath(basePath, location),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "YAML value exceeds configured nesting depth",
			Expected: fmt.Sprintf("depth <= %d", ctx.MaxDepth),
			Got:      fmt.Sprintf("depth %d", depth),
		}
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return convertYAMLNodeToJSON(node.Content[0], location, basePath, ctx, index, visiting, depth)
	}
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return nil, jsonConversionError(node, basePath, location, "unresolved YAML alias")
		}
		if visiting[node.Alias] {
			return nil, jsonConversionError(node, basePath, location, "recursive YAML alias cannot be represented as a JSON instance")
		}
		return convertYAMLNodeToJSON(node.Alias, location, basePath, ctx, index, visiting, depth)
	}

	index.values[jsonPointer(location)] = node
	visiting[node] = true
	defer delete(visiting, node)

	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]any)
		for _, pair := range expandMappingWithMerges(node) {
			keyNode := pair.key
			if !yamlKeyIsJSONString(keyNode) {
				return nil, jsonConversionError(keyNode, basePath, location, "JSON Schema object keys must be strings")
			}
			key := keyNode.Value
			childLocation := appendPath(location, key)
			index.keys[jsonPointer(childLocation)] = keyNode
			value, err := convertYAMLNodeToJSON(pair.value, childLocation, basePath, ctx, index, visiting, depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for idx, child := range node.Content {
			childLocation := appendPath(location, strconv.Itoa(idx))
			value, err := convertYAMLNodeToJSON(child, childLocation, basePath, ctx, index, visiting, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		return yamlScalarToJSON(node, ctx, basePath, location)
	default:
		return nil, jsonConversionError(node, basePath, location, fmt.Sprintf("unsupported YAML node kind %d", node.Kind))
	}
}

func yamlKeyIsJSONString(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}
	return node.Tag == "" || node.Tag == "!!str" || !strings.HasPrefix(node.Tag, "!!")
}

func yamlScalarToJSON(node *yaml.Node, ctx *ValidationContext, basePath string, location []string) (any, *ValidationError) {
	switch node.Tag {
	case "!!null":
		return nil, nil
	case "!!bool":
		value, err := strconv.ParseBool(strings.ToLower(node.Value))
		if err != nil {
			return nil, jsonConversionError(node, basePath, location, "invalid YAML boolean")
		}
		return value, nil
	case "!!int":
		value, err := normalizeYAMLInteger(node.Value)
		if err != nil {
			return nil, jsonConversionError(node, basePath, location, err.Error())
		}
		return json.Number(value), nil
	case "!!float":
		lower := strings.ToLower(strings.ReplaceAll(node.Value, "_", ""))
		if lower == ".nan" || lower == ".inf" || lower == "+.inf" || lower == "-.inf" {
			return nil, jsonConversionError(node, basePath, location, "non-finite YAML numbers cannot be represented by the JSON data model")
		}
		if _, ok := new(big.Rat).SetString(lower); !ok {
			return nil, jsonConversionError(node, basePath, location, "invalid YAML number for JSON Schema validation")
		}
		return json.Number(lower), nil
	case "!!str", "":
		if ctx != nil && ctx.YAML11Booleans && node.Style == 0 {
			if value, ok := yaml11Boolean(node.Value); ok {
				return value, nil
			}
		}
		return node.Value, nil
	default:
		// JSON has no tag system. Unknown/custom tagged scalars are represented by
		// their scalar text, which is the least lossy JSON-compatible representation.
		return node.Value, nil
	}
}

func yaml11Boolean(value string) (bool, bool) {
	switch strings.ToLower(value) {
	case "y", "yes", "true", "on":
		return true, true
	case "n", "no", "false", "off":
		return false, true
	default:
		return false, false
	}
}

func jsonConversionError(node *yaml.Node, basePath string, location []string, message string) *ValidationError {
	return &ValidationError{
		Level:   LevelError,
		Code:    "json_instance_conversion",
		Path:    joinJSONSchemaInstancePath(basePath, location),
		Line:    nodeLine(node),
		Column:  nodeColumn(node),
		Message: message,
	}
}

func nodeLine(node *yaml.Node) int {
	if node == nil {
		return 0
	}
	return node.Line
}

func nodeColumn(node *yaml.Node) int {
	if node == nil {
		return 0
	}
	return node.Column
}

func jsonPointer(path []string) string {
	if len(path) == 0 {
		return ""
	}
	var b strings.Builder
	for _, segment := range path {
		b.WriteByte('/')
		b.WriteString(escapeJSONPointerToken(segment))
	}
	return b.String()
}

func escapeJSONPointerToken(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func joinJSONSchemaInstancePath(base string, segments []string) string {
	path := base
	for _, segment := range segments {
		if _, err := strconv.Atoi(segment); err == nil {
			path += "[" + segment + "]"
			continue
		}
		if isSimplePathSegment(segment) && !(path == "" && segment == "doc") {
			if path != "" {
				path += "."
			}
			path += segment
			continue
		}
		encoded, _ := json.Marshal(segment)
		path += "[" + string(encoded) + "]"
	}
	return cleanPath(path)
}

func isSimplePathSegment(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
