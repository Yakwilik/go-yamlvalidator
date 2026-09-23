# yamlvalidator

A flexible, production-ready YAML validation library for Go with support for:

- **Type checking** with YAML 1.2 (and optional YAML 1.1) compliance
- **JSON Schema compilation** for the structural subset used by EasyP
- **Custom validators** for values and keys
- **Conditional logic** (AnyOf, ExactlyOneOf, MutuallyExclusive, Conditions)
- **Detailed error reporting** with source context and precise positions
- **Multi-document YAML** support
- **Anchor/alias** support
- **Unicode and tab** handling in error output

## Installation

```bash
go get github.com/Yakwilik/go-yamlvalidator
```

## Quick Start

```go
package main

import (
    "fmt"
    "regexp"

    v "github.com/Yakwilik/go-yamlvalidator"
    valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func main() {
    schema := &v.FieldSchema{
        Type: v.TypeMap,
        AllowedKeys: map[string]*v.FieldSchema{
            "name": {
                Type:     v.TypeString,
                Required: true,
                Validators: []v.ValueValidator{
                    valv.RegexValidator{
                        Pattern: regexp.MustCompile(`^[a-z][a-z0-9-]*$`),
                        Message: "must be lowercase with dashes",
                    },
                },
            },
            "replicas": {
                Type: v.TypeInt,
                Validators: []v.ValueValidator{
                    valv.RangeValidator{Min: v.Ptr[float64](1), Max: v.Ptr[float64](100)},
                },
            },
        },
    }

    yaml := []byte(`
name: my-app
replicas: 50
`)

    validator := v.NewValidator(schema)
    result := validator.ValidateBytes(yaml)

    if result.HasErrors() {
        fmt.Println(result.FormatAll(true))
    }
}
```

## Schema Definition

### Field Schema

```go
type FieldSchema struct {
    // Basic properties
    Type         NodeType   // Expected type (TypeString, TypeInt, etc.)
    AllowedTypes []NodeType // Accept any listed type; takes precedence over Type
    Required     bool       // Field must be present
    Nullable    bool        // Allow null values
    Deprecated  string      // Deprecation message (empty = not deprecated)
    Default     interface{} // Default value (warning if missing)

    // Map-specific
    AllowedKeys          map[string]*FieldSchema // Known keys
    AdditionalProperties *FieldSchema            // Schema for unknown keys
    UnknownKeyPolicy     UnknownKeyPolicy        // How to handle unknown keys
    KeyValidators        []KeyValidator          // Key name validators

    // Sequence-specific
    ItemSchema *FieldSchema // Schema for items
    MinItems   *int
    MaxItems   *int

    // Value validators
    Validators []ValueValidator

    // Inter-field logic
    AnyOf             [][]string        // At least one group must be present
    ExactlyOneOf      []string          // Exactly one field must be present
    MutuallyExclusive []string          // At most one field can be present
    Conditions        []ConditionalRule // Conditional validation
    OneOfRequired     [][]string        // Exactly one field group must be present
    ForbiddenTogether [][]string        // A complete field group is forbidden
    DependentRequired map[string][]string

    // Schema composition
    OneOfSchemas []*FieldSchema // Exactly one schema must match
    AnyOfSchemas []*FieldSchema // At least one schema must match
}
```

### Node Types

| Type | Description |
|------|-------------|
| `TypeAny` | Any type; bare maps/sequences are not recursively constrained |
| `TypeNull` | Null values only |
| `TypeString` | String values |
| `TypeInt` | Integer values |
| `TypeFloat` | Float values (also accepts int) |
| `TypeBool` | Boolean values |
| `TypeMap` | Mapping nodes |
| `TypeSequence` | Sequence/array nodes |

### Unknown Key Policy

| Policy | Behavior |
|--------|----------|
| `UnknownKeyInherit` | Uses `ctx.StrictKeys` (default) |
| `UnknownKeyError` | Unknown keys are errors |
| `UnknownKeyWarn` | Unknown keys are warnings |
| `UnknownKeyIgnore` | Unknown keys are ignored |

## JSON Schema Compilation

<code>CompileJSONSchema</code> compiles JSON Schema directly into <code>FieldSchema</code>. The compiler currently supports the structural JSON Schema subset used by EasyP: <code>type</code> (including arrays of types), <code>properties</code>, <code>required</code>, <code>additionalProperties</code>, <code>items</code>, <code>minItems</code>, <code>maxItems</code>, <code>oneOf</code>, <code>anyOf</code>, <code>not</code>, and <code>dependentRequired</code>. Boolean schemas are supported as well. Unknown schema keywords are rejected explicitly instead of being ignored.

~~~go
schemaJSON, err := os.ReadFile("easyp-config.schema.json")
if err != nil {
    return err
}

schema, err := v.CompileJSONSchema(schemaJSON)
if err != nil {
    return err
}

result := v.NewValidator(schema).ValidateBytes(yamlData)
~~~

JSON Schema normally treats <code>additionalProperties: false</code> as a validation error. Callers such as configuration editors can downgrade unknown keys to warnings while preserving the same canonical schema:

~~~go
policy := v.UnknownKeyWarn
schema, err := v.CompileJSONSchemaWithOptions(schemaJSON, v.JSONSchemaCompileOptions{
    AdditionalPropertiesFalsePolicy: &policy,
})
~~~

## Built-in Validators

### Value Validators

```go
// Enum validation
EnumValidator{Allowed: []string{"v1", "v2", "v3"}}

// Regex validation
RegexValidator{
    Pattern: regexp.MustCompile(`^[a-z]+$`),
    Message: "must be lowercase letters",
}

// Numeric range
RangeValidator{Min: v.Ptr[float64](1), Max: v.Ptr[float64](100)}

// Non-empty check
NonEmptyValidator{}

// Length validation
LengthValidator{Min: v.Ptr[int](1), Max: v.Ptr[int](255)}

// URL validation
URLValidator{RequireScheme: true, AllowedSchemes: []string{"http", "https"}}
```

### Key Validators

```go
// Regex key validation
RegexKeyValidator{
    Pattern: regexp.MustCompile(`^[a-z][a-z0-9._-]*$`),
    Message: "invalid key format",
}

// Forbidden keys
ForbiddenKeyValidator{Forbidden: []string{"password", "secret"}}

// Key length
LengthKeyValidator{Min: v.Ptr[int](1), Max: v.Ptr[int](63)}
```

## Inter-field Logic

### AnyOf (at least one group)

```go
// Either configFile, OR both host AND port
AnyOf: [][]string{{"configFile"}, {"host", "port"}}
```

### ExactlyOneOf (exactly one field)

```go
// Exactly one of inline/file/url
ExactlyOneOf: []string{"inline", "file", "url"}
```

### MutuallyExclusive (at most one)

```go
// debug and quiet cannot both be present
MutuallyExclusive: []string{"debug", "quiet"}
```

### Conditional Rules

```go
Conditions: []ConditionalRule{
    {
        ConditionField: "type",
        ConditionValue: "external",
        ThenRequired:   []string{"endpoint"},
        ThenForbidden:  []string{"local"},
    },
}
```

## Validation Options

```go
result := validator.ValidateWithOptions(yaml, ValidationContext{
    StrictKeys:     true,  // Unknown keys are errors
    StopOnFirst:    false, // Continue after first error
    StrictTypes:    false, // Parse values for type inference
    YAML11Booleans: false, // When true, plain YAML 1.1 literals such as yes/no/on/off are booleans; quoted scalars remain strings
})
```

## CLI

The repository ships a small CLI to validate any YAML file using a schema described in YAML or JSON (a serialized `FieldSchema`). Provide the schema file with `-schema` and the YAML to validate with `-file` (or stdin).

Minimal schema example (`/tmp/schema.yaml`):

```yaml
type: map
required: true
unknownKeyPolicy: warn
allowedKeys:
  name:
    type: string
    required: true
  replicas:
    type: int
    validators:
      - name: range
        min: 1
        max: 10
```

Validate a file:

```bash
go run ./cmd/yamlvalidator \
  -schema /tmp/schema.yaml \
  -file ./path/to/config.yaml \
  -strict-keys \
  -yaml11-bools
```

Flags: `-schema` (required), `-file` (defaults to stdin), `-strict-keys`, `-stop-on-first`, `-strict-types`, `-yaml11-bools`, and `-sort`.

## Error Handling

### Error Levels

- `LevelError` - Critical validation failures
- `LevelWarning` - Non-critical issues (deprecated fields, unknown keys in permissive mode)

### Error Formatting

```go
result := validator.ValidateBytes(yaml)

// Get all errors
for _, err := range result.Collector.Errors() {
    fmt.Println(err)
}

// Format with source context
fmt.Println(result.FormatAll(true)) // true = sort by position
```

### Example Output

```
[ERROR] line 2:13: invalid value "v3" (expected one of [v1 v1beta1 apps/v1], got v3) (path: apiVersion)
     1 |
>    2 | apiVersion: v3
       |             ^
     3 | kind: Deployment

[ERROR] line 5:9: must be lowercase DNS-compatible name (got MyApp) (path: metadata.name)
     4 | metadata:
>    5 |   name: MyApp
       |         ^
     6 |   namespace: production
```

## Multi-Document Support

The validator automatically handles multi-document YAML (separated by `---`):

```yaml
name: first
---
name: second
---
name: third
```

Errors in subsequent documents are prefixed with `doc[N].`:

```
[ERROR] line 5:1: required field "name" is missing (path: doc[2].name)
```

## Custom Validators

### Value Validator

```go
type MyValidator struct{}

func (v MyValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
    if node.Value != "expected" {
        ctx.AddError(ValidationError{
            Level:   LevelError,
            Path:    path,
            Line:    node.Line,
            Column:  node.Column,
            Message: "value is not expected",
            Got:     node.Value,
        })
    }
}
```

### Key Validator

```go
type MyKeyValidator struct{}

func (v MyKeyValidator) ValidateKey(key string, keyNode *yaml.Node, path string, ctx *ValidationContext) {
    if strings.HasPrefix(key, "_") {
        ctx.AddError(ValidationError{
            Level:   LevelWarning,
            Path:    path,
            Line:    keyNode.Line,
            Column:  keyNode.Column,
            Message: "keys starting with underscore are reserved",
            Got:     key,
        })
    }
}
```

## Best Practices

### 1. Use `AdditionalProperties` for arbitrary keys

```go
// Allow any string values for unknown keys
labels: {
    Type:                 TypeMap,
    AdditionalProperties: &FieldSchema{Type: TypeString},
}
```

### 2. Combine with key validators

```go
labels: {
    Type:                 TypeMap,
    AdditionalProperties: &FieldSchema{Type: TypeString},
    KeyValidators: []KeyValidator{
        RegexKeyValidator{Pattern: regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)},
    },
}
```

### 3. Use `Nullable` for optional fields that can be explicitly null

```go
timeout: {
    Type:     TypeInt,
    Nullable: true, // Allows: timeout: null
}
```

### 4. Use helper functions for pointers

```go
MinItems: Ptr[int](1)
Max:      Ptr[float64](100)
```

## API Reference

### Validator

```go
// Create validator
v := NewValidator(schema)

// Validate bytes
result := v.ValidateBytes(yamlData)

// Validate with options
result := v.ValidateWithOptions(yamlData, ValidationContext{...})
```

### ValidationResult

```go
result.HasErrors()              // bool
result.Collector.Errors()       // []ValidationError
result.Collector.Warnings()     // []ValidationError
result.Collector.All()          // []ValidationError in collection order
result.SortByPosition()         // Sort by line/column
result.FormatAll(sortByPos)     // Format with source context
```

### ValidationError

```go
type ValidationError struct {
    Level    ErrorLevel
    Path     string    // e.g., "spec.containers[0].image"
    Line     int       // 1-based (0 if unknown)
    Column   int       // 1-based (0 if unknown)
    Message  string
    Got      string    // Actual value/type
    Expected string    // Expected value/type
}
```

## License

Apache License 2.0
