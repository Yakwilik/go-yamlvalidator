# API coverage and source map

This package documents <code>github.com/Yakwilik/go-yamlvalidator v1.1.0</code>.
The skill/plugin has its own version, <code>0.2.0</code>. It neither upgrades the
Go module nor changes library behavior. These references describe the v1.1.0
surface, including high-level struct tags and optional typed code generation.

## Coverage map

| Public surface | Reference | Executable coverage |
| --- | --- | --- |
| High-level Marshal/Unmarshal, Options, yamlvalidate tags, Registry bindings | [High-level tags and codegen](high-level-and-codegen.md) | highlevel_test.go, library high-level acceptance tests |
| yamlvalidator-gen -all/-type/-check, generated yaml.v3 hooks, recursive generated models | [High-level tags and codegen](high-level-and-codegen.md) | generator integration tests, checked-in generated examples |
| Module installation, NewValidator, optional CompileFieldSchema and ValidateFieldSchema | [Installation](installation.md), [native schemas](native-schema.md) | [Native program](../examples/native/main.go), native_test.go |
| All FieldSchema fields, NodeType constants, UnknownKeyPolicy values, ConditionalRule | [Native schemas](native-schema.md) | native_test.go |
| ValueValidator, KeyValidator, DefinitionValidator, both cloner interfaces | [Native validators](native-validators.md), [execution](execution-and-testing.md) | custom_test.go, execution_test.go |
| Enum, Regex, Range, NonEmpty, Length, URL, OneOfType, all key validators | [Native validators](native-validators.md) | builtins_test.go |
| Ptr, ExactNumber, ParseExactNumber, MustExactNumber, InferNodeType | [Native validators](native-validators.md) | builtins_test.go, diagnostics_test.go |
| All four CompileJSONSchema functions, all five JSONSchemaDraft constants, all JSONSchemaCompileOptions fields | [JSON Schema](json-schema.md) | jsonschema_test.go, resolvers_test.go |
| JSONSchemaFormat, all three keyword callbacks, keyword compilation and validation contexts | [Extensions](json-schema-extensions.md) | extensions_test.go |
| JSONSchemaVocabulary and Schema, JSONSchemaSubschema, all four position selectors, Subschema, Reference, ValidateSubschema | [Extensions](json-schema-extensions.md) | extensions_test.go |
| JSONSchemaContentEncoding and JSONSchemaContentMediaType | [Extensions](json-schema-extensions.md) | content_test.go |
| JSONSchemaResolver, ResolverFunc, ResourceMap, ResolverChain, both resolver constructors and errors | [Resolvers](resolvers.md) | resolvers_test.go |
| All eight Validator validation entry points, ValidationContext options and methods | [Execution](execution-and-testing.md) | execution_test.go, custom_test.go |
| ValidationResult, ErrorCollector methods, ErrorLevel, ValidationError, all six Details types | [Diagnostics](diagnostics-and-yaml.md) | diagnostics_test.go, jsonschema_test.go |
| PathToken, PathTokenKind, ParsePathTokens, FormatPathTokens, AppendPropertyPath, AppendIndexPath | [Diagnostics](diagnostics-and-yaml.md) | diagnostics_test.go |
| FormatErrorWithSource, RenderLineWithCaret, source coordinates | [Diagnostics](diagnostics-and-yaml.md) | diagnostics_test.go |
| Standalone CLI flags, native serialized schema subset and exit codes | [CLI](cli.md) | [Fixtures](../assets/valid.yaml), package integration tests |

Test filenames in this table are relative to
[examples/recipes](../examples/recipes/README.md). They are Go tests, not a claim
that every possible schema or YAML input has been exercised.

## Primary implementation sources

When working in the library repository, read the checked-out files. When using
an installed skill, consult these pinned source links only when the bundled
reference does not answer a version-specific question:

| Source | Owns |
| --- | --- |
| [api.go](https://github.com/Yakwilik/go-yamlvalidator/blob/v1.1.0/api.go) | Application-facing root facade and stable exported names |
| [internal/engine](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/internal/engine) | High-level tag compiler, native validation, JSON Schema bridge, diagnostics, registry and plan cache |
| [genruntime](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/genruntime) | ABI used by generated YAML hooks and typed codec helpers |
| [cmd/yamlvalidator-gen](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/cmd/yamlvalidator-gen) | Go source/type frontend, -all selection and code generation |
| [pkg/valuevalidator](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/pkg/valuevalidator) | Native value checks, exact bounds, cloning |
| [pkg/keyvalidator](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/pkg/keyvalidator) | Native key checks |
| [CLI](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/cmd/yamlvalidator) | CLI loading, options, serialization support |

Implementation takes precedence over README shortcuts. For example, native
unknown keys are warnings by default, native schema compilation is optional,
and the CLI's native schema loader cannot express every exported FieldSchema
field. Do not silently substitute desired behavior for current behavior.

## Maintenance

For a new library release, inspect changed exports and semantics, update the
relevant reference and tests, run the recipe suite against both the checkout and
the published version, and update the source revision only after verification.
Check that the plugin version, frontmatter version, package documentation, and
marketplace candidate metadata agree. Do not rewrite published library tags as
part of skill maintenance.
