# Troubleshooting and version-specific migration

Investigate the actual source/schema/input first. Preserve a failing fixture and
fix the narrowest cause. Do not hide failures by changing every field to TypeAny
or silently disabling unknown-key checks.

| Symptom | Check / correct action |
| --- | --- |
| Unknown keys do not make HasErrors true | Native inherited policy defaults to warning; set UnknownKeyError or StrictKeys |
| Every map key is unknown | TypeMap without AllowedKeys is not an arbitrary-map schema; use AdditionalProperties |
| An arbitrary object gets no recursive errors | Bare TypeAny does not constrain structure |
| Blank required value is accepted | Presence differs from nonempty; inspect nullability and the field's type/validators |
| Default was not applied | This library validates, it does not fill defaults |
| Both token and username pass | OneOfRequired counts complete groups; add ForbiddenTogether/dependencies |
| MutuallyExclusive accepts neither key | Its contract is zero or one; use ExactlyOneOf for exactly one |
| Union matches both branches | oneOf needs exactly one match; constrain branch types/properties, or use anyOf deliberately |
| Custom format never runs | Register it and enable AssertFormat where the draft defaults to annotation |
| Added a custom keyword but got no error | JSON unknown keywords may be ignored; register Keywords/Vocabularies on compilation |
| Compile keyword error appears only on input validation | Use Compile or Vocabulary.Schema rather than Validate to check configuration early |
| Relative $ref requests an unexpected HTTPS URI | Default SchemaURL is in-memory HTTPS; set the root retrieval URL correctly |
| File resolver did not resolve a ref | Check $id/base URI, file URL, root containment and sentinel error |
| additionalProperties warning policy did not fix oneOf | Diagnostic policy does not rewrite branch matching |
| Callback panics on a map | Use type assertions; maps/slices cannot be compared through interface == |
| JSON Schema callback cannot find a YAML node | It receives the JSON data model; use native callbacks for YAML tags/styles |
| Large numbers round or Int64 fails | Keep json.Number / ExactNumber; choose conversions matching the required range |
| Cannot compileFieldSchema on a recursive graph | Use JSON Schema refs for recursion; native definition checker rejects it |
| Cancellation returns a result with no errors | Inspect returned error and Canceled/Truncated before HasErrors |
| Required missing field has parent's location | The missing node has no source position |
| Native schema file rejects a Go API field | CLI serialization covers a subset; see cli.md |

## Migration from older library versions

Inspect the exact selected version with go list -m, not an old README or copied
snippet. This skill targets v1.0.0. In v1:

- Use Resolver and JSONSchemaResolverFunc, not LoadURL or JSONSchemaLoadFunc.
- Use Formats, Keywords, Vocabularies, ContentEncodings, ContentMediaTypes and
  CompileWithContext; ConfigureCompiler is not available.
- Use package-level InferNodeType; InferTypeForPublic is not available.
- SourceLines belongs to ValidationResult, not ValidationContext.
- JSONSchemaKeywordIssue is not exported. Use AddError / AddErrorAt.
- Application-specific DirectoryValidator, PluginSourceValidator,
  ManagedDisableValidator and ManagedOverrideValidator are not generic v1 APIs.

Do not add compatibility shims to the library while integrating an application
unless requested. Preserve a project's pinned version and Go toolchain policy;
propose a dependency upgrade separately when necessary.

## Differences worth keeping in regression tests

Use separate expected outcomes for native and JSON Schema integer classification
of 1.0. Do not expect UnknownKeyDetails.Suggestion for JSON Schema's
additionalProperties failure. Do not expect JSON Schema format assertions to
act like native RegexValidator registration. Do not expect JSON Schema default
annotations to emit the native missing-default warning.

If the desired rule cannot be expressed by this API, say which part is absent
and choose an explicit application callback or a standard JSON Schema rule.
Never invent an exported helper to make an example look shorter.
