# Changelog

## Unreleased
- Reworked `CompileJSONSchema`/`CompileJSONSchemaWithOptions` to use a full JSON Schema engine instead of translating a limited keyword subset. Supports draft-04, draft-06, draft-07, 2019-09, and 2020-12, including refs/dynamic refs, vocabularies, unevaluated keywords, conditional schemas, and all engine-supported constraints.
- Added external schema resources/loaders, format/content/vocabulary assertion controls, custom compiler configuration, ECMAScript-compatible regex handling, and stricter standard format validators.
- Added YAML-to-JSON data-model conversion with exact numeric handling and source-position mapping for JSON Schema diagnostics. Official core JSON-Schema-Test-Suite result: 4950/4950 passing.
- Added first-class schema unions, dependent-required constraints, forbidden field groups, and multi-type schemas.
- Fixed YAML merge precedence so explicit keys always win and earlier mappings in a merge sequence take precedence.
- Added document-wide duplicate mapping-key detection.
- Fixed bare `TypeAny` so arbitrary maps/sequences are not recursively treated as unknown fields.
- YAML 1.1 compatibility booleans now apply only to plain scalars; quoted values remain strings.
- Preserved diagnostic insertion order in `ErrorCollector.All` and `SortByPosition`.
- Fixed range validation for legacy integer forms and bounded NaN values.
- Made CLI schema loading reject unknown schema fields and preserve descriptions.
- Added standalone CLI (`cmd/yamlvalidator`) that validates YAML using a schema described in YAML/JSON (serialized FieldSchema), with flags for strict keys, YAML 1.1 booleans, type strictness, and stop-on-first.
- Added schema loader tests for YAML/JSON inputs and validation of validator names.
- Refactored `examples/easyp` to share its schema via `examples/easyp/schema` instead of defining validators inline.

## v0.1.0
- Initial release of `go-yamlvalidator`:
  - YAML 1.2 validation core with optional YAML 1.1 boolean support.
  - Schema-driven validation: types, required/nullable/default/deprecated, maps/sequences, unknown key policies, inter-field rules (AnyOf/ExactlyOneOf/MutuallyExclusive/Conditions), multi-document and alias handling, position-aware error formatting.
  - Built-in validators split into packages:
    - `pkg/valuevalidator`: enum, regex, range, non-empty, length, URL, one-of-type, custom easyp validators.
    - `pkg/keyvalidator`: regex, forbidden, length (rune-count aware).
  - Public helpers: pointer helpers, exported caret renderer, type inference helper for external validators.
  - Examples: Kubernetes-like validation (`examples/basic`) and `easyp` config validator (`examples/easyp`), plus README guides for key/value validators and settings.
  - Tests: coverage for types, nullability, unknown keys, sequences, inter-field logic, YAML 1.1 booleans, aliases, formatting, merge keys, numeric parsing in RangeValidator, Unicode key lengths, and position sorting.
