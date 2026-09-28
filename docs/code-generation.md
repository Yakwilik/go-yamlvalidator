# Typed YAML code generation

The generator adds typed YAML node encoders and decoders to selected Go types. It writes a separate file in the package and leaves source files untouched. The checked-in example is in examples/codegen/model; examples/codegen is runnable with go run ./examples/codegen.

## Generate

Run this from the package containing the types:

~~~sh
go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -type=Config,Item -output=zz_yamlvalidator_generated.go
~~~

Check a generated file in CI without changing it:

~~~sh
go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -type=Config,Item -output=zz_yamlvalidator_generated.go -check
~~~

The generator uses go/packages and loads types under the active Go build constraints. It rejects a selected type declared only in a conditional source file, since an unconstrained generated file would fail in other builds. Regeneration replaces its own prior output with temporary method signatures during type analysis, so field changes and user code referring to generated interfaces still compile. It refuses to replace a file without its generated marker. Existing user methods on a selected type cause a definition error. Separate output files receive distinct helper names.

## Calls and validation

Generated types implement the yaml.v3 MarshalYAML and UnmarshalYAML hooks. The standard yaml.v3 API validates generated types through those hooks. It does not validate types without hooks. The high-level yamlvalidator Marshal, Unmarshal, MarshalOptions, and UnmarshalOptions APIs retain their existing signatures.

The high-level APIs parse and validate once, then call an unchecked typed decode, or encode a typed node once and validate the emitted bytes. Generated child codecs also use unchecked operations inside a validated parent. A per-call decode context carries limits through nested generated values; when SkipValidation is set, it bounds typed node and YAML merge work. A runtime parent containing a generated child uses a reflective mixed-tree fallback so options such as SkipValidation, UnknownKeyPolicy, registry bindings, and callbacks stay on the enclosing call.

The generator follows the statically known graph reachable from each selected root and emits typed codec helpers and a normalized native schema graph. Validation tags are parsed and lowered during generation; the generated file contains field constraints, child edges, validator specifications, group rules, conditions, and symbolic registry references. At runtime the library allocates and wires `FieldSchema` nodes, creates validators from those specifications, and resolves registry names and type bindings. The generated root's YAML hooks and high-level operations validate with that graph directly. The `YAMLValidatorSchema` method returns a native schema that callers can pass to `CompileFieldSchema`. The runtime reflection compiler remains for types without generated code, and mixed trees and dynamic interface values still use reflection. Generated cycle preflight visits known fields through typed helpers; it uses reflection only to identify pointer and slice containers.

Standard yaml.v3 hooks receive a node, not raw input bytes or a full options object. They use default validation policy. yaml.v3 can bypass a hook for null, and method promotion on anonymous embeddings can invoke an inner hook with the inner receiver. Use the high-level APIs for per-call options and source-backed diagnostics.

## Supported shapes and limits

The generated codec directly handles structs, named primitive values, pointers, slices, arrays, and string-keyed maps. It supports yaml omitempty, flow, and inline fields, including inline maps. Primitive scalar nodes are encoded and decoded directly. Time values, byte slices, custom YAML and text codecs, and opaque interface values use an isolated node fallback and retain the existing type-binding requirements for validation.

Recursive Go type graphs are supported through child-value edges, including slices and pointers. Finite YAML values validate and decode; cyclic Go values fail during Marshal. Native schema compilation rejects same-node composition cycles. The parser, native rule semantics, diagnostics, registry checks, and low-level JSON Schema API remain shared with the runtime path. The generator does not add streaming, interpolation, metadata-only fields, or a JSON Schema route for native tags.

Keep the generated file in version control alongside its source types so consumers can run go test without a generation step.

## Checked-in example

From the repository root:

~~~sh
go generate ./examples/codegen/model
go run ./examples/codegen
go test -count=1 ./examples/tagrules
~~~

The example generation directive includes Config, Item, Containers, Dynamic, Node, Link, and Registered. For a freshness-only check in that package, use the same full type list with -check.
