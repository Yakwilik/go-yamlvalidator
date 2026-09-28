// Package yamlvalidator validates YAML against native schemas and Go struct tags.
// Its public API is a facade over the shared internal validation engine.
package yamlvalidator

import (
	context "context"
	engine "github.com/Yakwilik/go-yamlvalidator/internal/engine"
	yaml "gopkg.in/yaml.v3"
)

// PathTokenKind identifies one component of a structured validation path.
type PathTokenKind = engine.PathTokenKind

const PathTokenProperty = engine.PathTokenProperty

const PathTokenIndex = engine.PathTokenIndex

// PathTokenDocument represents the reserved leading doc[N] prefix used for
// multi-document YAML diagnostics.
const PathTokenDocument = engine.PathTokenDocument

// PathToken is one typed component of ValidationError.Path.
type PathToken = engine.PathToken

// RequiredDetails describes a missing required field/property.
type RequiredDetails = engine.RequiredDetails

// UnknownKeyDetails describes an unknown mapping key and optional suggestion.
type UnknownKeyDetails = engine.UnknownKeyDetails

// TypeMismatchDetails describes a type validation failure.
type TypeMismatchDetails = engine.TypeMismatchDetails

// NumericRangeDetails describes a numeric bound failure.
type NumericRangeDetails = engine.NumericRangeDetails

// ItemCountDetails describes a sequence length failure.
type ItemCountDetails = engine.ItemCountDetails

// DependencyDetails describes a field required by another field.
type DependencyDetails = engine.DependencyDetails

// ParsePathTokens parses the library's display path syntax into typed tokens.
func ParsePathTokens(path string) ([]PathToken, error) { return engine.ParsePathTokens(path) }

// FormatPathTokens formats typed path tokens using the library's stable display syntax.
func FormatPathTokens(tokens []PathToken) string { return engine.FormatPathTokens(tokens) }

// AppendPropertyPath appends a mapping property to a stable display path.
// Ambiguous property names are quoted automatically.
func AppendPropertyPath(base, property string) string {
	return engine.AppendPropertyPath(base, property)
}

// AppendIndexPath appends a non-negative sequence index to a stable display
// path. It returns an error for negative indexes.
func AppendIndexPath(base string, index int) (string, error) {
	return engine.AppendIndexPath(base, index)
}

// Limits bounds high-level validation work. A zero field selects its default.
type Limits = engine.Limits

// SchemaError describes an invalid Go type, field, or validation declaration.
type SchemaError = engine.SchemaError

// ValidationErrors groups the final diagnostics from one high-level operation.
type ValidationErrors = engine.ValidationErrors

// UnmarshalOptions controls validated YAML decoding. Its zero value enables validation.
type UnmarshalOptions = engine.UnmarshalOptions

// MarshalOptions controls validated YAML encoding. Its zero value enables validation.
type MarshalOptions = engine.MarshalOptions

// Unmarshal decodes one YAML document after validating it.
func Unmarshal(data []byte, out any) error { return engine.Unmarshal(data, out) }

// Marshal encodes and validates a Go value, returning the exact validated bytes.
func Marshal(in any) ([]byte, error) { return engine.Marshal(in) }

// ValueValidatorFactory builds a validator from structured tag arguments.
// Numeric arguments are json.Number values, preserving their exact spelling.
type ValueValidatorFactory = engine.ValueValidatorFactory

// KeyValidatorFactory builds a key validator from structured tag arguments.
type KeyValidatorFactory = engine.KeyValidatorFactory

// TypeBinding supplies a schema for a type whose YAML representation cannot
// be inferred safely. Encode and Decode may differ.
type TypeBinding = engine.TypeBinding

// RegistryConfig declares named validation extensions. The constructor takes
// a snapshot of its maps, slices, and cloneable validators.
type RegistryConfig = engine.RegistryConfig

// Registry is an immutable collection of high-level schema extensions.
type Registry = engine.Registry

// NewRegistry snapshots and validates the supplied registry configuration.
func NewRegistry(config RegistryConfig) (*Registry, error) { return engine.NewRegistry(config) }

// JSONSchemaDraft selects the default JSON Schema dialect used when a schema
// does not declare $schema. An explicit $schema in the document takes precedence.
type JSONSchemaDraft = engine.JSONSchemaDraft

const JSONSchemaDraft4 = engine.JSONSchemaDraft4

const JSONSchemaDraft6 = engine.JSONSchemaDraft6

const JSONSchemaDraft7 = engine.JSONSchemaDraft7

const JSONSchemaDraft2019 = engine.JSONSchemaDraft2019

const JSONSchemaDraft2020 = engine.JSONSchemaDraft2020

// JSONSchemaCompileOptions controls JSON Schema compilation and validation.
type JSONSchemaCompileOptions = engine.JSONSchemaCompileOptions

// CompileJSONSchema compiles a JSON Schema using draft 2020-12 as the default
// dialect. Schemas declaring $schema are compiled using their declared dialect.
func CompileJSONSchema(data []byte) (*FieldSchema, error) { return engine.CompileJSONSchema(data) }

// CompileJSONSchemaWithOptions compiles a standards-compliant JSON Schema and
// returns a FieldSchema adapter that validates YAML through the JSON Schema engine
// while preserving YAML source positions in diagnostics.
func CompileJSONSchemaWithOptions(data []byte, opts JSONSchemaCompileOptions) (*FieldSchema, error) {
	return engine.CompileJSONSchemaWithOptions(data, opts)
}

// CompileJSONSchemaContext is the context-aware form of CompileJSONSchema.
// Context cancellation is observed by library-controlled compilation work and
// by JSONSchemaResolver implementations.
func CompileJSONSchemaContext(ctx context.Context, data []byte) (*FieldSchema, error) {
	return engine.CompileJSONSchemaContext(ctx, data)
}

// CompileJSONSchemaContextWithOptions is the context-aware form of
// CompileJSONSchemaWithOptions.
func CompileJSONSchemaContextWithOptions(
	ctx context.Context,
	data []byte,
	opts JSONSchemaCompileOptions,
) (*FieldSchema, error) {
	return engine.CompileJSONSchemaContextWithOptions(ctx, data, opts)
}

// JSONSchemaContentEncoding registers a custom JSON Schema contentEncoding.
type JSONSchemaContentEncoding = engine.JSONSchemaContentEncoding

// JSONSchemaContentMediaType registers a custom JSON Schema contentMediaType.
// UnmarshalJSON is optional and should be set only when the media type contains
// a JSON-compatible data model that contentSchema may validate.
type JSONSchemaContentMediaType = engine.JSONSchemaContentMediaType

// JSONSchemaFormat registers a custom JSON Schema format without exposing the
// underlying JSON Schema engine API.
type JSONSchemaFormat = engine.JSONSchemaFormat

// JSONSchemaKeywordValidationContext collects custom keyword diagnostics and
// evaluation annotations during one keyword validation call.
type JSONSchemaKeywordValidationContext = engine.JSONSchemaKeywordValidationContext

// JSONSchemaSubschema is a compiled nested schema used by custom keywords.
// Its implementation is intentionally opaque so callers do not depend on the
// underlying JSON Schema engine.
type JSONSchemaSubschema = engine.JSONSchemaSubschema

// JSONSchemaKeywordCompileContext exposes high-level helpers needed by custom
// keywords which contain nested schemas.
type JSONSchemaKeywordCompileContext = engine.JSONSchemaKeywordCompileContext

// JSONSchemaSubschemaPosition identifies one possible nested-schema location
// inside a custom keyword value. Use the constructor functions below.
type JSONSchemaSubschemaPosition = engine.JSONSchemaSubschemaPosition

// JSONSchemaSubschemaPath declares a nested schema location relative to a
// custom keyword value.
type JSONSchemaSubschemaPath = engine.JSONSchemaSubschemaPath

// JSONSchemaSubschemaProperty selects one object property.
func JSONSchemaSubschemaProperty(name string) JSONSchemaSubschemaPosition {
	return engine.JSONSchemaSubschemaProperty(name)
}

// JSONSchemaSubschemaAllProperties selects all object property values.
func JSONSchemaSubschemaAllProperties() JSONSchemaSubschemaPosition {
	return engine.JSONSchemaSubschemaAllProperties()
}

// JSONSchemaSubschemaItem selects one array item.
func JSONSchemaSubschemaItem(index int) JSONSchemaSubschemaPosition {
	return engine.JSONSchemaSubschemaItem(index)
}

// JSONSchemaSubschemaAllItems selects all array items.
func JSONSchemaSubschemaAllItems() JSONSchemaSubschemaPosition {
	return engine.JSONSchemaSubschemaAllItems()
}

// JSONSchemaKeywordValidateFunc validates one JSON instance value.
type JSONSchemaKeywordValidateFunc = engine.JSONSchemaKeywordValidateFunc

// JSONSchemaKeywordFunc is the simple custom-keyword form. The schema keyword
// value and current JSON instance value are supplied on each validation call.
type JSONSchemaKeywordFunc = engine.JSONSchemaKeywordFunc

// JSONSchemaKeywordCompileFunc compiles a keyword value from the schema into a
// runtime validation function. It is useful for parsing/validating keyword
// configuration once instead of on every validation call.
type JSONSchemaKeywordCompileFunc = engine.JSONSchemaKeywordCompileFunc

// JSONSchemaKeywordCompileContextFunc is the advanced high-level compile form
// for keywords that contain nested schemas or schema references.
type JSONSchemaKeywordCompileContextFunc = engine.JSONSchemaKeywordCompileContextFunc

// JSONSchemaKeyword defines one custom JSON Schema keyword. Set exactly one of
// Validate, Compile, or CompileWithContext. Subschemas declares locations of
// nested schemas relative to the keyword value.
type JSONSchemaKeyword = engine.JSONSchemaKeyword

// JSONSchemaVocabulary groups custom keywords under a vocabulary URL.
// Vocabularies registered through JSONSchemaCompileOptions are activated for
// that compilation.
type JSONSchemaVocabulary = engine.JSONSchemaVocabulary

// ErrJSONSchemaResourceNotFound allows resolver chains to distinguish a miss
// from an actual resolver failure.
var ErrJSONSchemaResourceNotFound = engine.ErrJSONSchemaResourceNotFound

// ErrJSONSchemaResourceOutsideRoot is returned by JSONSchemaFileResolver when
// a file URL escapes its configured root.
var ErrJSONSchemaResourceOutsideRoot = engine.ErrJSONSchemaResourceOutsideRoot

// JSONSchemaResolver resolves an absolute JSON Schema retrieval URL.
// Implementations must be safe for concurrent use if a compile configuration is
// reused concurrently.
type JSONSchemaResolver = engine.JSONSchemaResolver

// JSONSchemaResolverFunc adapts a function to JSONSchemaResolver.
type JSONSchemaResolverFunc = engine.JSONSchemaResolverFunc

// JSONSchemaResourceMap is an in-memory resolver keyed by absolute retrieval URL.
// Treat the map as immutable while it is in use; resolved byte slices are copied.
type JSONSchemaResourceMap = engine.JSONSchemaResourceMap

// JSONSchemaResolverChain tries resolvers in order. Only
// ErrJSONSchemaResourceNotFound advances to the next resolver. Treat the slice
// as immutable while it is in use.
type JSONSchemaResolverChain = engine.JSONSchemaResolverChain

// JSONSchemaCachingResolver caches successful resolutions.
// Returned byte slices are defensive copies.
type JSONSchemaCachingResolver = engine.JSONSchemaCachingResolver

// NewJSONSchemaCachingResolver wraps resolver with a concurrency-safe cache.
func NewJSONSchemaCachingResolver(resolver JSONSchemaResolver) *JSONSchemaCachingResolver {
	return engine.NewJSONSchemaCachingResolver(resolver)
}

// JSONSchemaFileResolver resolves file:// resources confined to one directory.
// The root must exist when the resolver is created.
type JSONSchemaFileResolver = engine.JSONSchemaFileResolver

// NewJSONSchemaFileResolver creates a file resolver constrained to root.
func NewJSONSchemaFileResolver(root string) (*JSONSchemaFileResolver, error) {
	return engine.NewJSONSchemaFileResolver(root)
}

// Ptr returns a pointer to the value. Useful for setting Min/Max fields.
func Ptr[T any](v T) *T { return engine.Ptr[T](v) }

// DefinitionValidator may be implemented by value/key validators that can
// validate their own configuration during FieldSchema compilation.
type DefinitionValidator = engine.DefinitionValidator

// CompileFieldSchema validates a native FieldSchema before constructing a
// Validator. New code should prefer this over NewValidator when schemas are
// assembled dynamically or loaded from configuration.
func CompileFieldSchema(schema *FieldSchema) (*Validator, error) {
	return engine.CompileFieldSchema(schema)
}

// ValidateFieldSchema checks FieldSchema invariants, including nested schemas,
// invalid constraints, non-progressing recursion, and validator definitions.
func ValidateFieldSchema(schema *FieldSchema) error { return engine.ValidateFieldSchema(schema) }

// ErrorLevel defines the severity of a validation error.
type ErrorLevel = engine.ErrorLevel

// LevelWarning indicates a non-critical issue (e.g., deprecated field, unknown key in permissive mode).
const LevelWarning = engine.LevelWarning

// LevelError indicates a critical validation failure.
const LevelError = engine.LevelError

// ValidationError represents a single validation issue.
type ValidationError = engine.ValidationError

// ErrorCollector accumulates validation errors and warnings.
type ErrorCollector = engine.ErrorCollector

// NewErrorCollector creates a new empty ErrorCollector.
func NewErrorCollector() *ErrorCollector { return engine.NewErrorCollector() }

// ValidationContext holds configuration and state for a validation run.
type ValidationContext = engine.ValidationContext

// NewValidationContext creates a new ValidationContext with default settings.
func NewValidationContext() *ValidationContext { return engine.NewValidationContext() }

// NodeType represents the expected YAML node type.
type NodeType = engine.NodeType

// TypeAny accepts any type.
const TypeAny = engine.TypeAny

// TypeNull represents null/nil values.
const TypeNull = engine.TypeNull

// TypeString represents string values.
const TypeString = engine.TypeString

// TypeInt represents integer values.
const TypeInt = engine.TypeInt

// TypeFloat represents floating-point values (also accepts int).
const TypeFloat = engine.TypeFloat

// TypeBool represents boolean values.
const TypeBool = engine.TypeBool

// TypeMap represents mapping nodes.
const TypeMap = engine.TypeMap

// TypeSequence represents sequence/array nodes.
const TypeSequence = engine.TypeSequence

// UnknownKeyPolicy determines how unknown keys in maps are handled.
type UnknownKeyPolicy = engine.UnknownKeyPolicy

// UnknownKeyInherit uses ctx.StrictKeys to decide:
//
//	StrictKeys=true  -> error
//	StrictKeys=false -> warning
const UnknownKeyInherit = engine.UnknownKeyInherit

// UnknownKeyError treats unknown keys as errors.
const UnknownKeyError = engine.UnknownKeyError

// UnknownKeyWarn treats unknown keys as warnings.
const UnknownKeyWarn = engine.UnknownKeyWarn

// UnknownKeyIgnore silently ignores unknown keys.
const UnknownKeyIgnore = engine.UnknownKeyIgnore

// ValueValidator validates a node's value. Validator instances may be reused
// concurrently; custom implementations that keep mutable state must therefore
// provide their own synchronization.
type ValueValidator = engine.ValueValidator

// ValueValidatorCloner can be implemented by stateful/configurable custom
// validators so CompileFieldSchema can snapshot their configuration.
type ValueValidatorCloner = engine.ValueValidatorCloner

// KeyValidator validates key names in mappings. The same concurrency contract
// as ValueValidator applies to stateful implementations.
type KeyValidator = engine.KeyValidator

// KeyValidatorCloner is the key-validator counterpart of ValueValidatorCloner.
type KeyValidatorCloner = engine.KeyValidatorCloner

// ConditionalRule defines conditional validation logic.
// When ConditionField equals ConditionValue, additional requirements apply.
type ConditionalRule = engine.ConditionalRule

// FieldSchema defines the validation rules for a field.
type FieldSchema = engine.FieldSchema

// ValidationResult contains the validation outcome and context for formatting.
type ValidationResult = engine.ValidationResult

// Validator performs YAML validation against a schema.
type Validator = engine.Validator

// NewValidator creates a Validator that references the supplied schema directly.
// Callers must not mutate that schema concurrently with validation. Prefer
// CompileFieldSchema when an immutable, concurrency-safe schema snapshot is desired.
func NewValidator(schema *FieldSchema) *Validator { return engine.NewValidator(schema) }

// InferNodeType applies the same YAML type inference used by FieldSchema validation.
// A nil context uses default type-inference settings.
func InferNodeType(node *yaml.Node, ctx *ValidationContext) NodeType {
	return engine.InferNodeType(node, ctx)
}

// RenderLineWithCaret is an exported wrapper useful for external tests and tools.
func RenderLineWithCaret(line string, byteCol int) (string, int, int) {
	return engine.RenderLineWithCaret(line, byteCol)
}

// FormatErrorWithSource formats an error with source context.
// Correctly handles tabs and Unicode.
func FormatErrorWithSource(err ValidationError, lines []string) string {
	return engine.FormatErrorWithSource(err, lines)
}
