// Package yamlvalidator provides a flexible YAML validation library
// with support for type checking, custom validators, conditional logic,
// and detailed error reporting with source context.
package yamlvalidator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// Error Levels and Collection
// ============================================================================

// ErrorLevel defines the severity of a validation error.
type ErrorLevel int

const (
	// LevelWarning indicates a non-critical issue (e.g., deprecated field, unknown key in permissive mode).
	LevelWarning ErrorLevel = iota
	// LevelError indicates a critical validation failure.
	LevelError
)

func (l ErrorLevel) String() string {
	if l == LevelWarning {
		return "WARNING"
	}
	return "ERROR"
}

// ValidationError represents a single validation issue.
type ValidationError struct {
	Level      ErrorLevel
	Code       string      // Stable machine-readable diagnostic code when available.
	SchemaPath string      // Schema location that produced the diagnostic, when available.
	Path       string      // Stable display path, e.g. "spec.containers[0].image".
	PathTokens []PathToken // Typed path components for programmatic consumers.
	Line       int         // 1-based line number (0 if unknown)
	Column     int         // 1-based column number (0 if unknown)
	Message    string
	Got        string // Actual value/type description
	Expected   string // Expected value/type description
	Details    any    // Optional typed machine-readable diagnostic details.
}

func (e ValidationError) Error() string {
	var details string
	if e.Expected != "" && e.Got != "" {
		details = fmt.Sprintf(" (expected %s, got %s)", e.Expected, e.Got)
	} else if e.Got != "" {
		details = fmt.Sprintf(" (got %s)", e.Got)
	}

	var pos string
	if e.Line > 0 {
		if e.Column > 0 {
			pos = fmt.Sprintf("line %d:%d: ", e.Line, e.Column)
		} else {
			pos = fmt.Sprintf("line %d: ", e.Line)
		}
	}

	return fmt.Sprintf("[%s] %s%s%s (path: %s)", e.Level, pos, e.Message, details, e.Path)
}

// ErrorCollector accumulates validation errors and warnings.
type ErrorCollector struct {
	errors   []ValidationError
	warnings []ValidationError
	all      []ValidationError
}

// NewErrorCollector creates a new empty ErrorCollector.
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{}
}

// Add adds a validation error to the collector.
func (c *ErrorCollector) Add(err ValidationError) {
	normalizeDiagnosticPath(&err)
	c.all = append(c.all, err)
	if err.Level == LevelError {
		c.errors = append(c.errors, err)
	} else {
		c.warnings = append(c.warnings, err)
	}
}

// HasErrors returns true if there are any errors (not warnings).
func (c *ErrorCollector) HasErrors() bool {
	return len(c.errors) > 0
}

// Errors returns a defensive copy of all errors.
func (c *ErrorCollector) Errors() []ValidationError {
	return cloneDiagnostics(c.errors)
}

// Warnings returns a defensive copy of all warnings.
func (c *ErrorCollector) Warnings() []ValidationError {
	return cloneDiagnostics(c.warnings)
}

// All returns a defensive copy of all diagnostics in insertion order.
func (c *ErrorCollector) All() []ValidationError {
	return cloneDiagnostics(c.all)
}

// ============================================================================
// Validation Context
// ============================================================================

// ValidationContext holds configuration and state for a validation run.
type ValidationContext struct {
	// StrictKeys determines the default behavior for unknown keys
	// when UnknownKeyPolicy is UnknownKeyInherit:
	//   true  -> unknown keys are errors
	//   false -> unknown keys are warnings
	StrictKeys bool

	// StopOnFirst stops validation after the first error.
	StopOnFirst bool

	// StrictTypes uses only YAML tags for type inference.
	// When false, values are parsed to infer types (e.g., "123" -> int).
	StrictTypes bool

	// YAML11Booleans enables YAML 1.1 boolean literals (yes/no/on/off).
	// By default, only YAML 1.2 booleans (true/false) are recognized.
	YAML11Booleans bool

	// MaxBytes rejects inputs larger than this many bytes. Zero means unlimited.
	MaxBytes int

	// MaxDocuments limits the number of YAML documents in a stream. Zero means unlimited.
	MaxDocuments int

	// MaxDepth limits YAML container/schema traversal depth. The root value has depth 1.
	// Zero means unlimited.
	MaxDepth int

	// MaxDiagnostics stops validation after this many errors/warnings. Zero means unlimited.
	MaxDiagnostics int

	// SourceLines contains the original YAML lines for error formatting.
	SourceLines []string

	collector    *ErrorCollector
	stopped      bool
	limitReached bool
	depth        int
	runContext   context.Context
	contextErr   error
}

// NewValidationContext creates a new ValidationContext with default settings.
func NewValidationContext() *ValidationContext {
	return &ValidationContext{
		collector: NewErrorCollector(),
	}
}

// Context returns the context associated with the current validation run.
// Custom validators should use it for cancellation/deadline-aware work.
func (ctx *ValidationContext) Context() context.Context {
	if ctx.runContext == nil {
		return context.Background()
	}
	return ctx.runContext
}

// ContextErr reports context cancellation/deadline failure for this run.
func (ctx *ValidationContext) ContextErr() error {
	ctx.checkCanceled()
	return ctx.contextErr
}

// AddError adds an error to the context's collector.
func (ctx *ValidationContext) AddError(err ValidationError) {
	if ctx.checkCanceled() || ctx.stopped {
		return
	}
	if ctx.MaxDiagnostics > 0 && len(ctx.collector.all) >= ctx.MaxDiagnostics {
		ctx.stopped = true
		ctx.limitReached = true
		return
	}
	ctx.collector.Add(err)
	if ctx.MaxDiagnostics > 0 && len(ctx.collector.all) >= ctx.MaxDiagnostics {
		ctx.stopped = true
		ctx.limitReached = true
	}
	if ctx.StopOnFirst && err.Level == LevelError {
		ctx.stopped = true
	}
}

func (ctx *ValidationContext) checkCanceled() bool {
	if ctx.contextErr != nil {
		ctx.stopped = true
		return true
	}
	if ctx.runContext == nil {
		return false
	}
	select {
	case <-ctx.runContext.Done():
		ctx.contextErr = ctx.runContext.Err()
		ctx.stopped = true
		return true
	default:
		return false
	}
}

// IsStopped returns true if validation has been stopped by validation policy
// or by context cancellation/deadline.
func (ctx *ValidationContext) IsStopped() bool {
	return ctx.checkCanceled() || ctx.stopped
}

// Collector returns the error collector.
func (ctx *ValidationContext) Collector() *ErrorCollector {
	return ctx.collector
}

// ============================================================================
// Node Types
// ============================================================================

// NodeType represents the expected YAML node type.
type NodeType int

const (
	// TypeAny accepts any type.
	TypeAny NodeType = iota
	// TypeNull represents null/nil values.
	TypeNull
	// TypeString represents string values.
	TypeString
	// TypeInt represents integer values.
	TypeInt
	// TypeFloat represents floating-point values (also accepts int).
	TypeFloat
	// TypeBool represents boolean values.
	TypeBool
	// TypeMap represents mapping nodes.
	TypeMap
	// TypeSequence represents sequence/array nodes.
	TypeSequence
)

func (t NodeType) String() string {
	switch t {
	case TypeAny:
		return "any"
	case TypeNull:
		return "null"
	case TypeString:
		return "string"
	case TypeInt:
		return "integer"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "boolean"
	case TypeMap:
		return "map"
	case TypeSequence:
		return "sequence"
	default:
		return "unknown"
	}
}

// ============================================================================
// Unknown Key Policy
// ============================================================================

// UnknownKeyPolicy determines how unknown keys in maps are handled.
type UnknownKeyPolicy int

const (
	// UnknownKeyInherit uses ctx.StrictKeys to decide:
	//   StrictKeys=true  -> error
	//   StrictKeys=false -> warning
	UnknownKeyInherit UnknownKeyPolicy = iota

	// UnknownKeyError treats unknown keys as errors.
	UnknownKeyError

	// UnknownKeyWarn treats unknown keys as warnings.
	UnknownKeyWarn

	// UnknownKeyIgnore silently ignores unknown keys.
	UnknownKeyIgnore
)

// ============================================================================
// Validators Interfaces
// ============================================================================

// ValueValidator validates a node's value. Validator instances may be reused
// concurrently; custom implementations that keep mutable state must therefore
// provide their own synchronization.
type ValueValidator interface {
	Validate(node *yaml.Node, path string, ctx *ValidationContext)
}

// ValueValidatorCloner can be implemented by stateful/configurable custom
// validators so CompileFieldSchema can snapshot their configuration.
type ValueValidatorCloner interface {
	CloneValueValidator() ValueValidator
}

// KeyValidator validates key names in mappings. The same concurrency contract
// as ValueValidator applies to stateful implementations.
type KeyValidator interface {
	ValidateKey(key string, keyNode *yaml.Node, path string, ctx *ValidationContext)
}

// KeyValidatorCloner is the key-validator counterpart of ValueValidatorCloner.
type KeyValidatorCloner interface {
	CloneKeyValidator() KeyValidator
}

// ============================================================================
// Conditional Rules
// ============================================================================

// ConditionalRule defines conditional validation logic.
// When ConditionField equals ConditionValue, additional requirements apply.
type ConditionalRule struct {
	// ConditionField is the field to check.
	ConditionField string
	// ConditionValue is the expected value (scalar comparison).
	ConditionValue string
	// ThenRequired lists fields that become required when condition is met.
	ThenRequired []string
	// ThenForbidden lists fields that are forbidden when condition is met.
	ThenForbidden []string
}

// ============================================================================
// Field Schema
// ============================================================================

// FieldSchema defines the validation rules for a field.
type FieldSchema struct {
	// Type is the expected node type.
	Type NodeType

	// AllowedTypes accepts any of the listed node types. When non-empty it
	// takes precedence over Type and is useful for schema unions.
	AllowedTypes []NodeType

	// Required indicates the field must be present.
	Required bool

	// Nullable allows null values even when Type is not TypeNull.
	Nullable bool

	// Deprecated contains a deprecation message (empty = not deprecated).
	// Use "true" for a generic message.
	Deprecated string

	// Description is a human-readable field description.
	Description string

	// Default is the default value. If set and field is missing, a warning is emitted.
	Default interface{}

	// ─────────────────────────────────────────────────────────────────────────
	// Map-specific fields
	// ─────────────────────────────────────────────────────────────────────────

	// AllowedKeys defines known keys and their schemas.
	// If nil, ALL keys are considered unknown.
	// This does NOT mean "don't check" - it means "no known keys".
	//
	// For "any keys allowed, no validation":
	//   AllowedKeys: nil,
	//   AdditionalProperties: &FieldSchema{Type: TypeAny},
	//
	// For "don't touch this map at all":
	//   AllowedKeys: nil,
	//   AdditionalProperties: nil,
	//   UnknownKeyPolicy: UnknownKeyIgnore,
	AllowedKeys map[string]*FieldSchema

	// AdditionalProperties is the schema for keys not in AllowedKeys.
	// If not nil: unknown keys are allowed and validated against this schema.
	// If nil: unknown keys are handled by UnknownKeyPolicy.
	AdditionalProperties *FieldSchema

	// UnknownKeyPolicy determines handling of keys not in AllowedKeys
	// when AdditionalProperties is nil.
	UnknownKeyPolicy UnknownKeyPolicy

	// KeyValidators validate key names (applied to ALL keys).
	KeyValidators []KeyValidator

	// ─────────────────────────────────────────────────────────────────────────
	// Sequence-specific fields
	// ─────────────────────────────────────────────────────────────────────────

	// ItemSchema is the schema for sequence items.
	ItemSchema *FieldSchema

	// MinItems is the minimum number of items (nil = no limit).
	MinItems *int

	// MaxItems is the maximum number of items (nil = no limit).
	MaxItems *int

	// ─────────────────────────────────────────────────────────────────────────
	// Value validators
	// ─────────────────────────────────────────────────────────────────────────

	// Validators are custom value validators.
	Validators []ValueValidator

	// ─────────────────────────────────────────────────────────────────────────
	// Inter-field logic (map only)
	// ─────────────────────────────────────────────────────────────────────────

	// AnyOf requires at least one field group to be fully present.
	// Example: [][]string{{"configFile"}, {"host", "port"}}
	// Means: either configFile, OR both host AND port.
	AnyOf [][]string

	// ExactlyOneOf requires exactly one field from the list.
	// Example: []string{"inline", "file", "url"}
	// Means: exactly one of inline/file/url must be present.
	ExactlyOneOf []string

	// MutuallyExclusive allows at most one field from the list (zero is OK).
	// Example: []string{"debug", "quiet"}
	// Means: debug and quiet cannot both be present.
	MutuallyExclusive []string

	// Conditions define conditional validation rules.
	Conditions []ConditionalRule

	// OneOfSchemas requires exactly one child schema to validate successfully.
	OneOfSchemas []*FieldSchema

	// AnyOfSchemas requires at least one child schema to validate successfully.
	AnyOfSchemas []*FieldSchema

	// OneOfRequired requires exactly one field group to be fully present.
	OneOfRequired [][]string

	// ForbiddenTogether rejects groups whose fields are all present together.
	ForbiddenTogether [][]string

	// DependentRequired requires additional fields when a trigger field is present.
	DependentRequired map[string][]string
}

// ============================================================================
// Validation Result
// ============================================================================

// ValidationResult contains the validation outcome and context for formatting.
type ValidationResult struct {
	// Collector contains all errors and warnings.
	Collector *ErrorCollector
	// SourceLines contains the original YAML lines.
	SourceLines []string
	// Truncated reports that validation stopped because MaxDiagnostics was reached.
	Truncated bool
	// Canceled reports that validation stopped because its context was canceled
	// or its deadline expired. Context-aware methods also return ContextErr.
	Canceled bool
	// ContextErr is context.Canceled or context.DeadlineExceeded when Canceled is true.
	ContextErr error
}

// HasErrors returns true if there are any errors.
func (r *ValidationResult) HasErrors() bool {
	return r.Collector.HasErrors()
}

// SortByPosition sorts errors by position in the file.
func (r *ValidationResult) SortByPosition() {
	all := r.Collector.All()
	sort.Slice(all, func(i, j int) bool {
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		if all[i].Column != all[j].Column {
			return all[i].Column < all[j].Column
		}
		// Errors before warnings when at same position
		return all[i].Level > all[j].Level
	})

	r.Collector = NewErrorCollector()
	for _, err := range all {
		r.Collector.Add(err)
	}
}

// FormatAll formats all errors with source context.
func (r *ValidationResult) FormatAll(sortByPos bool) string {
	var sb strings.Builder
	var items []ValidationError
	if sortByPos {
		items = r.sortedAllByPosition()
	} else {
		items = r.Collector.All()
	}

	for _, err := range items {
		sb.WriteString(FormatErrorWithSource(err, r.SourceLines))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r *ValidationResult) sortedAllByPosition() []ValidationError {
	all := r.Collector.All()
	sort.Slice(all, func(i, j int) bool {
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		if all[i].Column != all[j].Column {
			return all[i].Column < all[j].Column
		}
		// Errors before warnings when at same position
		return all[i].Level > all[j].Level
	})
	return all
}

// ============================================================================
// Validator
// ============================================================================

// Validator performs YAML validation against a schema.
type Validator struct {
	schema *FieldSchema
}

// NewValidator creates a Validator that references the supplied schema directly.
// Callers must not mutate that schema concurrently with validation. Prefer
// CompileFieldSchema when an immutable, concurrency-safe schema snapshot is desired.
func NewValidator(schema *FieldSchema) *Validator {
	return &Validator{schema: schema}
}

// ValidateBytes validates YAML data and returns the result.
// Supports multi-document YAML (separated by ---).
func (v *Validator) ValidateBytes(data []byte) *ValidationResult {
	result, _ := v.validateDataContext(context.Background(), data, ValidationContext{})
	return result
}

// ValidateWithOptions validates YAML data with custom options.
func (v *Validator) ValidateWithOptions(data []byte, opts ValidationContext) *ValidationResult {
	result, _ := v.validateDataContext(context.Background(), data, opts)
	return result
}

// ValidateContext validates YAML data with cancellation/deadline support.
// Validation diagnostics remain in the returned result; the error is reserved
// for context cancellation/deadline.
func (v *Validator) ValidateContext(runCtx context.Context, data []byte) (*ValidationResult, error) {
	return v.validateDataContext(runCtx, data, ValidationContext{})
}

// ValidateContextWithOptions is the context-aware form of ValidateWithOptions.
func (v *Validator) ValidateContextWithOptions(
	runCtx context.Context,
	data []byte,
	opts ValidationContext,
) (*ValidationResult, error) {
	return v.validateDataContext(runCtx, data, opts)
}

func (v *Validator) validateDataContext(
	runCtx context.Context,
	data []byte,
	opts ValidationContext,
) (*ValidationResult, error) {
	if runCtx == nil {
		runCtx = context.Background()
	}

	ctx := &opts
	ctx.collector = NewErrorCollector()
	ctx.stopped = false
	ctx.limitReached = false
	ctx.depth = 0
	ctx.runContext = runCtx
	ctx.contextErr = nil

	if ctx.checkCanceled() {
		result := validationResultFromContext(ctx)
		return result, ctx.contextErr
	}

	if ctx.MaxBytes > 0 && len(data) > ctx.MaxBytes {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "max_bytes",
			Message:  "YAML input exceeds configured byte limit",
			Got:      fmt.Sprintf("%d bytes", len(data)),
			Expected: fmt.Sprintf("<= %d bytes", ctx.MaxBytes),
		})
		result := validationResultFromContext(ctx)
		return result, ctx.contextErr
	}

	ctx.SourceLines = splitLines(data)
	v.validateWithContext(&contextReader{ctx: runCtx, reader: bytes.NewReader(data)}, ctx)
	ctx.checkCanceled()
	result := validationResultFromContext(ctx)
	return result, ctx.contextErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-reader.ctx.Done():
		return 0, reader.ctx.Err()
	default:
		return reader.reader.Read(buffer)
	}
}

func validationResultFromContext(ctx *ValidationContext) *ValidationResult {
	return &ValidationResult{
		Collector:   ctx.Collector(),
		SourceLines: ctx.SourceLines,
		Truncated:   ctx.limitReached,
		Canceled:    ctx.contextErr != nil,
		ContextErr:  ctx.contextErr,
	}
}

func (v *Validator) validateWithContext(r io.Reader, ctx *ValidationContext) {
	decoder := yaml.NewDecoder(r)
	docIndex := 0

	for {
		if ctx.IsStopped() {
			return
		}
		var root yaml.Node
		err := decoder.Decode(&root)
		if err == io.EOF {
			break
		}
		if err != nil {
			if runErr := ctx.Context().Err(); runErr != nil {
				ctx.contextErr = runErr
				ctx.stopped = true
				return
			}
			ctx.AddError(parseYAMLError(err, docIndex))
			return
		}

		if ctx.MaxDocuments > 0 && docIndex >= ctx.MaxDocuments {
			line, column := 0, 0
			if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
				line, column = root.Content[0].Line, root.Content[0].Column
			}
			ctx.AddError(ValidationError{
				Level:    LevelError,
				Code:     "max_documents",
				Path:     fmt.Sprintf("doc[%d]", docIndex),
				Line:     line,
				Column:   column,
				Message:  "YAML stream contains too many documents",
				Expected: fmt.Sprintf("at most %d documents", ctx.MaxDocuments),
			})
			return
		}

		if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
			prefix := ""
			if docIndex > 0 {
				prefix = fmt.Sprintf("doc[%d]", docIndex)
			}
			v.checkAliasSafety(root.Content[0], prefix, ctx, make(map[*yaml.Node]uint8))
			if !ctx.IsStopped() {
				v.checkDuplicateKeysRecursive(root.Content[0], prefix, ctx, make(map[*yaml.Node]bool), 1)
			}
			if !ctx.IsStopped() {
				v.validateNode(root.Content[0], v.schema, prefix, ctx)
			}
		}

		docIndex++
		if ctx.IsStopped() {
			break
		}
	}

	if docIndex == 0 && v.schema != nil && v.schema.Required && !ctx.IsStopped() {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "required_document",
			Message: "required YAML document is missing",
		})
	}
}

// InferTypeForPublic exposes internal type inference for external validators.
func (v *Validator) InferTypeForPublic(node *yaml.Node, ctx *ValidationContext) NodeType {
	return v.inferType(node, ctx)
}

// ============================================================================
// Node Validation
// ============================================================================

func (v *Validator) validateNode(node *yaml.Node, schema *FieldSchema, path string, ctx *ValidationContext) {
	if schema == nil || ctx.IsStopped() {
		return
	}

	ctx.depth++
	defer func() { ctx.depth-- }()
	if ctx.MaxDepth > 0 && ctx.depth > ctx.MaxDepth {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "max_depth",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "YAML value exceeds configured nesting depth",
			Expected: fmt.Sprintf("depth <= %d", ctx.MaxDepth),
			Got:      fmt.Sprintf("depth %d", ctx.depth),
		})
		return
	}

	// Resolve aliases
	if node.Kind == yaml.AliasNode {
		if node.Alias != nil {
			node = node.Alias
		} else {
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Code:    "unresolved_alias",
				Path:    cleanPath(path),
				Line:    node.Line,
				Column:  node.Column,
				Message: "unresolved alias",
			})
			return
		}
	}

	// Check deprecated
	if schema.Deprecated != "" {
		msg := schema.Deprecated
		if msg == "true" {
			msg = "this field is deprecated"
		}
		ctx.AddError(ValidationError{
			Level:   LevelWarning,
			Code:    "deprecated",
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: msg,
		})
	}

	// Schema composition is evaluated before the base schema constraints.
	if len(schema.OneOfSchemas) > 0 && !v.validateSchemaAlternatives(node, schema.OneOfSchemas, path, ctx, true) {
		return
	}
	if len(schema.AnyOfSchemas) > 0 && !v.validateSchemaAlternatives(node, schema.AnyOfSchemas, path, ctx, false) {
		return
	}

	// Type check
	if !v.checkTypeWithSchema(node, schema, path, ctx) {
		return
	}

	// Structure validation. A bare TypeAny does not recursively validate map/sequence
	// contents unless structural constraints were explicitly configured.
	switch node.Kind {
	case yaml.MappingNode:
		if schema.Type != TypeAny || hasMappingConstraints(schema) {
			v.validateMapping(node, schema, path, ctx)
		}
	case yaml.SequenceNode:
		if schema.Type != TypeAny || hasSequenceConstraints(schema) {
			v.validateSequence(node, schema, path, ctx)
		}
	case yaml.ScalarNode:
		// Scalars are validated via ValueValidators
	}

	// Custom validators
	for _, validator := range schema.Validators {
		if ctx.IsStopped() {
			return
		}
		validator.Validate(node, cleanPath(path), ctx)
	}
}

func hasMappingConstraints(schema *FieldSchema) bool {
	return schema.AllowedKeys != nil ||
		schema.AdditionalProperties != nil ||
		schema.UnknownKeyPolicy != UnknownKeyInherit ||
		len(schema.KeyValidators) > 0 ||
		len(schema.AnyOf) > 0 ||
		len(schema.ExactlyOneOf) > 0 ||
		len(schema.MutuallyExclusive) > 0 ||
		len(schema.OneOfRequired) > 0 ||
		len(schema.ForbiddenTogether) > 0 ||
		len(schema.DependentRequired) > 0 ||
		len(schema.Conditions) > 0
}

func hasSequenceConstraints(schema *FieldSchema) bool {
	return schema.ItemSchema != nil || schema.MinItems != nil || schema.MaxItems != nil
}

func (v *Validator) validateSchemaAlternatives(node *yaml.Node, schemas []*FieldSchema, path string,
	ctx *ValidationContext, exactlyOne bool) bool {

	matches := 0
	var selected []ValidationError

	for _, candidate := range schemas {
		branchCtx := &ValidationContext{
			StrictKeys:     ctx.StrictKeys,
			StopOnFirst:    true,
			StrictTypes:    ctx.StrictTypes,
			YAML11Booleans: ctx.YAML11Booleans,
			MaxDepth:       ctx.MaxDepth,
			SourceLines:    ctx.SourceLines,
			collector:      NewErrorCollector(),
			depth:          ctx.depth - 1,
			runContext:     ctx.runContext,
		}
		v.validateNode(node, candidate, path, branchCtx)
		if err := branchCtx.ContextErr(); err != nil {
			ctx.contextErr = err
			ctx.stopped = true
			return false
		}
		if branchCtx.Collector().HasErrors() {
			continue
		}
		matches++
		if selected == nil {
			selected = branchCtx.Collector().All()
		}
	}

	valid := matches > 0
	message := "value does not match any allowed schema"
	if exactlyOne {
		valid = matches == 1
		if matches > 1 {
			message = "value matches more than one mutually exclusive schema"
		}
	}
	if !valid {
		code := "any_of_schema"
		if exactlyOne {
			code = "one_of_schema"
		}
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    code,
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: message,
		})
		return false
	}

	for _, diagnostic := range selected {
		ctx.AddError(diagnostic)
	}
	return true
}

func (v *Validator) checkTypeWithSchema(node *yaml.Node, schema *FieldSchema, path string, ctx *ValidationContext) bool {
	expected := schema.Type
	actual := v.inferType(node, ctx)

	if len(schema.AllowedTypes) > 0 {
		if actual == TypeNull && schema.Nullable {
			return true
		}
		for _, allowed := range schema.AllowedTypes {
			if typeMatches(allowed, actual) {
				return true
			}
		}
		names := make([]string, 0, len(schema.AllowedTypes))
		for _, allowed := range schema.AllowedTypes {
			names = append(names, allowed.String())
		}
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "type_mismatch",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "type mismatch",
			Expected: fmt.Sprintf("one of %v", names),
			Got:      v.describeNode(node),
			Details: TypeMismatchDetails{
				Expected: append([]string(nil), names...),
				Actual:   actual.String(),
			},
		})
		return false
	}

	if expected == TypeAny {
		return true
	}

	// Null handling
	if actual == TypeNull {
		if expected == TypeNull {
			return true
		}
		if schema.Nullable {
			return true
		}
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "type_mismatch",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "unexpected null value",
			Expected: expected.String(),
			Got:      "null",
			Details: TypeMismatchDetails{
				Expected: []string{expected.String()},
				Actual:   TypeNull.String(),
			},
		})
		return false
	}

	if typeMatches(expected, actual) {
		return true
	}

	ctx.AddError(ValidationError{
		Level:    LevelError,
		Code:     "type_mismatch",
		Path:     cleanPath(path),
		Line:     node.Line,
		Column:   node.Column,
		Message:  "type mismatch",
		Expected: expected.String(),
		Got:      v.describeNode(node),
		Details: TypeMismatchDetails{
			Expected: []string{expected.String()},
			Actual:   actual.String(),
		},
	})
	return false
}

func typeMatches(expected, actual NodeType) bool {
	return expected == actual || (expected == TypeFloat && actual == TypeInt)
}

func (v *Validator) inferType(node *yaml.Node, ctx *ValidationContext) NodeType {
	switch node.Kind {
	case yaml.MappingNode:
		return TypeMap
	case yaml.SequenceNode:
		return TypeSequence
	case yaml.ScalarNode:
		return v.inferScalarType(node, ctx)
	case yaml.AliasNode:
		if node.Alias != nil {
			return v.inferType(node.Alias, ctx)
		}
		return TypeAny
	default:
		return TypeAny
	}
}

func (v *Validator) inferScalarType(node *yaml.Node, ctx *ValidationContext) NodeType {
	// Step 1: By tags (yaml.v3 has already parsed)
	switch node.Tag {
	case "!!str":
		// YAML 1.1 compatibility applies only to plain scalars. Quoted and
		// block scalars are explicitly strings and must remain strings.
		if ctx.YAML11Booleans && node.Style == 0 {
			lower := strings.ToLower(node.Value)
			if lower == "y" || lower == "yes" || lower == "true" || lower == "on" ||
				lower == "n" || lower == "no" || lower == "false" || lower == "off" {
				return TypeBool
			}
		}
		return TypeString
	case "!!int":
		return TypeInt
	case "!!float":
		// yaml.v3 resolves integer literals outside its machine-sized integer
		// range as !!float. Preserve mathematical integer semantics by checking
		// the original scalar text with arbitrary precision.
		if yamlScalarLooksInteger(node.Value) {
			return TypeInt
		}
		return TypeFloat
	case "!!bool":
		return TypeBool
	case "!!null":
		return TypeNull
	}

	// If strict, don't parse values
	if ctx.StrictTypes {
		return TypeString
	}

	// Step 2: Parse value (fallback for unrecognized tags)
	val := node.Value
	lower := strings.ToLower(val)

	// Null: explicit forms only, NOT empty string
	// Empty string "" is a valid string, yaml.v3 sets !!str
	// Empty unquoted value is resolved as !!null by yaml.v3
	if lower == "null" || val == "~" {
		return TypeNull
	}

	// Bool — YAML 1.2: only true/false
	if lower == "true" || lower == "false" {
		return TypeBool
	}

	// Bool — YAML 1.1 (optional)
	if ctx.YAML11Booleans {
		if lower == "y" || lower == "yes" || lower == "true" || lower == "on" ||
			lower == "n" || lower == "no" || lower == "false" || lower == "off" {
			return TypeBool
		}
	}

	// Int
	if v.looksLikeInt(val) {
		return TypeInt
	}

	// Float
	if v.looksLikeFloat(val) {
		return TypeFloat
	}

	return TypeString
}

// looksLikeInt checks if value looks like an integer.
// Supports:
//   - Decimal: 123, -456, +789
//   - Hex: 0x1A, 0X1a
//   - Octal (YAML 1.2): 0o17, 0O17
//   - Binary: 0b1010, 0B1010
//
// Does NOT support YAML 1.1 octal (0777).
func (v *Validator) looksLikeInt(s string) bool {
	if s == "" {
		return false
	}

	// Remove sign
	if s[0] == '+' || s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}

	// Hex: 0x...
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		_, err := strconv.ParseInt(s, 0, 64)
		return err == nil
	}

	// Octal YAML 1.2: 0o...
	if len(s) > 2 && (s[:2] == "0o" || s[:2] == "0O") {
		_, err := strconv.ParseInt(s[2:], 8, 64)
		return err == nil
	}

	// Binary: 0b...
	if len(s) > 2 && (s[:2] == "0b" || s[:2] == "0B") {
		_, err := strconv.ParseInt(s[2:], 2, 64)
		return err == nil
	}

	// Decimal
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func (v *Validator) looksLikeFloat(s string) bool {
	lower := strings.ToLower(s)
	if lower == ".inf" || lower == "-.inf" || lower == "+.inf" || lower == ".nan" {
		return true
	}

	// Must have dot or exponent for float
	if !strings.ContainsAny(s, ".eE") {
		return false
	}

	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func (v *Validator) describeNode(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "map"
	case yaml.SequenceNode:
		return fmt.Sprintf("sequence (len=%d)", len(node.Content))
	case yaml.ScalarNode:
		val := node.Value
		runes := []rune(val)
		if len(runes) > 20 {
			val = string(runes[:20]) + "..."
		}
		tag := strings.TrimPrefix(node.Tag, "!!")
		if tag == "" {
			tag = "scalar"
		}
		return fmt.Sprintf("%s %q", tag, val)
	default:
		return node.Tag
	}
}

// ============================================================================
// Mapping Validation
// ============================================================================

func (v *Validator) validateMapping(node *yaml.Node, schema *FieldSchema, path string, ctx *ValidationContext) {
	foundKeys := make(map[string]*yaml.Node)
	keyNodes := make(map[string]*yaml.Node)

	pairs := expandMappingWithMerges(node)

	for _, kv := range pairs {
		if ctx.IsStopped() {
			return
		}

		keyNode := kv.key
		valueNode := kv.value
		key := keyNode.Value
		fieldPath := joinPath(path, key)

		foundKeys[key] = valueNode
		keyNodes[key] = keyNode

		// Key validators (for all keys)
		for _, validator := range schema.KeyValidators {
			if ctx.IsStopped() {
				return
			}
			validator.ValidateKey(key, keyNode, cleanPath(fieldPath), ctx)
		}

		// Known key?
		if fieldSchema, ok := schema.AllowedKeys[key]; ok {
			v.validateNode(valueNode, fieldSchema, fieldPath, ctx)
			continue
		}

		// Unknown key handling
		if schema.AdditionalProperties != nil {
			// Validate value against AdditionalProperties schema
			v.validateNode(valueNode, schema.AdditionalProperties, fieldPath, ctx)
			continue
		}

		// Report unknown key based on policy
		level, report := v.resolveUnknownKeyLevel(schema.UnknownKeyPolicy, ctx)
		if report {
			message := fmt.Sprintf("unknown key %q", key)
			suggestion := suggestKnownKey(key, schema.AllowedKeys)
			if suggestion != "" {
				message += fmt.Sprintf("; did you mean %q?", suggestion)
			}
			ctx.AddError(ValidationError{
				Level:   level,
				Code:    "unknown_key",
				Path:    cleanPath(fieldPath),
				Line:    keyNode.Line,
				Column:  keyNode.Column,
				Message: message,
				Got:     v.describeNode(valueNode),
				Details: UnknownKeyDetails{
					Key:        key,
					Suggestion: suggestion,
				},
			})
		}
	}

	// Check required fields, defaults, and inter-field logic.
	v.checkRequiredFields(node, schema, path, foundKeys, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkDefaults(node, schema, path, foundKeys, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkAnyOf(node, schema, path, foundKeys, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkExactlyOneOf(node, schema, path, foundKeys, keyNodes, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkMutuallyExclusive(node, schema, path, foundKeys, keyNodes, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkOneOfRequired(node, schema, path, foundKeys, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkForbiddenTogether(node, schema, path, foundKeys, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkDependentRequired(node, schema, path, foundKeys, keyNodes, ctx)
	if ctx.IsStopped() {
		return
	}
	v.checkConditions(node, schema, path, foundKeys, keyNodes, ctx)
}

type kvPair struct {
	key   *yaml.Node
	value *yaml.Node
}

// expandMappingWithMerges expands YAML merge keys (<<) into concrete key/value pairs.
// Explicit keys always override merged keys. For a merge sequence [A, B], keys
// from A take precedence over keys from B, matching the YAML merge-key spec.
func expandMappingWithMerges(node *yaml.Node) []kvPair {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	var merged []kvPair
	var explicit []kvPair
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Value == "<<" {
			merged = appendUniquePairs(merged, extractMergePairs(valueNode))
			continue
		}
		explicit = append(explicit, kvPair{key: keyNode, value: valueNode})
	}

	explicit = dedupePairsKeepLast(explicit)
	explicitKeys := make(map[string]struct{}, len(explicit))
	for _, kv := range explicit {
		explicitKeys[kv.key.Value] = struct{}{}
	}

	out := make([]kvPair, 0, len(merged)+len(explicit))
	for _, kv := range merged {
		if _, overridden := explicitKeys[kv.key.Value]; !overridden {
			out = append(out, kv)
		}
	}
	return append(out, explicit...)
}

func extractMergePairs(val *yaml.Node) []kvPair {
	switch val.Kind {
	case yaml.AliasNode:
		if val.Alias != nil {
			return extractMergePairs(val.Alias)
		}
	case yaml.MappingNode:
		return expandMappingWithMerges(val)
	case yaml.SequenceNode:
		var out []kvPair
		for _, item := range val.Content {
			out = appendUniquePairs(out, extractMergePairs(item))
		}
		return out
	}
	return nil
}

func appendUniquePairs(dst, src []kvPair) []kvPair {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, kv := range dst {
		seen[kv.key.Value] = struct{}{}
	}
	for _, kv := range src {
		if _, exists := seen[kv.key.Value]; exists {
			continue
		}
		seen[kv.key.Value] = struct{}{}
		dst = append(dst, kv)
	}
	return dst
}

// dedupePairsKeepLast is used only after duplicate explicit keys have already
// been reported. Keeping the last value lets validation continue deterministically.
func dedupePairsKeepLast(pairs []kvPair) []kvPair {
	seen := make(map[string]int)
	for idx, kv := range pairs {
		seen[kv.key.Value] = idx
	}
	out := make([]kvPair, 0, len(seen))
	for idx, kv := range pairs {
		if seen[kv.key.Value] == idx {
			out = append(out, kv)
		}
	}
	return out
}

func (v *Validator) checkDuplicateKeysRecursive(node *yaml.Node, path string, ctx *ValidationContext,
	visited map[*yaml.Node]bool, depth int) {

	if node == nil || ctx.IsStopped() || visited[node] {
		return
	}
	if ctx.MaxDepth > 0 && depth > ctx.MaxDepth {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "max_depth",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "YAML value exceeds configured nesting depth",
			Expected: fmt.Sprintf("depth <= %d", ctx.MaxDepth),
			Got:      fmt.Sprintf("depth %d", depth),
		})
		ctx.stopped = true
		return
	}
	visited[node] = true

	if node.Kind == yaml.AliasNode {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]*yaml.Node)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := keyNode.Value
			fieldPath := joinPath(path, key)
			if keyNode.Kind == yaml.ScalarNode {
				if first := seen[key]; first != nil {
					ctx.AddError(ValidationError{
						Level:   LevelError,
						Code:    "duplicate_key",
						Path:    cleanPath(fieldPath),
						Line:    keyNode.Line,
						Column:  keyNode.Column,
						Message: fmt.Sprintf("duplicate key %q; first defined at line %d:%d", key, first.Line, first.Column),
					})
					if ctx.IsStopped() {
						return
					}
				} else {
					seen[key] = keyNode
				}
			}
			v.checkDuplicateKeysRecursive(valueNode, fieldPath, ctx, visited, depth+1)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			v.checkDuplicateKeysRecursive(child, fmt.Sprintf("%s[%d]", path, i), ctx, visited, depth+1)
		}
	}
}

func (v *Validator) checkAliasSafety(node *yaml.Node, path string, ctx *ValidationContext, state map[*yaml.Node]uint8) {
	if node == nil || ctx.IsStopped() {
		return
	}
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Code:    "unresolved_alias",
				Path:    cleanPath(path),
				Line:    node.Line,
				Column:  node.Column,
				Message: "unresolved YAML alias",
			})
			ctx.stopped = true
			return
		}
		if state[node.Alias] == 1 {
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Code:    "recursive_alias",
				Path:    cleanPath(path),
				Line:    node.Line,
				Column:  node.Column,
				Message: "recursive YAML alias is not supported",
			})
			ctx.stopped = true
			return
		}
		v.checkAliasSafety(node.Alias, path, ctx, state)
		return
	}
	if state[node] == 2 {
		return
	}
	if state[node] == 1 {
		return
	}
	state[node] = 1
	defer func() { state[node] = 2 }()

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			fieldPath := joinPath(path, keyNode.Value)
			if keyNode.Value == "<<" && !validMergeValue(valueNode, make(map[*yaml.Node]bool)) {
				ctx.AddError(ValidationError{
					Level:   LevelError,
					Code:    "invalid_merge",
					Path:    cleanPath(fieldPath),
					Line:    valueNode.Line,
					Column:  valueNode.Column,
					Message: "YAML merge value must be a mapping, mapping alias, or sequence of mappings/aliases",
				})
				if ctx.IsStopped() {
					return
				}
			}
			v.checkAliasSafety(valueNode, fieldPath, ctx, state)
			if ctx.IsStopped() {
				return
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			v.checkAliasSafety(child, fmt.Sprintf("%s[%d]", path, i), ctx, state)
			if ctx.IsStopped() {
				return
			}
		}
	}
}

func validMergeValue(node *yaml.Node, visiting map[*yaml.Node]bool) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil || visiting[node.Alias] {
			return false
		}
		visiting[node.Alias] = true
		defer delete(visiting, node.Alias)
		return node.Alias.Kind == yaml.MappingNode
	}
	switch node.Kind {
	case yaml.MappingNode:
		return true
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if !validMergeValue(item, visiting) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (v *Validator) resolveUnknownKeyLevel(policy UnknownKeyPolicy, ctx *ValidationContext) (ErrorLevel, bool) {
	switch policy {
	case UnknownKeyError:
		return LevelError, true
	case UnknownKeyWarn:
		return LevelWarning, true
	case UnknownKeyIgnore:
		return 0, false
	case UnknownKeyInherit:
		fallthrough
	default:
		if ctx.StrictKeys {
			return LevelError, true
		}
		return LevelWarning, true
	}
}

func (v *Validator) checkRequiredFields(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	for key, fieldSchema := range schema.AllowedKeys {
		if ctx.IsStopped() {
			return
		}
		if fieldSchema.Required && foundKeys[key] == nil {
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Code:    "required",
				Path:    cleanPath(joinPath(path, key)),
				Line:    node.Line,
				Column:  node.Column,
				Message: fmt.Sprintf("required field %q is missing", key),
				Details: RequiredDetails{Field: key},
			})
		}
	}
}

func (v *Validator) checkDefaults(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	for key, fieldSchema := range schema.AllowedKeys {
		if ctx.IsStopped() {
			return
		}
		if fieldSchema.Default != nil && foundKeys[key] == nil && !fieldSchema.Required {
			ctx.AddError(ValidationError{
				Level:   LevelWarning,
				Code:    "default",
				Path:    cleanPath(joinPath(path, key)),
				Line:    node.Line,
				Column:  node.Column,
				Message: fmt.Sprintf("field %q not set; default is %v", key, fieldSchema.Default),
			})
		}
	}
}

func (v *Validator) checkAnyOf(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	if len(schema.AnyOf) == 0 {
		return
	}

	for _, group := range schema.AnyOf {
		if ctx.IsStopped() {
			return
		}
		allPresent := true
		for _, key := range group {
			if foundKeys[key] == nil {
				allPresent = false
				break
			}
		}
		if allPresent {
			return // At least one group is fully present
		}
	}

	// No group is fully present
	var groupStrs []string
	for _, g := range schema.AnyOf {
		if len(g) == 1 {
			groupStrs = append(groupStrs, fmt.Sprintf("%q", g[0]))
		} else {
			groupStrs = append(groupStrs, fmt.Sprintf("(%s)", strings.Join(quoteAll(g), " and ")))
		}
	}

	ctx.AddError(ValidationError{
		Level:   LevelError,
		Code:    "any_of_required",
		Path:    cleanPath(path),
		Line:    node.Line,
		Column:  node.Column,
		Message: fmt.Sprintf("at least one of %s is required", strings.Join(groupStrs, " or ")),
	})
}

func (v *Validator) checkExactlyOneOf(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	if len(schema.ExactlyOneOf) == 0 {
		return
	}

	var found []string
	for _, key := range schema.ExactlyOneOf {
		if foundKeys[key] != nil {
			found = append(found, key)
		}
	}

	if len(found) == 0 {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "exactly_one_of",
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: fmt.Sprintf("exactly one of %v is required, none found", schema.ExactlyOneOf),
		})
	} else if len(found) > 1 {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "exactly_one_of",
			Path:    cleanPath(path),
			Line:    keyNodes[found[1]].Line,
			Column:  keyNodes[found[1]].Column,
			Message: fmt.Sprintf("exactly one of %v is required, found: %v", schema.ExactlyOneOf, found),
		})
	}
}

func (v *Validator) checkMutuallyExclusive(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	if len(schema.MutuallyExclusive) == 0 {
		return
	}

	var found []string
	for _, key := range schema.MutuallyExclusive {
		if foundKeys[key] != nil {
			found = append(found, key)
		}
	}

	if len(found) > 1 {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "mutually_exclusive",
			Path:    cleanPath(path),
			Line:    keyNodes[found[1]].Line,
			Column:  keyNodes[found[1]].Column,
			Message: fmt.Sprintf("fields %v are mutually exclusive", found),
		})
	}
}

func (v *Validator) checkOneOfRequired(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	if len(schema.OneOfRequired) == 0 {
		return
	}

	matches := 0
	for _, group := range schema.OneOfRequired {
		allPresent := true
		for _, key := range group {
			if foundKeys[key] == nil {
				allPresent = false
				break
			}
		}
		if allPresent {
			matches++
		}
	}
	if matches == 1 {
		return
	}

	ctx.AddError(ValidationError{
		Level:   LevelError,
		Code:    "one_of_required",
		Path:    cleanPath(path),
		Line:    node.Line,
		Column:  node.Column,
		Message: fmt.Sprintf("exactly one required field group must match, got %d", matches),
	})
}

func (v *Validator) checkForbiddenTogether(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	for _, group := range schema.ForbiddenTogether {
		if ctx.IsStopped() {
			return
		}
		allPresent := len(group) > 0
		for _, key := range group {
			if foundKeys[key] == nil {
				allPresent = false
				break
			}
		}
		if !allPresent {
			continue
		}
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Code:    "forbidden_together",
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: fmt.Sprintf("fields %v must not be present together", group),
		})
		if ctx.IsStopped() {
			return
		}
	}
}

func (v *Validator) checkDependentRequired(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	for trigger, required := range schema.DependentRequired {
		if ctx.IsStopped() {
			return
		}
		if foundKeys[trigger] == nil {
			continue
		}
		anchor := keyNodes[trigger]
		for _, key := range required {
			if foundKeys[key] != nil {
				continue
			}
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Code:    "dependent_required",
				Path:    cleanPath(joinPath(path, key)),
				Line:    anchor.Line,
				Column:  anchor.Column,
				Message: fmt.Sprintf("field %q is required when %q is present", key, trigger),
				Details: DependencyDetails{
					Field:   key,
					Trigger: trigger,
				},
			})
			if ctx.IsStopped() {
				return
			}
		}
	}
}

func (v *Validator) checkConditions(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

	if ctx.IsStopped() {
		return
	}
	for _, rule := range schema.Conditions {
		if ctx.IsStopped() {
			return
		}
		condNode := foundKeys[rule.ConditionField]
		if condNode == nil {
			continue
		}

		// Conditions only apply to scalars
		if condNode.Kind != yaml.ScalarNode {
			continue
		}

		if condNode.Value != rule.ConditionValue {
			continue
		}

		// ThenRequired
		for _, reqKey := range rule.ThenRequired {
			if foundKeys[reqKey] == nil {
				ctx.AddError(ValidationError{
					Level:  LevelError,
					Code:   "condition_required",
					Path:   cleanPath(joinPath(path, reqKey)),
					Line:   condNode.Line,
					Column: condNode.Column,
					Message: fmt.Sprintf("field %q is required when %s=%q",
						reqKey, rule.ConditionField, rule.ConditionValue),
				})
			}
		}

		// ThenForbidden
		for _, forbKey := range rule.ThenForbidden {
			if keyNode := keyNodes[forbKey]; keyNode != nil {
				ctx.AddError(ValidationError{
					Level:  LevelError,
					Code:   "condition_forbidden",
					Path:   cleanPath(joinPath(path, forbKey)),
					Line:   keyNode.Line,
					Column: keyNode.Column,
					Message: fmt.Sprintf("field %q is forbidden when %s=%q",
						forbKey, rule.ConditionField, rule.ConditionValue),
				})
			}
		}
	}
}

// ============================================================================
// Sequence Validation
// ============================================================================

func (v *Validator) validateSequence(node *yaml.Node, schema *FieldSchema, path string, ctx *ValidationContext) {
	length := len(node.Content)

	if schema.MinItems != nil && length < *schema.MinItems {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "min_items",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "too few items",
			Expected: fmt.Sprintf("at least %d", *schema.MinItems),
			Got:      fmt.Sprintf("%d", length),
			Details: ItemCountDetails{
				Actual: length,
				Bound:  *schema.MinItems,
			},
		})
	}

	if schema.MaxItems != nil && length > *schema.MaxItems {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Code:     "max_items",
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "too many items",
			Expected: fmt.Sprintf("at most %d", *schema.MaxItems),
			Got:      fmt.Sprintf("%d", length),
			Details: ItemCountDetails{
				Actual: length,
				Bound:  *schema.MaxItems,
			},
		})
	}

	if schema.ItemSchema == nil {
		return
	}

	for i, item := range node.Content {
		if ctx.IsStopped() {
			return
		}
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		v.validateNode(item, schema.ItemSchema, itemPath, ctx)
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func joinPath(base, key string) string {
	if isSimplePathSegment(key) && !(base == "" && key == "doc") {
		if base == "" {
			return key
		}
		return base + "." + key
	}
	encoded, _ := json.Marshal(key)
	return base + "[" + string(encoded) + "]"
}

func cleanPath(path string) string {
	return strings.TrimPrefix(path, ".")
}

func splitLines(data []byte) []string {
	raw := bytes.Split(data, []byte("\n"))
	lines := make([]string, len(raw))
	for i, line := range raw {
		line = bytes.TrimSuffix(line, []byte("\r"))
		lines[i] = string(line)
	}
	return lines
}

func suggestKnownKey(key string, allowed map[string]*FieldSchema) string {
	if len(allowed) == 0 {
		return ""
	}
	lowerKey := strings.ToLower(key)
	best := ""
	bestDistance := -1
	for candidate := range allowed {
		distance := levenshteinDistance(lowerKey, strings.ToLower(candidate))
		if bestDistance == -1 || distance < bestDistance || distance == bestDistance && candidate < best {
			best, bestDistance = candidate, distance
		}
	}
	limit := 2
	if utf8.RuneCountInString(key) >= 8 {
		limit = 3
	}
	if bestDistance > limit {
		return ""
	}
	return best
}

func levenshteinDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, ra := range ar {
		current[0] = i + 1
		for j, rb := range br {
			cost := 0
			if ra != rb {
				cost = 1
			}
			deletion := previous[j+1] + 1
			insertion := current[j] + 1
			substitution := previous[j] + cost
			current[j+1] = minInt(deletion, insertion, substitution)
		}
		previous, current = current, previous
	}
	return previous[len(br)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// Package-level compiled regexes
var (
	yamlErrorLineColRe = regexp.MustCompile(`line (\d+):\s*column (\d+)`)
	yamlErrorLineRe    = regexp.MustCompile(`line (\d+):`)
)

func parseYAMLError(err error, docIndex int) ValidationError {
	msg := err.Error()
	line, col := 0, 0

	if m := yamlErrorLineColRe.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
		col, _ = strconv.Atoi(m[2])
	} else if m := yamlErrorLineRe.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
	}

	return ValidationError{
		Level:   LevelError,
		Code:    "yaml_syntax",
		Path:    fmt.Sprintf("doc[%d]", docIndex),
		Line:    line,
		Column:  col,
		Message: msg,
	}
}

// ============================================================================
// Error Formatting
// ============================================================================

const tabWidth = 4

// renderLineWithCaret renders a line with tabs expanded to tabstops
// and calculates the visual caret position.
//
// byteCol: position in bytes (1-based), as returned by yaml.v3.
// Interpretation: "between bytes":
//
//	1             -> before first byte
//	len(line)+1   -> after last byte
//
// If byteCol falls inside a multi-byte rune, caret is placed before that rune.
func renderLineWithCaret(line string, byteCol int) (rendered string, visualCol int, renderedLen int) {
	var sb strings.Builder
	sb.Grow(len(line) + 16)

	if byteCol > len(line)+1 {
		byteCol = len(line) + 1
	}

	visual := 0
	caretSet := false

	for bytePos := 0; bytePos < len(line); {
		if !caretSet && byteCol > 0 && (byteCol-1) <= bytePos {
			visualCol = visual + 1
			caretSet = true
		}

		r, size := utf8.DecodeRuneInString(line[bytePos:])
		if r == utf8.RuneError && size == 1 {
			r = '�'
		}

		if r == '\t' {
			spacesToAdd := tabWidth - (visual % tabWidth)
			sb.WriteString(strings.Repeat(" ", spacesToAdd))
			visual += spacesToAdd
		} else {
			sb.WriteRune(r)
			visual++
		}

		bytePos += size
	}

	if !caretSet && byteCol > 0 {
		visualCol = visual + 1
	}

	return sb.String(), visualCol, visual
}

// RenderLineWithCaret is an exported wrapper useful for external tests and tools.
func RenderLineWithCaret(line string, byteCol int) (string, int, int) {
	return renderLineWithCaret(line, byteCol)
}

// FormatErrorWithSource formats an error with source context.
// Correctly handles tabs and Unicode.
func FormatErrorWithSource(err ValidationError, lines []string) string {
	var sb strings.Builder
	sb.WriteString(err.Error())
	sb.WriteString("\n")

	if err.Line <= 0 || err.Line > len(lines) {
		return sb.String()
	}

	lineIdx := err.Line - 1

	// Context: line before
	if lineIdx > 0 {
		prevRendered, _, _ := renderLineWithCaret(lines[lineIdx-1], 0)
		sb.WriteString(fmt.Sprintf("  %4d | %s\n", err.Line-1, prevRendered))
	}

	// Current line with caret
	currentRendered, visualCol, renderedLen := renderLineWithCaret(lines[lineIdx], err.Column)
	sb.WriteString(fmt.Sprintf("> %4d | %s\n", err.Line, currentRendered))

	// Caret with bounds protection
	if visualCol > 0 {
		if visualCol > renderedLen+1 {
			visualCol = renderedLen + 1
		}
		sb.WriteString(fmt.Sprintf("       | %s^\n", strings.Repeat(" ", visualCol-1)))
	}

	// Context: line after
	if lineIdx+1 < len(lines) {
		nextRendered, _, _ := renderLineWithCaret(lines[lineIdx+1], 0)
		sb.WriteString(fmt.Sprintf("  %4d | %s\n", err.Line+1, nextRendered))
	}

	return sb.String()
}
