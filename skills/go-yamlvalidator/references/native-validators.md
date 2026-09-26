# Native value and key validators

Use imports v for the root package, valv for pkg/valuevalidator, keyv for
pkg/keyvalidator, and yaml for gopkg.in/yaml.v3. The executable
[built-in tests](../examples/recipes/builtins_test.go) and
[custom callback tests](../examples/recipes/custom_test.go) are the complete recipes.

## Built-in values

Attach these values to <code>FieldSchema.Validators</code>. Use a matching field
type rather than relying on a validator to coerce or infer everything.

| Validator | Configuration | Notes |
| --- | --- | --- |
| EnumValidator | Allowed []string, Message string | Compares scalar text; nonempty list, no duplicates in checked definitions |
| RegexValidator | Pattern *regexp.Regexp, Message string | Go regexp, not the JSON Schema ECMAScript matcher; pattern must be non-nil |
| RangeValidator | Min/Max *float64, MinExact/MaxExact *ExactNumber | Inclusive bounds; choose one form per side |
| NonEmptyValidator | None | Scalar text length or container content; does not trim whitespace |
| LengthValidator | Min/Max *int | Unicode rune count for scalar text, item count for sequences, raw pair count for mappings |
| URLValidator | RequireScheme bool, AllowedSchemes []string | Syntax/scheme check; does not check reachability or require a host by itself |
| OneOfTypeValidator | Types []NodeType | Concrete type alternatives; TypeFloat also accepts TypeInt |

For unions prefer FieldSchema.AllowedTypes. Do not put TypeAny into an
OneOfTypeValidator list expecting a wildcard: use a bare TypeAny field instead.
Nullable permits null at the type layer, but native callbacks still receive it.
Add a null guard to a custom callback when null should bypass that check.

~~~go
field := &v.FieldSchema{
    Type: v.TypeString,
    Validators: []v.ValueValidator{
        valv.EnumValidator{Allowed: []string{"dev", "stage", "prod"}},
    },
}
~~~

## Exact bounds

~~~go
bound, err := valv.ParseExactNumber("9007199254740993")
if err != nil {
    return err
}
field := &v.FieldSchema{
    Type: v.TypeInt,
    Validators: []v.ValueValidator{
        valv.RangeValidator{MinExact: &bound},
    },
}
~~~

ParseExactNumber returns a value, not a pointer. MustExactNumber returns a pointer
and may panic; use it only for checked, static literals. ExactNumber.String may
use a rational representation; do not assume it is a JSON number literal.
Legacy Min/Max float64 values are interpreted via their shortest decimal
representation. Precision already lost when constructing a float64 cannot be
recovered. Bounded NaN fails; JSON Schema rejects non-finite YAML numbers at the
conversion layer altogether. Do not compare arbitrary JSON numbers through Int64
or Float64 unless the rule deliberately restricts that range.

## Built-in keys

| Validator | Configuration |
| --- | --- |
| RegexKeyValidator | Pattern *regexp.Regexp, Message string |
| ForbiddenKeyValidator | Forbidden []string, Message string |
| LengthKeyValidator | Min/Max *int; Unicode rune counts |

They belong to the mapping's KeyValidators, not to its value Validators. Checked
forbidden lists must be nonempty and unique.

~~~go
schema := &v.FieldSchema{
    Type: v.TypeMap,
    AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
    KeyValidators: []v.KeyValidator{
        keyv.RegexKeyValidator{Pattern: regexp.MustCompile("^[a-z][a-z0-9_]*$")},
        keyv.ForbiddenKeyValidator{Forbidden: []string{"secret"}},
        keyv.LengthKeyValidator{Min: v.Ptr(1), Max: v.Ptr(32)},
    },
}
~~~

## Application-defined callbacks

There is no exported native ValueValidatorFunc or KeyValidatorFunc in v1.0.0.
Implement the interface with a struct, or define a function adapter in the
application. Do not invent a library helper. Complete adapter example:

~~~go
type valueCheck func(*yaml.Node, string, *v.ValidationContext)

func (f valueCheck) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
    f(node, path, ctx)
}

func forbidReserved(node *yaml.Node, path string, ctx *v.ValidationContext) {
    if node.Tag == "!!null" || node.Value != "reserved" {
        return
    }
    ctx.AddError(v.ValidationError{
        Level: v.LevelError,
        Code: "reserved_name",
        Path: path,
        Line: node.Line,
        Column: node.Column,
        Message: "name is reserved",
    })
}
~~~

Attach <code>valueCheck(forbidReserved)</code> to Validators. A key callback has
signature <code>ValidateKey(key string, keyNode *yaml.Node, path string, ctx
*v.ValidationContext)</code>. The path already identifies the current field; do
not append its name a second time. Level's zero value is warning, so explicitly
set LevelError for rejection. Use NewValidationContext when testing callbacks
directly; normal validation initializes the collector for you.

A parent value callback runs after structural checks unless validation has
stopped; it must type-check the node itself and tolerate other diagnostics.
Native callbacks receive source nodes, including tags/styles; JSON Schema
callbacks do not. Never modify nodes to repair user data during validation.

For expensive native work, check ctx.IsStopped and use ctx.Context. Implement
ValidateDefinition() error to reject malformed callback configuration in
CompileFieldSchema. Implement CloneValueValidator() v.ValueValidator or
CloneKeyValidator() v.KeyValidator to snapshot custom configuration. Closures
capturing mutable maps are not magically isolated by cloning an interface.
