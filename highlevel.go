package yamlvalidator

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Limits bounds high-level validation work. A zero field selects its default.
type Limits struct {
	MaxBytes       int
	MaxDepth       int
	MaxDiagnostics int
	MaxNodeVisits  int
}

// SchemaError describes an invalid Go type, field, or validation declaration.
type SchemaError struct {
	Type   reflect.Type
	Field  string
	Tag    string
	Offset int
	Reason string
}

func (e *SchemaError) Error() string {
	if e == nil {
		return "<nil>"
	}
	where := "schema"
	if e.Type != nil {
		where = e.Type.String()
	}
	if e.Field != "" {
		where += "." + e.Field
	}
	if e.Tag != "" {
		where += fmt.Sprintf(" tag %q at byte %d", e.Tag, e.Offset)
	}
	return where + ": " + e.Reason
}

// ValidationErrors groups the final diagnostics from one high-level operation.
type ValidationErrors struct {
	diagnostics []ValidationError
	source      []string
	generated   bool
	truncated   bool
}

func (e *ValidationErrors) Error() string {
	if e == nil {
		return "YAML validation failed"
	}
	if e.truncated && len(e.diagnostics) == 0 {
		return "YAML validation incomplete"
	}
	if len(e.diagnostics) == 0 {
		return "YAML validation failed"
	}
	if e.truncated {
		return fmt.Sprintf("YAML validation incomplete after %d diagnostics: %s", len(e.diagnostics), e.diagnostics[0].Error())
	}
	if len(e.diagnostics) == 1 {
		return e.diagnostics[0].Error()
	}
	return fmt.Sprintf("YAML validation failed with %d diagnostics: %s", len(e.diagnostics), e.diagnostics[0].Error())
}

// Diagnostics returns defensive copies in validation order.
func (e *ValidationErrors) Diagnostics() []ValidationError { return cloneDiagnostics(e.diagnostics) }

// FormatWithSource renders diagnostics with source snippets and carets.
func (e *ValidationErrors) FormatWithSource() string {
	if e == nil {
		return ""
	}
	var out strings.Builder
	if e.generated {
		out.WriteString("generated YAML:\n")
	}
	for _, d := range e.diagnostics {
		out.WriteString(FormatErrorWithSource(d, e.source))
		out.WriteByte('\n')
	}
	return out.String()
}

// GeneratedSource reports whether source positions refer to Marshal output.
func (e *ValidationErrors) GeneratedSource() bool { return e != nil && e.generated }

// Truncated reports that validation stopped at a diagnostic or visit limit.
func (e *ValidationErrors) Truncated() bool { return e != nil && e.truncated }

// UnmarshalOptions controls validated YAML decoding. Its zero value enables validation.
type UnmarshalOptions struct {
	UnknownKeyPolicy UnknownKeyPolicy
	WarningsAsErrors bool
	OnDiagnostic     func(ValidationError)
	StopOnFirst      bool
	Limits           Limits
	Registry         *Registry
	Schema           *Validator
	SkipValidation   bool
}

// MarshalOptions controls validated YAML encoding. Its zero value enables validation.
type MarshalOptions struct {
	UnknownKeyPolicy UnknownKeyPolicy
	WarningsAsErrors bool
	OnDiagnostic     func(ValidationError)
	StopOnFirst      bool
	Limits           Limits
	Registry         *Registry
	Schema           *Validator
	SkipValidation   bool
	Indent           int
}

// Unmarshal decodes one YAML document after validating it.
func Unmarshal(data []byte, out any) error { return (UnmarshalOptions{}).Unmarshal(data, out) }

// Marshal encodes and validates a Go value, returning the exact validated bytes.
func Marshal(in any) ([]byte, error) { return (MarshalOptions{}).Marshal(in) }

func normalizeLimits(l Limits) (Limits, error) {
	if l.MaxBytes < 0 || l.MaxDepth < 0 || l.MaxDiagnostics < 0 || l.MaxNodeVisits < 0 {
		return l, fmt.Errorf("high-level limits must be non-negative")
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = 8 << 20
	}
	if l.MaxDepth == 0 {
		l.MaxDepth = 128
	}
	if l.MaxDiagnostics == 0 {
		l.MaxDiagnostics = 100
	}
	if l.MaxNodeVisits == 0 {
		l.MaxNodeVisits = 1_000_000
	}
	return l, nil
}

func validateHighLevelPolicy(policy UnknownKeyPolicy) error {
	if policy < UnknownKeyInherit || policy > UnknownKeyIgnore {
		return fmt.Errorf("invalid unknown key policy %d", policy)
	}
	return nil
}

func (o UnmarshalOptions) Unmarshal(data []byte, out any) error {
	limits, err := normalizeLimits(o.Limits)
	if err != nil {
		return err
	}
	if err := validateHighLevelPolicy(o.UnknownKeyPolicy); err != nil {
		return err
	}
	dst := reflect.ValueOf(out)
	if !dst.IsValid() || dst.Kind() != reflect.Pointer || dst.IsNil() {
		return fmt.Errorf("Unmarshal destination must be a non-nil pointer")
	}
	if len(data) > limits.MaxBytes {
		return fmt.Errorf("YAML input exceeds %d bytes", limits.MaxBytes)
	}
	var plan *highLevelPlan
	if !o.SkipValidation {
		plan, err = compileHighLevel(dst.Elem().Type(), false, o.Registry)
		if err != nil {
			return err
		}
	}
	root, err := parseSingleDocument(data)
	if err != nil {
		return err
	}
	if !o.SkipValidation {
		opts := highLevelRunOptions{o.UnknownKeyPolicy, o.WarningsAsErrors, o.OnDiagnostic, o.StopOnFirst, limits, o.Schema, false}
		if err := validateHighLevelDocument(root, plan, data, opts); err != nil {
			return err
		}
	}
	if err := root.Decode(out); err != nil {
		return fmt.Errorf("decode YAML: %w", err)
	}
	return nil
}

func (o MarshalOptions) Marshal(in any) ([]byte, error) {
	limits, err := normalizeLimits(o.Limits)
	if err != nil {
		return nil, err
	}
	if err := validateHighLevelPolicy(o.UnknownKeyPolicy); err != nil {
		return nil, err
	}
	if o.Indent < 0 {
		return nil, fmt.Errorf("YAML indentation must be non-negative")
	}
	var plan *highLevelPlan
	if !o.SkipValidation && in != nil {
		plan, err = compileHighLevel(reflect.TypeOf(in), true, o.Registry)
		if err != nil {
			return nil, err
		}
	}
	if err := rejectGoCycles(reflect.ValueOf(in), limits); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	if o.Indent != 0 {
		encoder.SetIndent(o.Indent)
	}
	if err := encoder.Encode(in); err != nil {
		return nil, fmt.Errorf("encode YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close YAML encoder: %w", err)
	}
	data := buffer.Bytes()
	if len(data) > limits.MaxBytes {
		return nil, fmt.Errorf("generated YAML exceeds %d bytes", limits.MaxBytes)
	}
	if o.SkipValidation {
		return data, nil
	}
	root, err := parseSingleDocument(data)
	if err != nil {
		return nil, err
	}
	opts := highLevelRunOptions{o.UnknownKeyPolicy, o.WarningsAsErrors, o.OnDiagnostic, o.StopOnFirst, limits, o.Schema, true}
	if err := validateHighLevelDocument(root, plan, data, opts); err != nil {
		return nil, err
	}
	return data, nil
}

func parseSingleDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("YAML document is empty")
		}
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil, fmt.Errorf("YAML document is empty")
	}
	var next yaml.Node
	if err := decoder.Decode(&next); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse second YAML document: %w", err)
		}
		return nil, fmt.Errorf("expected one YAML document, got more than one")
	}
	return &root, nil
}

type highLevelRunOptions struct {
	policy           UnknownKeyPolicy
	warningsAsErrors bool
	callback         func(ValidationError)
	stopOnFirst      bool
	limits           Limits
	schema           *Validator
	generated        bool
}

func validateHighLevelDocument(root *yaml.Node, plan *highLevelPlan, data []byte, opts highLevelRunOptions) error {
	ctx := NewValidationContext()
	ctx.StrictKeys = true
	ctx.StopOnFirst = opts.stopOnFirst
	ctx.StrictTypes = true
	ctx.UnknownKeyPolicy = opts.policy
	ctx.MaxDepth = opts.limits.MaxDepth
	ctx.MaxDiagnostics = opts.limits.MaxDiagnostics
	ctx.MaxNodeVisits = opts.limits.MaxNodeVisits
	if opts.policy == UnknownKeyWarn {
		ctx.StrictKeys = false
	}
	ctx.sourceLines = splitLines(data)
	validator := &Validator{}
	if plan != nil {
		validator.schema = plan.schema
	}
	value := root.Content[0]
	validator.validateParsedNode(value, "", ctx)
	if !ctx.IsStopped() && opts.schema != nil {
		opts.schema.validateNode(value, opts.schema.schema, "", ctx)
	}
	diagnostics := ctx.Collector().All()
	for _, d := range diagnostics {
		if opts.callback != nil {
			opts.callback(cloneDiagnostics([]ValidationError{d})[0])
		}
	}
	failed := ctx.Collector().HasErrors() || ctx.limitReached || ctx.contextErr != nil
	if opts.warningsAsErrors && len(diagnostics) > 0 {
		failed = true
	}
	if !failed {
		return nil
	}
	return &ValidationErrors{diagnostics: diagnostics, source: splitLines(data), generated: opts.generated, truncated: ctx.limitReached}
}

func rejectGoCycles(value reflect.Value, limits Limits) error {
	type identity struct {
		typ     reflect.Type
		pointer uintptr
		length  int
	}
	seen := map[identity]bool{}
	visits := 0
	var walk func(reflect.Value, int) error
	walk = func(v reflect.Value, depth int) error {
		if !v.IsValid() {
			return nil
		}
		visits++
		if visits > limits.MaxNodeVisits {
			return fmt.Errorf("Go value exceeds %d visits", limits.MaxNodeVisits)
		}
		if depth > limits.MaxDepth {
			return fmt.Errorf("Go value exceeds depth %d", limits.MaxDepth)
		}
		for v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		if v.Type().Implements(yamlMarshalerType) || v.Type().Implements(textMarshalerType) {
			return nil
		}
		var pointer uintptr
		switch v.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
			if v.IsNil() {
				return nil
			}
			if v.Kind() == reflect.Slice && v.Len() == 0 {
				return nil
			}
			pointer = v.Pointer()
		}
		if pointer != 0 {
			length := 0
			if v.Kind() == reflect.Slice {
				length = v.Len()
			}
			key := identity{v.Type(), pointer, length}
			if seen[key] {
				return fmt.Errorf("cyclic Go value cannot be encoded as YAML")
			}
			seen[key] = true
			defer delete(seen, key)
		}
		switch v.Kind() {
		case reflect.Pointer:
			return walk(v.Elem(), depth+1)
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				field := v.Type().Field(i)
				if field.PkgPath == "" && strings.Split(field.Tag.Get("yaml"), ",")[0] != "-" {
					if strings.Contains(","+field.Tag.Get("yaml")+",", ",omitempty,") && v.Field(i).IsZero() {
						continue
					}
					if err := walk(v.Field(i), depth+1); err != nil {
						return err
					}
				}
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				if err := walk(iter.Value(), depth+1); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if err := walk(v.Index(i), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value, 0)
}
