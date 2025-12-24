// Package yamlvalidator provides a flexible YAML validation library
// with support for type checking, custom validators, conditional logic,
// and detailed error reporting with source context.
package yamlvalidator

import (
	"bufio"
	"bytes"
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
	Level    ErrorLevel
	Path     string // Path to the problematic node, e.g., "spec.containers[0].image"
	Line     int    // 1-based line number (0 if unknown)
	Column   int    // 1-based column number (0 if unknown)
	Message  string
	Got      string // Actual value/type description
	Expected string // Expected value/type description
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
}

// NewErrorCollector creates a new empty ErrorCollector.
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{}
}

// Add adds a validation error to the collector.
func (c *ErrorCollector) Add(err ValidationError) {
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

// Errors returns all errors.
func (c *ErrorCollector) Errors() []ValidationError {
	return c.errors
}

// Warnings returns all warnings.
func (c *ErrorCollector) Warnings() []ValidationError {
	return c.warnings
}

// All returns all errors followed by all warnings.
func (c *ErrorCollector) All() []ValidationError {
	result := make([]ValidationError, 0, len(c.errors)+len(c.warnings))
	result = append(result, c.errors...)
	result = append(result, c.warnings...)
	return result
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

	// SourceLines contains the original YAML lines for error formatting.
	SourceLines []string

	collector *ErrorCollector
	stopped   bool
}

// NewValidationContext creates a new ValidationContext with default settings.
func NewValidationContext() *ValidationContext {
	return &ValidationContext{
		collector: NewErrorCollector(),
	}
}

// AddError adds an error to the context's collector.
func (ctx *ValidationContext) AddError(err ValidationError) {
	if ctx.stopped {
		return
	}
	ctx.collector.Add(err)
	if ctx.StopOnFirst && err.Level == LevelError {
		ctx.stopped = true
	}
}

// IsStopped returns true if validation has been stopped.
func (ctx *ValidationContext) IsStopped() bool {
	return ctx.stopped
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

// ValueValidator validates a node's value.
type ValueValidator interface {
	Validate(node *yaml.Node, path string, ctx *ValidationContext)
}

// KeyValidator validates key names in mappings.
type KeyValidator interface {
	ValidateKey(key string, keyNode *yaml.Node, path string, ctx *ValidationContext)
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

// NewValidator creates a new Validator with the given schema.
func NewValidator(schema *FieldSchema) *Validator {
	return &Validator{schema: schema}
}

// ValidateBytes validates YAML data and returns the result.
// Supports multi-document YAML (separated by ---).
func (v *Validator) ValidateBytes(data []byte) *ValidationResult {
	ctx := NewValidationContext()
	ctx.SourceLines = splitLines(data)
	v.validateWithContext(bytes.NewReader(data), ctx)
	return &ValidationResult{
		Collector:   ctx.Collector(),
		SourceLines: ctx.SourceLines,
	}
}

// ValidateWithOptions validates YAML data with custom options.
func (v *Validator) ValidateWithOptions(data []byte, opts ValidationContext) *ValidationResult {
	ctx := &opts
	ctx.collector = NewErrorCollector()
	ctx.SourceLines = splitLines(data)
	v.validateWithContext(bytes.NewReader(data), ctx)
	return &ValidationResult{
		Collector:   ctx.Collector(),
		SourceLines: ctx.SourceLines,
	}
}

func (v *Validator) validateWithContext(r io.Reader, ctx *ValidationContext) {
	decoder := yaml.NewDecoder(r)
	docIndex := 0

	for {
		var root yaml.Node
		err := decoder.Decode(&root)
		if err == io.EOF {
			break
		}
		if err != nil {
			ctx.AddError(parseYAMLError(err, docIndex))
			return
		}

		if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
			prefix := ""
			if docIndex > 0 {
				prefix = fmt.Sprintf("doc[%d]", docIndex)
			}
			v.validateNode(root.Content[0], v.schema, prefix, ctx)
		}

		docIndex++
		if ctx.IsStopped() {
			break
		}
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

	// Resolve aliases
	if node.Kind == yaml.AliasNode {
		if node.Alias != nil {
			node = node.Alias
		} else {
			ctx.AddError(ValidationError{
				Level:   LevelError,
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
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: msg,
		})
	}

	// Type check
	if !v.checkTypeWithSchema(node, schema, path, ctx) {
		return
	}

	// Structure validation
	switch node.Kind {
	case yaml.MappingNode:
		v.validateMapping(node, schema, path, ctx)
	case yaml.SequenceNode:
		v.validateSequence(node, schema, path, ctx)
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

func (v *Validator) checkTypeWithSchema(node *yaml.Node, schema *FieldSchema, path string, ctx *ValidationContext) bool {
	expected := schema.Type

	if expected == TypeAny {
		return true
	}

	actual := v.inferType(node, ctx)

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
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "unexpected null value",
			Expected: expected.String(),
			Got:      "null",
		})
		return false
	}

	if actual == expected {
		return true
	}

	// Float accepts int
	if expected == TypeFloat && actual == TypeInt {
		return true
	}

	ctx.AddError(ValidationError{
		Level:    LevelError,
		Path:     cleanPath(path),
		Line:     node.Line,
		Column:   node.Column,
		Message:  "type mismatch",
		Expected: expected.String(),
		Got:      v.describeNode(node),
	})
	return false
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
		if ctx.YAML11Booleans {
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
		if len(val) > 20 {
			val = val[:20] + "..."
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
		for _, kv := range schema.KeyValidators {
			kv.ValidateKey(key, keyNode, cleanPath(fieldPath), ctx)
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
			ctx.AddError(ValidationError{
				Level:   level,
				Path:    cleanPath(fieldPath),
				Line:    keyNode.Line,
				Column:  keyNode.Column,
				Message: fmt.Sprintf("unknown key %q", key),
				Got:     v.describeNode(valueNode),
			})
		}
	}

	// Check required fields, defaults, and inter-field logic
	v.checkRequiredFields(node, schema, path, foundKeys, ctx)
	v.checkDefaults(node, schema, path, foundKeys, ctx)
	v.checkAnyOf(node, schema, path, foundKeys, ctx)
	v.checkExactlyOneOf(node, schema, path, foundKeys, keyNodes, ctx)
	v.checkMutuallyExclusive(node, schema, path, foundKeys, keyNodes, ctx)
	v.checkConditions(node, schema, path, foundKeys, keyNodes, ctx)
}

type kvPair struct {
	key   *yaml.Node
	value *yaml.Node
}

// expandMappingWithMerges expands YAML merge keys (<<) into concrete key/value pairs.
// Later merges override earlier ones; explicit keys override merges.
func expandMappingWithMerges(node *yaml.Node) []kvPair {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	var pairs []kvPair
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if keyNode.Value == "<<" {
			mergePairs := extractMergePairs(valueNode)
			pairs = append(pairs, mergePairs...)
			continue
		}
		pairs = append(pairs, kvPair{key: keyNode, value: valueNode})
	}
	return dedupePairsKeepLast(pairs)
}

func extractMergePairs(val *yaml.Node) []kvPair {
	switch val.Kind {
	case yaml.AliasNode:
		if val.Alias != nil {
			return extractMergePairs(val.Alias)
		}
	case yaml.MappingNode:
		return mappingToPairs(val)
	case yaml.SequenceNode:
		var out []kvPair
		for _, item := range val.Content {
			out = append(out, extractMergePairs(item)...)
		}
		return out
	}
	return nil
}

func mappingToPairs(m *yaml.Node) []kvPair {
	var out []kvPair
	for i := 0; i < len(m.Content); i += 2 {
		out = append(out, kvPair{key: m.Content[i], value: m.Content[i+1]})
	}
	return out
}

// dedupePairsKeepLast keeps the last occurrence of each key to model merge override and explicit override.
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
		if fieldSchema.Required && foundKeys[key] == nil {
			ctx.AddError(ValidationError{
				Level:   LevelError,
				Path:    cleanPath(joinPath(path, key)),
				Line:    node.Line,
				Column:  node.Column,
				Message: fmt.Sprintf("required field %q is missing", key),
			})
		}
	}
}

func (v *Validator) checkDefaults(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	for key, fieldSchema := range schema.AllowedKeys {
		if fieldSchema.Default != nil && foundKeys[key] == nil && !fieldSchema.Required {
			ctx.AddError(ValidationError{
				Level:   LevelWarning,
				Path:    cleanPath(joinPath(path, key)),
				Line:    node.Line,
				Column:  node.Column,
				Message: fmt.Sprintf("field %q not set, will use default: %v", key, fieldSchema.Default),
			})
		}
	}
}

func (v *Validator) checkAnyOf(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, ctx *ValidationContext) {

	if len(schema.AnyOf) == 0 {
		return
	}

	for _, group := range schema.AnyOf {
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
		Path:    cleanPath(path),
		Line:    node.Line,
		Column:  node.Column,
		Message: fmt.Sprintf("at least one of %s is required", strings.Join(groupStrs, " or ")),
	})
}

func (v *Validator) checkExactlyOneOf(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

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
			Path:    cleanPath(path),
			Line:    node.Line,
			Column:  node.Column,
			Message: fmt.Sprintf("exactly one of %v is required, none found", schema.ExactlyOneOf),
		})
	} else if len(found) > 1 {
		ctx.AddError(ValidationError{
			Level:   LevelError,
			Path:    cleanPath(path),
			Line:    keyNodes[found[1]].Line,
			Column:  keyNodes[found[1]].Column,
			Message: fmt.Sprintf("exactly one of %v is required, found: %v", schema.ExactlyOneOf, found),
		})
	}
}

func (v *Validator) checkMutuallyExclusive(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

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
			Path:    cleanPath(path),
			Line:    keyNodes[found[1]].Line,
			Column:  keyNodes[found[1]].Column,
			Message: fmt.Sprintf("fields %v are mutually exclusive", found),
		})
	}
}

func (v *Validator) checkConditions(node *yaml.Node, schema *FieldSchema, path string,
	foundKeys map[string]*yaml.Node, keyNodes map[string]*yaml.Node, ctx *ValidationContext) {

	for _, rule := range schema.Conditions {
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
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "too few items",
			Expected: fmt.Sprintf("at least %d", *schema.MinItems),
			Got:      fmt.Sprintf("%d", length),
		})
	}

	if schema.MaxItems != nil && length > *schema.MaxItems {
		ctx.AddError(ValidationError{
			Level:    LevelError,
			Path:     cleanPath(path),
			Line:     node.Line,
			Column:   node.Column,
			Message:  "too many items",
			Expected: fmt.Sprintf("at most %d", *schema.MaxItems),
			Got:      fmt.Sprintf("%d", length),
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
	if base == "" {
		return key
	}
	return base + "." + key
}

func cleanPath(path string) string {
	return strings.TrimPrefix(path, ".")
}

func splitLines(data []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
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
