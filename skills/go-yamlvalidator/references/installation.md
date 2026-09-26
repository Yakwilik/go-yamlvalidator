# Installation and integration choice

This skill targets <code>github.com/Yakwilik/go-yamlvalidator v1.0.0</code>, whose
minimum Go directive is <code>1.24.0</code>. The plugin version is independent.
Inspect the caller's go.mod before changing dependencies; do not upgrade Go to
satisfy an example without asking. For older library versions, read their API.

~~~bash
go version
go list -m github.com/Yakwilik/go-yamlvalidator
go get github.com/Yakwilik/go-yamlvalidator@v1.0.0
~~~

The last command changes the current Go module and may download dependencies.
Run it only when dependency installation is part of the task. There is no /v1
suffix. Preserve uppercase <code>Yakwilik</code> in the module and import paths.

~~~go
import (
    v "github.com/Yakwilik/go-yamlvalidator"
    keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
    valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
    "gopkg.in/yaml.v3"
)
~~~

Include only used imports. Import yaml.v3 only for decoding or native callbacks.
Do not import the internal JSON Schema engine for supported extension APIs.

## Native versus JSON Schema

A <code>FieldSchema</code> is already a Go description of the rules. The normal
entry point is <code>NewValidator(schema)</code>. This keeps the supplied pointer;
finish building the schema before using it and do not mutate it concurrently.

<code>ValidateFieldSchema(schema)</code> checks the definition without creating a
validator. <code>CompileFieldSchema(schema)</code> checks it and takes a snapshot.
Use the latter when a schema is assembled dynamically or later mutated. It is
not an obligatory compiler pass for native validation and does not compile a
new optimized bytecode engine.

JSON Schema bytes first need <code>CompileJSONSchema</code> (or an options/context
variant). The returned <code>*FieldSchema</code> is an adapter with a compiled
validator attached, not a converted editable tree of AllowedKeys. Do not rebuild
it from Type or assume its AllowedKeys mirror the JSON Schema properties.

## Validation before application decoding

Keep the existing config struct if it is useful. Read the bytes once, validate,
then decode the same bytes. This application-level example rejects errors and
retains warnings for the caller:

~~~go
func ValidateThenDecode(data []byte, validator *v.Validator, target any) (*v.ValidationResult, error) {
    result := validator.ValidateBytes(data)
    if result.HasErrors() {
        return result, fmt.Errorf("configuration validation failed")
    }
    if err := yaml.Unmarshal(data, target); err != nil {
        return result, fmt.Errorf("decode validated configuration: %w", err)
    }
    return result, nil
}
~~~

This function requires imports fmt, v and yaml as above. Schema acceptance does
not guarantee every Go target type can decode. Decoder options and validation
options are separate: enabling YAML11Booleans for validation does not configure
a second decoder. For a single-config application, also decide whether multiple
YAML documents and an empty stream are acceptable; see
[execution](execution-and-testing.md) and [YAML semantics](diagnostics-and-yaml.md).

## Runnable examples

From the library checkout:

~~~bash
go run ./skills/go-yamlvalidator/examples/native
go run ./skills/go-yamlvalidator/examples/jsonschema
go test ./skills/go-yamlvalidator/examples/...
~~~

An installed skill may be outside any Go module. Its check-examples.sh script
copies only the bundled examples into a temporary module, pins v1.0.0, and runs
the tests. Pass <code>--local /path/to/go-yamlvalidator</code> to test a checkout
without editing the application's go.mod. Both commands and source files are
examples for explicit execution, not automatic hooks.
